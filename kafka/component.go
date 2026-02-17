package kafka

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	kafkago "github.com/segmentio/kafka-go"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
)

// ProducerCloser is satisfied by any producer that can be closed.
type ProducerCloser interface {
	Close() error
}

// ConsumerRunner is satisfied by any consumer that can run a consume loop.
type ConsumerRunner interface {
	Consume(ctx context.Context) error
	Close() error
	Topic() string
}

// Component wraps injected Producer and Consumer(s) and implements component.Component.
type Component struct {
	cfg       Config
	log       *logger.Logger
	producer  ProducerCloser
	consumers []ConsumerRunner
	cancelFn  context.CancelFunc
	wg        sync.WaitGroup
	mu        sync.Mutex
	running   bool
}

// ensure Component satisfies component.Component
var _ component.Component = (*Component)(nil)

// NewComponent creates a Kafka component for use with the component registry.
func NewComponent(cfg Config, log *logger.Logger) *Component {
	return &Component{
		cfg: cfg,
		log: log.WithComponent("kafka"),
	}
}

// SetProducer injects a producer into the component. Must be called before Start.
func (c *Component) SetProducer(p ProducerCloser) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.producer = p
}

// AddConsumer injects a consumer into the component. Must be called before Start.
func (c *Component) AddConsumer(cr ConsumerRunner) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.consumers = append(c.consumers, cr)
}

// Producer returns the underlying ProducerCloser, or nil if not set.
func (c *Component) Producer() ProducerCloser {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.producer
}

// Name returns the component name.
func (c *Component) Name() string { return "kafka" }

// Start begins consuming in background goroutines for all injected consumers.
func (c *Component) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return nil
	}

	consumeCtx, cancel := context.WithCancel(ctx)
	c.cancelFn = cancel

	for _, cr := range c.consumers {
		cr := cr
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			if err := cr.Consume(consumeCtx); err != nil && err != context.Canceled {
				c.log.Error("Consumer stopped with error", map[string]interface{}{
					"topic": cr.Topic(),
					"error": err.Error(),
				})
			}
		}()
	}

	c.running = true
	c.log.Info("Kafka component started")
	return nil
}

// Stop gracefully shuts down consumers and the producer.
func (c *Component) Stop(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	c.log.Info("Kafka component stopping")

	if c.cancelFn != nil {
		c.cancelFn()
	}
	c.wg.Wait()

	for _, cr := range c.consumers {
		_ = cr.Close()
	}
	c.consumers = nil

	if c.producer != nil {
		_ = c.producer.Close()
		c.producer = nil
	}

	c.running = false
	return nil
}

// BrokerHealth represents individual broker health status.
type BrokerHealth struct {
	Address string        `json:"address"`
	Status  string        `json:"status"` // "healthy", "unhealthy", "degraded"
	Error   string        `json:"error,omitempty"`
	Latency time.Duration `json:"latency"`
}

// Health checks broker connectivity by dialling all configured brokers.
// Returns degraded status when some (but not all) brokers are unreachable.
func (c *Component) Health(ctx context.Context) component.Health {
	c.mu.Lock()
	running := c.running
	cfg := c.cfg
	c.mu.Unlock()

	if !running {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "kafka not started",
		}
	}

	if len(cfg.Brokers) == 0 {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: "no brokers configured",
		}
	}

	dialer, err := CreateDialer(&cfg)
	if err != nil {
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: fmt.Sprintf("dialer: %v", err),
		}
	}

	healthy := 0
	var issues []string
	for _, broker := range cfg.Brokers {
		bh := c.checkBroker(ctx, dialer, broker)
		if bh.Status == "healthy" {
			healthy++
		} else if bh.Error != "" {
			issues = append(issues, fmt.Sprintf("%s: %s", bh.Address, bh.Error))
		}
	}

	total := len(cfg.Brokers)
	switch {
	case healthy == total:
		return component.Health{
			Name:   c.Name(),
			Status: component.StatusHealthy,
		}
	case healthy > 0:
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusDegraded,
			Message: fmt.Sprintf("%d/%d brokers healthy; %s", healthy, total, strings.Join(issues, "; ")),
		}
	default:
		return component.Health{
			Name:    c.Name(),
			Status:  component.StatusUnhealthy,
			Message: fmt.Sprintf("no brokers available; %s", strings.Join(issues, "; ")),
		}
	}
}

func (c *Component) checkBroker(ctx context.Context, dialer *kafkago.Dialer, addr string) BrokerHealth {
	bh := BrokerHealth{Address: addr}
	start := time.Now()

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		bh.Status = "unhealthy"
		bh.Error = err.Error()
		bh.Latency = time.Since(start)
		return bh
	}
	defer conn.Close()

	if _, err := conn.Brokers(); err != nil {
		bh.Status = "degraded"
		bh.Error = err.Error()
	} else {
		bh.Status = "healthy"
	}

	bh.Latency = time.Since(start)
	return bh
}

// Describe returns infrastructure summary info for the bootstrap display.
func (c *Component) Describe() component.Description {
	c.mu.Lock()
	defer c.mu.Unlock()

	details := fmt.Sprintf("brokers=%v", c.cfg.Brokers)
	topics := make([]string, 0, len(c.consumers))
	for _, cr := range c.consumers {
		topics = append(topics, cr.Topic())
	}
	if len(topics) > 0 {
		details += fmt.Sprintf(" topics=%v", topics)
	}
	if c.producer != nil {
		details += " producer=yes"
	}
	return component.Description{
		Name:    "Kafka",
		Type:    "kafka",
		Details: details,
	}
}
