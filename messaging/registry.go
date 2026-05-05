package messaging

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/kbukum/gokit/logger"
)

// ProducerFactory constructs a producer for a registered adapter.
//
// adapterCfg carries adapter-specific configuration, matching cache/storage
// factory registration: registration is config-free, while construction is
// runtime-configured and may create multiple instances from one registry.
type ProducerFactory func(context.Context, Config, any, *logger.Logger) (Producer, error)

// ConsumerFactory constructs a consumer for a registered adapter and topic.
type ConsumerFactory func(context.Context, Config, any, *logger.Logger, string) (Consumer, error)

// ConfigTypeError reports an adapter-specific adapter config type mismatch.
type ConfigTypeError struct {
	Adapter  string
	Expected string
	Actual   any
}

func (e *ConfigTypeError) Error() string {
	return fmt.Sprintf("messaging: adapter %s expected adapter config %s, got %T", e.Adapter, e.Expected, e.Actual)
}

// Registry stores explicitly registered messaging adapter factories.
// Registries are application-owned and injected; packages never register
// adapters through init side effects.
type Registry struct {
	mu        sync.RWMutex
	producers map[string]ProducerFactory
	consumers map[string]ConsumerFactory
}

// NewRegistry creates an empty messaging adapter registry.
func NewRegistry() *Registry {
	return &Registry{
		producers: make(map[string]ProducerFactory),
		consumers: make(map[string]ConsumerFactory),
	}
}

// RegisterProducer registers a producer factory under adapter.
func (r *Registry) RegisterProducer(adapter string, factory ProducerFactory) error {
	if adapter == "" {
		return fmt.Errorf("messaging: adapter name is required")
	}
	if factory == nil {
		return fmt.Errorf("messaging: producer factory for %q is nil", adapter)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.producers[adapter]; exists {
		return fmt.Errorf("messaging: producer adapter %q already registered", adapter)
	}
	r.producers[adapter] = factory
	return nil
}

// RegisterConsumer registers a consumer factory under adapter.
func (r *Registry) RegisterConsumer(adapter string, factory ConsumerFactory) error {
	if adapter == "" {
		return fmt.Errorf("messaging: adapter name is required")
	}
	if factory == nil {
		return fmt.Errorf("messaging: consumer factory for %q is nil", adapter)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.consumers[adapter]; exists {
		return fmt.Errorf("messaging: consumer adapter %q already registered", adapter)
	}
	r.consumers[adapter] = factory
	return nil
}

// NewProducer constructs a producer using cfg.Adapter and runtime adapter config.
func (r *Registry) NewProducer(ctx context.Context, cfg Config, adapterCfg any, log *logger.Logger) (Producer, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.IsEnabled() {
		return nil, fmt.Errorf("messaging: adapter %q is disabled", cfg.Adapter)
	}
	r.mu.RLock()
	factory, ok := r.producers[cfg.Adapter]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("messaging: producer adapter %q is not registered", cfg.Adapter)
	}
	msgLog := messagingLogger(log) //nolint:contextcheck // logger fallback construction has no request-scoped operation; ctx is passed to adapter factory
	producer, err := factory(ctx, cfg, adapterCfg, msgLog)
	if err != nil {
		return nil, err
	}
	if len(cfg.Topics) > 0 {
		producer = topicRestrictedProducer{Producer: producer, allowed: stringSet(cfg.Topics)}
	}
	return producer, nil
}

// NewConsumer constructs a consumer using cfg.Adapter, runtime adapter config, and topic.
func (r *Registry) NewConsumer(ctx context.Context, cfg Config, adapterCfg any, log *logger.Logger, topic string) (Consumer, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.IsEnabled() {
		return nil, fmt.Errorf("messaging: adapter %q is disabled", cfg.Adapter)
	}
	if err := ValidateTopic(topic); err != nil {
		return nil, err
	}
	// Use Subscriptions if configured, fall back to Topics for backward compatibility.
	// This allows distinct producer topics vs consumer subscriptions.
	allowedTopics := cfg.Subscriptions
	if len(allowedTopics) == 0 {
		allowedTopics = cfg.Topics
	}
	if err := validateConfiguredTopic("topic", topic, allowedTopics); err != nil {
		return nil, err
	}
	r.mu.RLock()
	factory, ok := r.consumers[cfg.Adapter]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("messaging: consumer adapter %q is not registered", cfg.Adapter)
	}
	msgLog := messagingLogger(log) //nolint:contextcheck // logger fallback construction has no request-scoped operation; ctx is passed to adapter factory
	return factory(ctx, cfg, adapterCfg, msgLog, topic)
}

// ProducerAdapters returns registered producer adapter names.
func (r *Registry) ProducerAdapters() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortedKeys(r.producers)
}

// ConsumerAdapters returns registered consumer adapter names.
func (r *Registry) ConsumerAdapters() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return sortedKeys(r.consumers)
}

func messagingLogger(log *logger.Logger) *logger.Logger {
	if log == nil {
		log = logger.NewDefault("messaging") //nolint:contextcheck // default logger construction has no request-scoped operation; ctx is passed to adapter factories separately
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
