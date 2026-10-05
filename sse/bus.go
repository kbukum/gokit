package sse

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"math"
	"strconv"
	"sync"
	"time"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/util"
)

// Bus owns single-instance scoped replay and live delivery. Publishing is synchronous and bounded; no dispatcher goroutine or inbound queue is needed.
type Bus struct {
	mu         sync.Mutex
	limits     Limits
	epoch      string
	sequence   uint64
	replay     []record
	head       int
	count      int
	replaySize int
	subs       map[*Subscription]struct{}
	principals map[string]int
	closed     bool
	stats      Stats
}

type record struct {
	sequence uint64
	pattern  string
	event    Event
	bytes    int
}

// NewBus validates limits before allocating replay storage and creates a cryptographically random instance epoch.
func NewBus(limits Limits) (*Bus, error) {
	if err := limits.validate(); err != nil {
		return nil, err
	}
	var epoch [16]byte
	if _, err := rand.Read(epoch[:]); err != nil {
		return nil, apperrors.Internal(err)
	}
	return &Bus{
		limits: limits, epoch: hex.EncodeToString(epoch[:]),
		replay: make([]record, limits.ReplayEvents),
		subs:   make(map[*Subscription]struct{}), principals: make(map[string]int),
	}, nil
}

// Cursor returns the current global high-water mark. It is not a snapshot transaction watermark or an acknowledgement of delivery.
func (b *Bus) Cursor() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.cursor()
}

func (b *Bus) cursor() string { return b.epoch + ":" + strconv.FormatUint(b.sequence, 10) }

// Publish encodes a proto message once and atomically appends it to replay and matching live queues. Patterns use util.GlobMatch; subscriber routes never contain wildcards.
func (b *Bus) Publish(ctx context.Context, pattern string, message proto.Message) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := validateKey(pattern, true); err != nil {
		return err
	}
	if util.IsNil(message) {
		return apperrors.InvalidInput("message", "SSE message is required")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.closed || b.sequence == math.MaxUint64 {
		return apperrors.ServiceUnavailable("SSE bus is closed or exhausted")
	}
	// Reject oversized input before proto-JSON allocation. Encoding is serialized so concurrent publishers cannot multiply temporary frame allocations.
	if proto.Size(message) > b.limits.MaxEventBytes {
		return apperrors.InvalidInput("message", "SSE event exceeds the frame limit")
	}
	data, err := protojson.Marshal(message)
	if err != nil {
		return apperrors.InvalidInput("message", "SSE message cannot be encoded").WithCause(err)
	}
	event := Event{
		ID:   b.epoch + ":" + strconv.FormatUint(b.sequence+1, 10),
		Name: string(message.ProtoReflect().Descriptor().FullName()), Data: string(data),
	}
	size := event.size()
	if size > b.limits.MaxEventBytes || size+len(pattern) > b.limits.ReplayBytes {
		return apperrors.InvalidInput("message", "SSE event exceeds the frame or replay byte limit")
	}
	b.sequence++
	rec := record{sequence: b.sequence, pattern: pattern, event: event, bytes: size}
	b.appendReplay(rec)
	for sub := range b.subs {
		if !sub.terminal && util.GlobMatch(pattern, sub.route) {
			sub.enqueue(rec)
		}
	}
	return nil
}

func (b *Bus) appendReplay(rec record) {
	size := rec.bytes + len(rec.pattern)
	for b.count == len(b.replay) || b.replaySize+size > b.limits.ReplayBytes {
		old := &b.replay[b.head]
		b.replaySize -= old.bytes + len(old.pattern)
		*old = record{}
		b.head = (b.head + 1) % len(b.replay)
		b.count--
	}
	b.replay[(b.head+b.count)%len(b.replay)] = rec
	b.count++
	b.replaySize += size
}

// SubscribeRequest contains verified routing and admission identity, never credentials. Cursor is untrusted. Changing authorized scope requires the consumer to clear its cursor.
type SubscribeRequest struct {
	Principal string
	Route     string
	Cursor    string
}

// Subscribe atomically captures a replay boundary and registers live delivery. Admission and cursor validation precede queue allocation. The caller owns Close; ctx cancellation also releases the subscription.
func (b *Bus) Subscribe(ctx context.Context, req SubscribeRequest) (*Subscription, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateKey(req.Principal, false); err != nil {
		return nil, err
	}
	if err := validateKey(req.Route, false); err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, apperrors.ServiceUnavailable("SSE bus is closed")
	}
	after, reset, err := b.resume(req.Cursor)
	if err != nil {
		return nil, err
	}
	if len(b.subs) >= b.limits.MaxConnections || b.principals[req.Principal] >= b.limits.MaxPerPrincipal {
		b.stats.RejectedConnections++
		// Capacity frees as soon as a relevant stream ends (any stream for the global limit, one of this principal's for the
		// per-principal limit), so clients get a short, explicit retry hint.
		return nil, apperrors.ServiceUnavailable("SSE connection limit reached").WithRetryAfter(time.Second)
	}
	s := &Subscription{
		bus: b, principal: req.Principal, route: req.Route,
		boundary: b.cursor(), replayAfter: after, replayUntil: b.sequence,
		queue: make([]record, b.limits.QueueEvents), wake: make(chan struct{}, 1),
	}
	if reset != "" {
		s.control = resetEvent(reset, b.cursor())
		b.stats.Resets++
	}
	b.subs[s] = struct{}{}
	b.principals[req.Principal]++
	b.stats.AllocatedQueues++
	s.stop = context.AfterFunc(ctx, s.Close)
	return s, nil
}

// Close stops acceptance and wakes all readers. It does not wait for HTTP handlers; each handler owns its bounded write and teardown.
func (b *Bus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for sub := range b.subs {
		sub.closeLocked()
	}
	clear(b.replay)
	b.count, b.replaySize = 0, 0
}
