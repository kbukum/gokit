package messaging

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/kbukum/gokit/logger"
)

// ProducerFactory constructs a producer for a registered backend.
//
// providerCfg carries backend-specific configuration, matching cache/storage
// factory registration: registration is config-free, while construction is
// runtime-configured and may create multiple instances from one registry.
type ProducerFactory func(context.Context, Config, any, *logger.Logger) (Producer, error)

// ConsumerFactory constructs a consumer for a registered backend and topic.
type ConsumerFactory func(context.Context, Config, any, *logger.Logger, string) (Consumer, error)

// ConfigTypeError reports a backend-specific provider config type mismatch.
type ConfigTypeError struct {
	Backend  string
	Expected string
	Actual   any
}

func (e *ConfigTypeError) Error() string {
	return fmt.Sprintf("messaging: backend %s expected provider config %s, got %T", e.Backend, e.Expected, e.Actual)
}

// Registry stores explicitly registered messaging backend factories.
// Registries are application-owned and injected; packages never register
// backends through init side effects.
type Registry struct {
	mu        sync.RWMutex
	producers map[string]ProducerFactory
	consumers map[string]ConsumerFactory
}

// NewRegistry creates an empty messaging backend registry.
func NewRegistry() *Registry {
	return &Registry{
		producers: make(map[string]ProducerFactory),
		consumers: make(map[string]ConsumerFactory),
	}
}

// RegisterProducer registers a producer factory under backend.
func (r *Registry) RegisterProducer(backend string, factory ProducerFactory) error {
	if backend == "" {
		return fmt.Errorf("messaging: backend name is required")
	}
	if factory == nil {
		return fmt.Errorf("messaging: producer factory for %q is nil", backend)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.producers[backend]; exists {
		return fmt.Errorf("messaging: producer backend %q already registered", backend)
	}
	r.producers[backend] = factory
	return nil
}

// RegisterConsumer registers a consumer factory under backend.
func (r *Registry) RegisterConsumer(backend string, factory ConsumerFactory) error {
	if backend == "" {
		return fmt.Errorf("messaging: backend name is required")
	}
	if factory == nil {
		return fmt.Errorf("messaging: consumer factory for %q is nil", backend)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.consumers[backend]; exists {
		return fmt.Errorf("messaging: consumer backend %q already registered", backend)
	}
	r.consumers[backend] = factory
	return nil
}

// NewProducer constructs a producer using cfg.Backend and runtime provider config.
func (r *Registry) NewProducer(ctx context.Context, cfg Config, providerCfg any, log *logger.Logger) (Producer, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.IsEnabled() {
		return nil, fmt.Errorf("messaging: backend %q is disabled", cfg.Backend)
	}
	r.mu.RLock()
	factory, ok := r.producers[cfg.Backend]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("messaging: producer backend %q is not registered", cfg.Backend)
	}
	producer, err := factory(ctx, cfg, providerCfg, messagingLogger(log))
	if err != nil {
		return nil, err
	}
	if len(cfg.Topics) > 0 {
		producer = topicRestrictedProducer{Producer: producer, allowed: stringSet(cfg.Topics)}
	}
	return producer, nil
}

// NewConsumer constructs a consumer using cfg.Backend, runtime provider config, and topic.
func (r *Registry) NewConsumer(ctx context.Context, cfg Config, providerCfg any, log *logger.Logger, topic string) (Consumer, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.IsEnabled() {
		return nil, fmt.Errorf("messaging: backend %q is disabled", cfg.Backend)
	}
	if err := ValidateTopic(topic); err != nil {
		return nil, err
	}
	if err := validateConfiguredTopic("subscription", topic, cfg.Subscriptions); err != nil {
		return nil, err
	}
	if err := validateConfiguredTopic("topic", topic, cfg.Topics); err != nil {
		return nil, err
	}
	r.mu.RLock()
	factory, ok := r.consumers[cfg.Backend]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("messaging: consumer backend %q is not registered", cfg.Backend)
	}
	return factory(ctx, cfg, providerCfg, messagingLogger(log), topic)
}

// ProducerBackends returns registered producer backend names.
func (r *Registry) ProducerBackends() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortedKeys(r.producers)
}

// ConsumerBackends returns registered consumer backend names.
func (r *Registry) ConsumerBackends() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortedKeys(r.consumers)
}

func messagingLogger(log *logger.Logger) *logger.Logger {
	if log == nil {
		log = logger.NewDefault("messaging")
	}
	return log.WithComponent("messaging")
}

func sortedKeys[F any](m map[string]F) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type topicRestrictedProducer struct {
	Producer
	allowed map[string]struct{}
}

func (p topicRestrictedProducer) Send(ctx context.Context, msg Message) error {
	if !containsTopic(p.allowed, msg.Topic) {
		return fmt.Errorf("messaging: topic %q is not configured", msg.Topic)
	}
	return p.Producer.Send(ctx, msg)
}

func (p topicRestrictedProducer) SendBatch(ctx context.Context, messages []Message) error {
	for _, msg := range messages {
		if !containsTopic(p.allowed, msg.Topic) {
			return fmt.Errorf("messaging: topic %q is not configured", msg.Topic)
		}
	}
	return p.Producer.SendBatch(ctx, messages)
}

func (p topicRestrictedProducer) Publish(ctx context.Context, topic string, event Event, key ...string) error {
	if !containsTopic(p.allowed, topic) {
		return fmt.Errorf("messaging: topic %q is not configured", topic)
	}
	return p.Producer.Publish(ctx, topic, event, key...)
}

func (p topicRestrictedProducer) PublishJSON(ctx context.Context, topic, key string, value any) error {
	if !containsTopic(p.allowed, topic) {
		return fmt.Errorf("messaging: topic %q is not configured", topic)
	}
	return p.Producer.PublishJSON(ctx, topic, key, value)
}

func (p topicRestrictedProducer) PublishBinary(ctx context.Context, topic, key string, data []byte) error {
	if !containsTopic(p.allowed, topic) {
		return fmt.Errorf("messaging: topic %q is not configured", topic)
	}
	return p.Producer.PublishBinary(ctx, topic, key, data)
}

func validateConfiguredTopic(kind, topic string, configured []string) error {
	if len(configured) == 0 || containsTopic(stringSet(configured), topic) {
		return nil
	}
	return fmt.Errorf("messaging: %s %q is not configured", kind, topic)
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	return set
}

func containsTopic(set map[string]struct{}, topic string) bool {
	_, ok := set[topic]
	return ok
}
