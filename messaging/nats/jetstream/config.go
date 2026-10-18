package jetstream

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	natsgo "github.com/nats-io/nats.go"

	"github.com/kbukum/gokit/messaging"
)

// AdapterName is the adapter's name in a [messaging.Registry].
const AdapterName = "jetstream"

// MsgIDHeader carries a message's deduplication ID. Within the stream's duplicates window, a second message with the
// same ID is acknowledged without being stored again. [Producer.Publish] sets it to the event ID.
const MsgIDHeader = natsgo.MsgIdHdr

// Bounds and defaults.
const (
	// MaxFetchBatch bounds the messages one consumer fetch may hold unacknowledged.
	MaxFetchBatch = 1000
	// MaxReplayLimit bounds the messages one replay page returns.
	MaxReplayLimit = 1000
	// MaxMessageIDBytes bounds a deduplication ID.
	MaxMessageIDBytes = 128

	DefaultAckWait       = 30 * time.Second
	DefaultMaxAckPending = 200
	DefaultBatch         = 50
	DefaultFetchWait     = 2 * time.Second
	DefaultRetryDelay    = time.Second
	DefaultMaxRetryDelay = 30 * time.Second

	keyHeader      = "message-key"
	cleanupTimeout = 2 * time.Second
)

var namePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// StreamConfig declares a file-backed stream with limits retention. Old messages are discarded when a limit is hit.
type StreamConfig struct {
	Name       string
	Subjects   []string
	MaxAge     time.Duration
	MaxBytes   int64
	MaxMsgSize int32
	// Duplicates is the deduplication window for message IDs; it must not exceed MaxAge.
	Duplicates time.Duration
	// Replicas defaults to 1.
	Replicas int
}

// Validate checks the stream declaration.
func (c StreamConfig) Validate() error {
	if !namePattern.MatchString(c.Name) {
		return fmt.Errorf("jetstream: invalid stream name %q", c.Name)
	}
	if len(c.Subjects) == 0 {
		return errors.New("jetstream: stream subjects are required")
	}
	for _, s := range c.Subjects {
		if err := validateSubject(s, true); err != nil {
			return err
		}
	}
	if c.MaxAge <= 0 || c.MaxBytes <= 0 || c.MaxMsgSize <= 0 || c.Duplicates <= 0 {
		return errors.New("jetstream: stream max_age, max_bytes, max_msg_size and duplicates must be > 0")
	}
	if c.Duplicates > c.MaxAge {
		return errors.New("jetstream: stream duplicates window must not exceed max_age")
	}
	if c.Replicas < 0 || c.Replicas > 5 {
		return errors.New("jetstream: stream replicas must be between 0 and 5; 0 means 1")
	}
	return nil
}

// ProducerConfig shapes a producer's publications.
type ProducerConfig struct {
	// Stream, when set, fails each publication unless that stream stores it.
	Stream string
	// RetryAttempts bounds attempts for a message with a deduplication ID; zero means one. Others are sent once.
	RetryAttempts int
	// RetryBackoff is the delay between attempts; it grows linearly.
	RetryBackoff time.Duration
}

// Validate checks the producer configuration.
func (c ProducerConfig) Validate() error {
	if c.Stream != "" && !namePattern.MatchString(c.Stream) {
		return fmt.Errorf("jetstream: invalid stream name %q", c.Stream)
	}
	if c.RetryAttempts < 0 || c.RetryBackoff < 0 {
		return errors.New("jetstream: producer retry_attempts and retry_backoff must be >= 0")
	}
	return nil
}

// ConsumerConfig declares a durable pull consumer with explicit acknowledgement that starts at the first retained
// message, and bounds the loop that reads it. Zero bounds take the Default values.
type ConsumerConfig struct {
	// Stream is the stream to read; when empty it is the stream that stores the first filter subject.
	Stream  string
	Durable string
	// Bind only binds to an existing durable consumer instead of declaring it, so the user needs no right to create
	// consumers. The declaration fields below then come from the server; the loop bounds still apply.
	Bind bool
	// Declare is the stream this consumer owns, if any. Consume creates or updates it each time it binds the durable,
	// so the stream's limits follow this configuration and a stream lost in a server restart comes back without
	// waiting for another process. Its name must be Stream, it does not apply with Bind, and the user needs the
	// [Client.EnsureStream] rights.
	Declare *StreamConfig
	// FilterSubjects limits the consumer to matching subjects; empty reads the whole stream.
	FilterSubjects []string
	// AckWait is how long a delivered message stays unacknowledged before redelivery. It must cover a whole batch.
	AckWait time.Duration
	// MaxAckPending bounds delivered, unacknowledged messages; it must be at least Batch.
	MaxAckPending int
	// MaxDeliver bounds deliveries per message; zero means unlimited. A message that reaches it is no longer
	// delivered but stays in the stream. A handler can also end redelivery at once with [ErrTerminal].
	MaxDeliver int
	// Batch bounds the messages fetched at once, up to MaxFetchBatch.
	Batch int
	// FetchWait bounds how long one fetch waits for messages.
	FetchWait time.Duration
	// RetryDelay is the first redelivery or reconnect delay; it doubles per attempt up to MaxRetryDelay.
	RetryDelay    time.Duration
	MaxRetryDelay time.Duration
	// OnError observes broker, acknowledgement and handler failures; the loop continues after each. It is required, so
	// failures are never silent.
	OnError func(error)
}

