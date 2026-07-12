# gokit: Integration Patterns

This document shows how gokit modules compose together to solve common microservice challenges. Each pattern demonstrates a practical workflow combining multiple modules.

## Pattern 1: Server + Discovery

**Problem**: Start an HTTP/gRPC server and automatically register it with a discovery service (Consul, etcd, etc.) for automatic deregistration on shutdown.

**Solution**: Wrap your `Server` component with `DiscoveryServerComponent` from the `server` package. The discovery wrapper automatically calls register on `Start()` and deregister on `Stop()`, ensuring graceful cleanup.

**Code example**:

```go
package main

import (
	"context"
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/discovery"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/server"
)

func setupDiscoveryServer(
	httpServer *server.Component,
	discoveryRegistry discovery.Registry,
	log *logging.Logger,
) (*server.DiscoveryServerComponent, error) {
	// Wrap the server with automatic discovery registration
	discoveryServer, err := server.NewDiscoveryServerComponent(
		httpServer,
		discoveryRegistry,
		"payment-svc-1",      // unique service instance ID
		"payment-service",    // logical service name
		"127.0.0.1",          // advertised address
		8080,                 // port to register
		[]string{"v1", "prod"}, // optional tags
		map[string]string{"region": "us-west"},
		log,
	)
	if err != nil {
		return nil, err
	}
	return discoveryServer, nil
}

func main() {
	ctx := context.Background()
	log := logging.NewDefault("payment-service")

	// Create HTTP server component
	httpServer := server.NewComponent(server.New(&server.Config{
		Port:    8080,
		Network: "tcp",
	}, log))

	// Setup discovery provider (e.g., Consul) — implements discovery.Registry
	consulProvider, err := consul.NewProvider(discovery.Config{}, nil, log)
	if err != nil {
		panic(err)
	}

	// Wrap with discovery
	discServer, err := setupDiscoveryServer(httpServer, consulProvider, log)
	if err != nil {
		panic(err)
	}
	
	// Use like any component
	component.Register(discServer)
	component.StartAll(ctx)
	defer component.StopAll(ctx)
}
```

**Modules involved**:
- `github.com/kbukum/gokit/server` — Server, DiscoveryServerComponent
- `github.com/kbukum/gokit/discovery` — Registry interface
- `github.com/kbukum/gokit/component` — Component lifecycle

---

## Pattern 2: Messaging + Middleware Stack

**Problem**: Process messages from a topic with automatic retry, metrics tracking, tracing, deduplication, and circuit breaker protection without manually nesting middleware.

**Solution**: Use `StackBuilder` from `messaging/middleware` to compose a typed middleware stack with a clean, chainable API. The builder applies middleware in a fixed, sensible order (tracing → metrics → dedup → circuit breaker → retry).

**Code example**:

```go
package main

import (
	"context"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/middleware"
	"github.com/kbukum/gokit/messaging/kafka"
)

func setupHandler(baseHandler messaging.MessageHandler, log *logging.Logger) messaging.MessageHandler {
	// Build a resilient message handler with multiple layers
	return middleware.NewStack(baseHandler).
		WithRetry(middleware.RetryMiddlewareConfig{
			MaxRetries: 3,
			BackoffMs:  100,
			DLQTopic:   "events.dlq",
		}).
		WithMetrics("orders.events", "order-processor").
		WithTracing().
		WithDedup(middleware.DedupConfig{
			Window: 60, // seconds
		}).
		WithCircuitBreaker(middleware.CircuitBreakerConfig{
			FailureThreshold: 5,
			SuccessThreshold: 2,
			Timeout:          30,
		}).
		Build()
}

func processOrder(ctx context.Context, msg *messaging.Message) error {
	// Your business logic
	return nil
}

func main() {
	// Create base handler
	baseHandler := messaging.HandlerFunc(processOrder)
	
	// Wrap with middleware
	resilientHandler := setupHandler(baseHandler, log)
	
	// Create consumer with wrapped handler
	consumer := kafka.NewConsumer(
		kafka.Config{Brokers: []string{"localhost:9092"}},
		"orders.created",
		"order-processor",
		resilientHandler,
	)
	
	consumer.Start(context.Background())
	defer consumer.Stop(context.Background())
}
```

**Modules involved**:
- `github.com/kbukum/gokit/messaging` — MessageHandler, Message types
- `github.com/kbukum/gokit/messaging/middleware` — StackBuilder, RetryMiddlewareConfig, etc.
- `github.com/kbukum/gokit/messaging/kafka` — Kafka consumer

---

## Pattern 3: gRPC Client + Discovery

**Problem**: Create a gRPC client that dynamically discovers and connects to a remote service with lazy initialization and built-in resilience.

