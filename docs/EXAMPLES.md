# gokit Usage Examples

A tour of common gokit patterns. For per-package details, see each package's own `README.md`.

## Config + Logging

```go
package main

import (
    "github.com/kbukum/gokit/config"
    "github.com/kbukum/gokit/logging"
)

type ServiceConfig struct {
    config.ServiceConfig `yaml:",inline" mapstructure:",squash"`
    Port int `yaml:"port"`
}

func main() {
    cfg := &ServiceConfig{}
    if err := config.LoadConfig("my-service", cfg,
        config.WithConfigFile("./config.yml"),
        config.WithEnvFile(".env"),
    ); err != nil {
        panic(err)
    }
    cfg.ApplyDefaults()

    log := logging.New(&logging.Config{
        Level:  "info",
        Format: "console",
    }, cfg.Name)

    log.Info("service configured", map[string]interface{}{
        "env": cfg.Environment,
    })
}
```

## HTTP Server with Middleware

```go
import "github.com/kbukum/gokit/server"

srvCfg := &server.Config{Host: "0.0.0.0", Port: 8080}
srvCfg.ApplyDefaults()

srv := server.New(srvCfg, log)
srv.ApplyDefaults("my-service", healthChecker)

srv.GinEngine().GET("/api/items", itemsHandler)

srv.Start(ctx)
defer srv.Stop(ctx)
```

## Provider Pattern

```go
import "github.com/kbukum/gokit/provider"

// Define a domain provider using the interaction pattern
type DiarizationProvider = provider.RequestResponse[AudioInput, []Segment]

// Use the manager for runtime selection
reg := provider.NewRegistry[DiarizationProvider]()
mgr := provider.NewManager(reg, &provider.HealthCheckSelector[DiarizationProvider]{})
p, _ := mgr.Get(ctx)
result, err := p.Execute(ctx, audioInput)
```

## Sink Composition

```go
import "github.com/kbukum/gokit/provider"

kafkaSink := provider.NewSinkFunc("kafka", func(ctx context.Context, event Event) error {
    return producer.Publish(ctx, topic, event)
})

sink := provider.FanOutSink("multi",
    kafkaSink,
    provider.AdaptSink(analyticsSink, "adapt", toAnalyticsEvent),
    provider.TapSink(loggingSink, func(ctx context.Context, e Event) {
        metrics.RecordEvent(e.Type)
    }),
)

wrapped := provider.ChainSink(withLogging, withMetrics)(sink)
wrapped.Send(ctx, event)
```

## Subprocess Execution

```go
import "github.com/kbukum/gokit/process"

result, err := process.Run(ctx, process.Command{
    Binary: "python", Args: []string{"diarize.py", "audio.wav"},
})
fmt.Println(string(result.Stdout))
```

## Bootstrap Lifecycle

```go
import "github.com/kbukum/gokit/bootstrap"

app := bootstrap.NewApp("my-service", "1.0.0",
    bootstrap.WithLogger(log),
    bootstrap.WithGracefulTimeout(15 * time.Second),
)

app.RegisterComponent(db)    // component.Component
app.RegisterComponent(cache) // component.Component

app.OnConfigure(func(ctx context.Context, app *bootstrap.App) error {
    // All components started — set up routes, handlers, business logic
    return nil
})

// Run: Init → Start → Configure → Ready → wait for signal → Stop
if err := app.Run(ctx); err != nil {
    log.Fatal("app failed", map[string]interface{}{"error": err})
}
```

## Agent Loop

```go
import (
    "github.com/kbukum/gokit/agent"
    "github.com/kbukum/gokit/tool"
)

registry := tool.NewRegistry()
registry.Register(weatherTool)

a := agent.New(llmProvider, registry,
    agent.WithContextStrategy(&agent.SlidingWindowStrategy{MaxTokens: 4096}),
)
result, err := a.Run(ctx, "What's the weather in Berlin?")
fmt.Println(result.Events)
```

## LLM Chat Completion

```go
import "github.com/kbukum/gokit/llm"

provider := llm.NewProvider(llm.Config{
    Dialect: "openai",
    Model:   "gpt-4",
    APIKey:  os.Getenv("OPENAI_API_KEY"),
})

resp, err := provider.ChatCompletion(ctx, llm.Request{
    Messages: []llm.Message{
        {Role: "user", Content: "Explain circuit breakers"},
    },
})
fmt.Println(resp.Content)
```

## Tool Definition

```go
import "github.com/kbukum/gokit/tool"

t := tool.New("get_weather", "Get current weather for a city",
    tool.HandlerFunc(func(ctx context.Context, input map[string]any) (any, error) {
        city := input["city"].(string)
        return fetchWeather(ctx, city)
    }),
)

registry := tool.NewRegistry()
registry.Register(t)
```

## Messaging

```go
import "github.com/kbukum/gokit/messaging"

producer, _ := messaging.NewProducer(cfg)
producer.Publish(ctx, "events", messaging.Message{
    Key:   []byte("user-123"),
    Value: payload,
})

consumer, _ := messaging.NewConsumer(cfg, "my-group")
consumer.Subscribe("events", func(ctx context.Context, msg messaging.Message) error {
    return processEvent(msg)
})
consumer.Start(ctx)
```

## Object Storage

```go
import "github.com/kbukum/gokit/storage"

store, _ := storage.New(cfg)
_ = store.Put(ctx, "uploads/report.pdf", reader)
rc, _ := store.Get(ctx, "uploads/report.pdf")
defer rc.Close()
```