// ApplyDefaults fills zero bounds.
func (c *ConsumerConfig) ApplyDefaults() {
	if c.AckWait == 0 {
		c.AckWait = DefaultAckWait
	}
	if c.MaxAckPending == 0 {
		c.MaxAckPending = DefaultMaxAckPending
	}
	if c.Batch == 0 {
		c.Batch = min(DefaultBatch, c.MaxAckPending)
	}
	if c.FetchWait == 0 {
		c.FetchWait = DefaultFetchWait
	}
	if c.RetryDelay == 0 {
		c.RetryDelay = DefaultRetryDelay
	}
	if c.MaxRetryDelay == 0 {
		c.MaxRetryDelay = max(DefaultMaxRetryDelay, c.RetryDelay)
	}
}

// Validate checks the consumer declaration and bounds after defaults.
func (c ConsumerConfig) Validate() error {
	if c.Stream != "" && !namePattern.MatchString(c.Stream) {
		return fmt.Errorf("jetstream: invalid stream name %q", c.Stream)
	}
	if !namePattern.MatchString(c.Durable) {
		return fmt.Errorf("jetstream: invalid durable name %q", c.Durable)
	}
	if c.OnError == nil {
		return errors.New("jetstream: consumer on_error is required")
	}
	if c.Stream == "" && len(c.FilterSubjects) == 0 {
		return errors.New("jetstream: consumer needs a stream or a filter subject")
	}
	if c.Declare != nil {
		if c.Bind || c.Declare.Name != c.Stream {
			return errors.New("jetstream: consumer declare needs the consumer's own stream and no bind")
		}
		if err := c.Declare.Validate(); err != nil {
			return err
		}
	}
	for _, s := range c.FilterSubjects {
		if err := validateSubject(s, true); err != nil {
			return err
		}
	}
	if c.AckWait <= 0 || c.MaxAckPending <= 0 || c.MaxDeliver < 0 {
		return errors.New("jetstream: consumer ack_wait and max_ack_pending must be > 0 and max_deliver >= 0")
	}
	if c.Batch < 1 || c.Batch > MaxFetchBatch || c.Batch > c.MaxAckPending {
		return fmt.Errorf("jetstream: consumer batch must be between 1 and min(%d, max_ack_pending)", MaxFetchBatch)
	}
	if c.FetchWait <= 0 || c.RetryDelay <= 0 || c.MaxRetryDelay < c.RetryDelay {
		return errors.New("jetstream: consumer fetch_wait and retry_delay must be > 0 and max_retry_delay >= retry_delay")
	}
	return nil
}

// topic names what the consumer reads, for logs and [messaging.Consumer.Topic].
func (c ConsumerConfig) topic() string {
	if len(c.FilterSubjects) > 0 {
		return strings.Join(c.FilterSubjects, ",")
	}
	return c.Stream
}

func validateSubject(s string, wildcards bool) error {
	if s == "" || len(s) > 255 || strings.HasPrefix(s, ".") || strings.HasSuffix(s, ".") || strings.Contains(s, "..") {
		return fmt.Errorf("jetstream: invalid subject %q", s)
	}
	tokens := strings.Split(s, ".")
	for i, token := range tokens {
		if wildcards && (token == "*" || (token == ">" && i == len(tokens)-1)) {
			continue
		}
		if err := messaging.ValidateTopic(token); err != nil {
			return fmt.Errorf("jetstream: invalid subject %q", s)
		}
	}
	return nil
}

func validateMessageID(id string) error {
	if id == "" || len(id) > MaxMessageIDBytes || strings.ContainsFunc(id, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsControl(r)
	}) {
		return fmt.Errorf("jetstream: message ID must be 1-%d bytes without spaces or control characters", MaxMessageIDBytes)
	}
	return nil
}