**Solution**: Use `DiscoveryConnectionFactory` to resolve services by name via discovery, then wrap the connection in a `LazyClient` for thread-safe, lazy initialization on first use.

**Code example**:

```go
package main

import (
	"context"
	"github.com/kbukum/gokit/discovery"
	"github.com/kbukum/gokit/grpc/client"
	"github.com/kbukum/gokit/logging"
	"google.golang.org/grpc"
	
	analysispb "github.com/myorg/analysis/api"
)

func setupAnalysisClient(
	discoveryClient *discovery.Client,
	log *logging.Logger,
) *client.LazyClient[analysispb.AnalysisServiceClient] {
	// Create a discovery-based connection factory
	factory := client.NewDiscoveryConnectionFactory(
		discoveryClient,
		grpc.DefaultCallOptions,
		log,
	)
	
	// Wrap in a lazy client for first-use initialization
	lazyClient := client.NewLazyClient(
		"analysis-service",
		factory,
		analysispb.NewAnalysisServiceClient,
		log,
	)
	
	return lazyClient
}

func main() {
	log := logging.NewDefault("my-service")
	consulProvider, _ := consul.NewProvider(discovery.Config{}, nil, log)
	discoveryClient := discovery.NewClient(consulProvider, discovery.ClientConfig{}, log)

	// Setup lazy gRPC client
	analysisClient := setupAnalysisClient(discoveryClient, log)
	
	// First call triggers discovery and connection
	client, err := analysisClient.GetClient()
	if err != nil {
		panic(err)
	}
	
	// Use the client
	resp, err := client.Analyze(context.Background(), &analysispb.AnalyzeRequest{
		Data: "hello world",
	})
	if err != nil {
		panic(err)
	}
}
```

**Modules involved**:
- `github.com/kbukum/gokit/grpc/client` — DiscoveryConnectionFactory, LazyClient
- `github.com/kbukum/gokit/discovery` — Client, Query, StrategyRoundRobin
- `google.golang.org/grpc` — ClientConn, gRPC types

---

## Pattern 4: EventPublisher + Messaging

**Problem**: Publish domain events from your application with automatic envelope (UUID, timestamp, source) construction without manual envelope handling.

**Solution**: Wrap a `Producer` with `EventPublisher` from the `messaging` package. The facade handles Event envelope creation automatically—you only provide the topic, event type, and payload.

**Code example**:

```go
package main

import (
	"context"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/messaging"
	"github.com/kbukum/gokit/messaging/kafka"
)

type OrderCreatedEvent struct {
	OrderID    string
	CustomerID string
	Amount     float64
}

func publishOrderEvents(kafkaProducer messaging.Producer, log *logging.Logger) *messaging.EventPublisher {
	// Wrap the producer with auto-envelope support
	return messaging.NewEventPublisher(kafkaProducer, "order-service")
}

func main() {
	// Create Kafka producer
	producer, err := kafka.NewProducer(kafka.Config{
		Brokers: []string{"localhost:9092"},
	})
	if err != nil {
		panic(err)
	}
	defer producer.Close()
	
	// Create event publisher
	eventPub := publishOrderEvents(producer, log)
	
	// Publish a domain event (no manual envelope needed)
	ctx := context.Background()
	err = eventPub.Publish(ctx, "orders.created", "order.created.v1", OrderCreatedEvent{
		OrderID:    "order-123",
		CustomerID: "cust-456",
		Amount:     99.99,
	})
	if err != nil {
		panic(err)
	}
	
	// Publish with partition key for ordering
	err = eventPub.PublishKeyed(ctx, "orders.created", "order.created.v1", OrderCreatedEvent{
		OrderID:    "order-124",
		CustomerID: "cust-457",
		Amount:     199.99,
	}, "cust-457") // key ensures all events for this customer go to same partition
	if err != nil {
		panic(err)
	}
}
```

**Modules involved**:
- `github.com/kbukum/gokit/messaging` — EventPublisher, Producer, Event types
- `github.com/kbukum/gokit/messaging/kafka` — Kafka producer implementation

---

## Pattern 5: TickerWorker + Resilience

**Problem**: Run periodic health checks or background tasks reliably with degradation tracking, so that if a critical task fails repeatedly, your system can degrade gracefully.

**Solution**: Create a `TickerWorker` for periodic execution and pair it with `DegradationManager` to track health state and make feature decisions based on the current health.

**Code example**:

