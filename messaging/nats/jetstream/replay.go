package jetstream

import (
	"context"
	"errors"
	"fmt"
	"time"

	natsjs "github.com/nats-io/nats.go/jetstream"

	"github.com/kbukum/gokit/messaging"
)

// ReplayRequest selects retained messages to read without acknowledging them or changing any durable consumer. Exactly one of StartSequence and StartTime is set.
type ReplayRequest struct {
	Stream        string
	Subject       string
	StartSequence uint64
	StartTime     time.Time
	// Limit bounds the page, up to MaxReplayLimit.
	Limit int
	// Wait bounds how long the page waits for messages the server reported as pending.
	Wait time.Duration
}

// Validate checks the replay request.
func (r ReplayRequest) Validate() error {
	if !namePattern.MatchString(r.Stream) {
		return fmt.Errorf("jetstream: invalid stream name %q", r.Stream)
	}
	if err := validateSubject(r.Subject, true); err != nil {
		return err
	}
	if (r.StartSequence == 0) == r.StartTime.IsZero() {
		return errors.New("jetstream: replay needs exactly one of start sequence and start time")
	}
	if r.Limit < 1 || r.Limit > MaxReplayLimit {
		return fmt.Errorf("jetstream: replay limit must be between 1 and %d", MaxReplayLimit)
	}
	if r.Wait <= 0 {
		return errors.New("jetstream: replay wait must be > 0")
	}
	return nil
}

// ReplayPage is one bounded page of retained messages in stream order. Each message is mapped as [Consumer] maps it.
type ReplayPage struct {
	Messages []messaging.Message
	// Next is the stream sequence to pass as StartSequence for the following page.
	Next uint64
	// More reports that matching messages remained when the page was taken.
	More bool
}

// Replay reads one page of retained messages matching the request's subject through a short-lived consumer that it deletes afterwards. The server's inactivity threshold removes the consumer if deletion fails. The user needs the rights to create and delete consumers on the stream and, for a time start, to read the stream's info.
func (c *Client) Replay(ctx context.Context, r ReplayRequest) (ReplayPage, error) {
	if err := r.Validate(); err != nil {
		return ReplayPage{}, err
	}
	js, err := c.api()
	if err != nil {
		return ReplayPage{}, err
	}
	// For a time start, the stream's last sequence is read before the consumer counts pending messages: an empty page then resumes after that snapshot, never past a message stored meanwhile.
	var last uint64
	if r.StartSequence == 0 {
		reqCtx, cancel := c.request(ctx)
		stream, streamErr := js.Stream(reqCtx, r.Stream)
		cancel()
		if streamErr != nil {
			return ReplayPage{}, fmt.Errorf("jetstream: replay %s: %w", r.Stream, streamErr)
		}
		last = stream.CachedInfo().State.LastSeq
	}
	cc := natsjs.ConsumerConfig{
		FilterSubjects: []string{r.Subject}, AckPolicy: natsjs.AckNonePolicy,
		InactiveThreshold: max(r.Wait*4, 30*time.Second), MemoryStorage: true, Replicas: 1,
	}
	if r.StartSequence > 0 {
		cc.DeliverPolicy, cc.OptStartSeq = natsjs.DeliverByStartSequencePolicy, r.StartSequence
	} else {
		start := r.StartTime
		cc.DeliverPolicy, cc.OptStartTime = natsjs.DeliverByStartTimePolicy, &start
	}
	createCtx, cancelCreate := c.request(ctx)
	pull, err := js.CreateConsumer(createCtx, r.Stream, cc)
	cancelCreate()
	if err != nil {
		return ReplayPage{}, fmt.Errorf("jetstream: replay %s: %w", r.Stream, err)
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cleanupTimeout)
		defer cancel()
		_ = js.DeleteConsumer(cleanupCtx, r.Stream, pull.CachedInfo().Name)
	}()
	info := pull.CachedInfo()
	pending := info.NumPending
	page := ReplayPage{Next: max(info.Delivered.Stream+1, r.StartSequence)}
	if pending == 0 {
		if r.StartSequence == 0 {
			page.Next = last + 1
		}
		return page, nil
	}
	want := int(min(pending, uint64(r.Limit)))
	fetchCtx, cancel := context.WithTimeout(ctx, r.Wait)
	defer cancel()
	batch, err := pull.Fetch(want, natsjs.FetchContext(fetchCtx))
	if err != nil {
		return ReplayPage{}, fmt.Errorf("jetstream: replay %s: %w", r.Stream, err)
	}
	page.Messages = make([]messaging.Message, 0, want)
	for msg := range batch.Messages() {
		m, _, err := toMessage(msg)
		if err != nil {
			return ReplayPage{}, fmt.Errorf("jetstream: replay metadata: %w", err)
		}
		page.Messages = append(page.Messages, m)
		page.Next = uint64(m.Offset) + 1 //nolint:gosec // Offset is a stream sequence, never negative.
	}
	if err := batch.Error(); err != nil && !errors.Is(err, natsjs.ErrNoMessages) {
		return ReplayPage{}, fmt.Errorf("jetstream: replay %s: %w", r.Stream, err)
	}
	if len(page.Messages) < want {
		return ReplayPage{}, fmt.Errorf("jetstream: replay %s: received %d of %d pending messages", r.Stream, len(page.Messages), want)
	}
	page.More = pending > uint64(want)
	return page, nil
}
