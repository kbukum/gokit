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

    log.Info("service configured", map[string]any{
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
import (
    "context"

    "github.com/kbukum/gokit/bootstrap"
    "github.com/kbukum/gokit/config"
)

type AppConfig struct {
    config.ServiceConfig `mapstructure:",squash"`
}

cfg := &AppConfig{ServiceConfig: config.ServiceConfig{Name: "my-service", Version: "1.0.0"}}
app, err := bootstrap.NewApp(cfg)
if err != nil {
    return err
}

_ = app.RegisterComponent(db)    // component.Component
_ = app.RegisterComponent(cache) // component.Component

app.OnConfigure(func(ctx context.Context, app *bootstrap.App[*AppConfig]) error {
    // All components started — set up routes, handlers, business logic
    return nil
})

// Run: Init → Start → Configure → Ready → wait for signal → Stop
if err := app.Run(ctx); err != nil {
    log.Fatal("app failed", map[string]any{"error": err})
}
```

## Agent Loop

```go
import (
    "context"
    "github.com/kbukum/gokit/agent"
    "github.com/kbukum/gokit/ai/chat"
    "github.com/kbukum/gokit/llm"
    "github.com/kbukum/gokit/llm/providers/ollama"
)

registry := llm.NewDialectRegistry()
_ = ollama.Register(registry)

adapter, err := llm.New(registry, llm.Config{
    Dialect: ollama.DialectName,
    BaseURL: ollama.DefaultBaseURL,
    Model:   "llama3.2",
})
if err != nil {
    return err
}

runner := agent.New(agent.Config{
    Provider: llm.NewProvider(adapter, "llama3.2"),
    Model:    "llama3.2",
})
result, err := runner.Run(context.Background(), []chat.Message{
    chat.User("What's the weather in Berlin?"),
})
fmt.Println(result.FinalMessage.Text())
```

## LLM Chat Completion

```go
import (
    "github.com/kbukum/gokit/ai/chat"
    "github.com/kbukum/gokit/llm"
    "github.com/kbukum/gokit/llm/providers/ollama"
)

registry := llm.NewDialectRegistry()
_ = ollama.Register(registry)

adapter, err := llm.New(registry, llm.Config{
    Dialect: ollama.DialectName,
    BaseURL: ollama.DefaultBaseURL,
    Model:   "llama3.2",
})

resp, err := adapter.Execute(ctx, llm.CompletionRequest{
    Messages: []chat.Message{
        chat.User("Explain circuit breakers"),
    },
})
fmt.Println(resp.Text())
```

## Tool Definition

```go
import (
    "encoding/json"
    "github.com/kbukum/gokit/tool"
)

type WeatherInput struct {
    City string `json:"city" jsonschema:"required"`
}

type WeatherOutput struct {
    Summary string `json:"summary"`
}

t := tool.FromFunc("get_weather", "Get current weather for a city",
    func(ctx context.Context, input WeatherInput) (WeatherOutput, error) {
        return WeatherOutput{Summary: "sunny in " + input.City}, nil
    })

registry := tool.NewRegistry()
_ = registry.Register(t)

result, err := registry.Call(tool.NewContext(ctx), "get_weather", json.RawMessage(`{"city":"Berlin"}`))
fmt.Println(result.Text())
```

## Messaging

```go
import (
    "github.com/kbukum/gokit/messaging"
    "github.com/kbukum/gokit/messaging/memory"
)

broker := memory.NewBroker()
producer := broker.Producer()
consumer := broker.Consumer("events")

event, _ := messaging.NewEvent("user.created", "auth-service", map[string]string{"id": "user-123"}, "user-123")
_ = producer.Publish(ctx, "events", event)

go consumer.Consume(ctx, func(ctx context.Context, msg messaging.Message) error {
    return processEvent(msg)
})
```

## Object Storage

```go
import (
    "github.com/kbukum/gokit/storage"
    local "github.com/kbukum/gokit/storage/local"
)

reg := storage.NewFactoryRegistry()
_ = local.Register(reg, local.Config{BasePath: "./data"})

store, _ := storage.New(reg, storage.Config{
    Provider: storage.ProviderLocal,
    Enabled:  true,
}, log)
_ = store.Put(ctx, "uploads/report.pdf", reader)
rc, _ := store.Get(ctx, "uploads/report.pdf")
defer rc.Close()
```