```go
package main

import (
	"context"
	"time"
	
	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logging"
	"github.com/kbukum/gokit/resilience"
	"github.com/kbukum/gokit/worker"
)

type HealthCheckService struct {
	cacheSvc      interface{}      // Your cache service
	degradation   *resilience.DegradationManager
	log           *logging.Logger
}

func (h *HealthCheckService) checkCacheHealth(ctx context.Context) error {
	// Simulate a health check
	err := h.cacheSvc.Ping(ctx)
	if err != nil {
		// Mark cache as degraded
		h.degradation.UpdateService("cache", resilience.Degraded, err)
		return err
	}
	
	// Cache is healthy
	h.degradation.UpdateService("cache", resilience.Healthy)
	return nil
}

func (h *HealthCheckService) shouldUseCacheOptimization() bool {
	// Check degradation state before applying optimization
	status := h.degradation.GetService("cache")
	if status.Health == resilience.Healthy {
		return true // Use cache optimization
	}
	h.log.Warn("Cache is degraded; skipping optimization")
	return false
}

func setupHealthChecker(log *logging.Logger) *worker.TickerWorker {
	degradation := resilience.NewDegradationManager()
	healthChecker := &HealthCheckService{
		degradation: degradation,
		log:         log,
	}
	
	// Create a periodic health check that runs every 30 seconds
	ticker := worker.NewTickerWorker(
		"cache-health-check",
		30*time.Second,
		healthChecker.checkCacheHealth,
	)
	
	return ticker
}

func main() {
	ctx := context.Background()
	
	// Setup health checker
	healthTicker := setupHealthChecker(log)
	
	// Register and start
	component.Register(healthTicker)
	component.StartAll(ctx)
	defer component.StopAll(ctx)
	
	// Worker runs in background, checking cache health every 30s
	select {}
}
```

**Modules involved**:
- `github.com/kbukum/gokit/worker` — TickerWorker, TickerFunc
- `github.com/kbukum/gokit/resilience` — DegradationManager, ServiceHealth
- `github.com/kbukum/gokit/component` — Component lifecycle (optional, but shown here)

---

## Cross-Pattern Composition

All five patterns work together in a complete microservice:

1. **Service Registration** (Pattern 1) ensures your service is discoverable.
2. **gRPC Clients** (Pattern 3) use that discovery to call other services.
3. **Message Processing** (Pattern 2) uses middleware stacks to handle async events reliably.
4. **Event Publishing** (Pattern 4) broadcasts domain events to downstream consumers.
5. **Health Checks** (Pattern 5) track degradation and inform feature decisions.

**Example architecture**:

```go
// main.go — complete microservice wiring
func main() {
	ctx := context.Background()
	log := logging.NewDefault("my-service")
	
	// Setup discovery provider (Consul) — serves as both Registry and Discovery
	consulProvider, _ := consul.NewProvider(discovery.Config{}, nil, log)
	discoveryClient := discovery.NewClient(consulProvider, discovery.ClientConfig{}, log)
	
	// 1. HTTP server with discovery
	httpServer := server.NewComponent(server.New(&server.Config{Port: 8080}, log))
	discServer, _ := server.NewDiscoveryServerComponent(
		httpServer, consulProvider, "svc-1", "my-service", "127.0.0.1", 8080, nil, nil, log)
	component.Register(discServer)
	
	// 2. Kafka message handler with middleware stack
	kafkaProducer, _ := kafka.NewProducer(kafka.Config{})
	baseHandler := messaging.HandlerFunc(processOrderEvent)
	resHandler := middleware.NewStack(baseHandler).
		WithRetry(/* ... */).
		WithMetrics("orders", "group").
		WithTracing().
		Build()
	consumer := kafka.NewConsumer(kafka.Config{}, "orders.created", "group", resHandler)
	component.Register(consumer)
	
	// 3. Event publisher for publishing domain events
	eventPub := messaging.NewEventPublisher(kafkaProducer, "my-service")
	
	// 4. gRPC client for calling downstream services
	analysisLazyClient := setupAnalysisClient(discoveryClient, log)
	
	// 5. Health checks with degradation tracking
	healthTicker := setupHealthChecker(log)
	component.Register(healthTicker)
	
	// Start all components
	component.StartAll(ctx)
	defer component.StopAll(ctx)
}
```

This architecture provides:
- ✅ **Discoverability** — other services find and call you
- ✅ **Resilience** — retries, circuit breakers, dedup for message processing
- ✅ **Observability** — metrics and tracing through middleware
- ✅ **Graceful degradation** — health checks inform feature flags
- ✅ **Event-driven communication** — publish and consume async events

---

## Best Practices

1. **Always use DiscoveryServerComponent** for service registration to ensure automatic deregistration on shutdown.
2. **Compose middleware with StackBuilder** rather than manually nesting; it ensures a predictable, sensible order.
3. **Use LazyClient for gRPC** to defer connection creation until first use and share connections efficiently.
4. **Publish events with EventPublisher** to ensure consistent Event envelopes across your system.
5. **Track degradation with DegradationManager** to inform feature decisions and degrade gracefully under load.
