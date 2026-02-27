# dag

DAG (Directed Acyclic Graph) execution engine for orchestrating provider-based service calls in dependency order.

## Install

```bash
go get github.com/kbukum/gokit
```

## Quick Start

```go
package main

import (
    "context"
    "fmt"
    "github.com/kbukum/gokit/dag"
)

func main() {
    // Define typed ports for compile-time safety
    rawPort := dag.Port[string]{Key: "raw"}
    resultPort := dag.Port[string]{Key: "result"}

    // Create nodes (using func nodes for brevity — real code uses FromProvider)
    extract := myExtractNode(rawPort)
    transform := myTransformNode(rawPort, resultPort)

    // Build graph with dependencies
    g := &dag.Graph{
        Nodes: map[string]dag.Node{
            "extract":   extract,
            "transform": transform,
        },
        Edges: []dag.Edge{{From: "extract", To: "transform"}},
    }

    // Execute in dependency order
    engine := &dag.Engine{}
    state := dag.NewState()
    result, _ := engine.ExecuteBatch(context.Background(), g, state)

    out, _ := dag.Read(state, resultPort)
    fmt.Println(out, result.Duration)
}
```

## Provider Integration

Every node wraps a `provider.RequestResponse[I,O]`. All existing provider middleware applies per-node:

```go
// Wrap a provider as a DAG node
svc := provider.WithResilience(rawSvc, resilience.Config{...})
node := dag.FromProvider(dag.NodeConfig[MyInput, *MyOutput]{
    Name:    "my-service",
    Service: svc,
    Extract: func(state *dag.State) (MyInput, error) {
        return dag.Read(state, inputPort)
    },
    Output: outputPort,
})
```

## Pipeline as Provider

Wrap a DAG pipeline as a `provider.RequestResponse` for composability:

```go
tool := dag.AsTool[Input, Output](engine, graph, dag.ToolConfig[Input, Output]{
    Name:    "my-pipeline",
    InputFn: func(in Input, state *dag.State) { dag.Write(state, inPort, in) },
    OutputFn: func(state *dag.State) (Output, error) { return dag.Read(state, outPort) },
})

// Now callable like any other provider
result, err := tool.Execute(ctx, myInput)
```

## YAML Pipeline Definitions

```yaml
name: full-process
mode: batch
includes:
  - data-enrichment
nodes:
  - component: extract
  - component: transform
    depends_on: [extract]
  - component: load
    depends_on: [transform]
```

```go
loader := dag.NewFilePipelineLoader("./pipelines")
pipeline, _ := loader.Load("full-process")
graph, _ := dag.ResolvePipeline(pipeline, registry, loader)
```

## Streaming Mode

For long-running sessions where nodes fire on different schedules:

```go
sess := dag.NewSession("session-1")
conditions := map[string]dag.ConditionFunc{
    "has-data": func(state *dag.State) bool {
        _, ok := state.Get("data")
        return ok
    },
}

filter := sess.ReadyFilter(pipeline, conditions)
result, _ := engine.ExecuteStreaming(ctx, graph, sess.State, filter)
```

## Observability

Per-node tracing, metrics, and logging:

```go
node = dag.WithTracing(node, "my-pipeline")
node = dag.WithMetrics(node, metrics)
node = dag.WithLogging(node, log)
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `State` | Thread-safe key-value store for passing data between nodes |
| `Port[T]` | Compile-time typed accessor for State |
| `Node` | Interface: `Name()` + `Run(ctx, state)` |
| `FromProvider[I,O]()` | Bridges `provider.RequestResponse[I,O]` into a Node |
| `Graph` / `Edge` | Declares nodes and dependency relationships |
| `BuildLevels()` | Kahn's algorithm — groups nodes by dependency level |
| `Engine` | Executes graphs via `ExecuteBatch()` or `ExecuteStreaming()` |
| `AsTool[I,O]()` | Wraps a DAG pipeline as `provider.RequestResponse[I,O]` |
| `Registry` | Named node lookup for dynamic graph construction |
| `Pipeline` / `NodeDef` | YAML-defined graph definitions with includes |
| `Session` | Per-session state and schedule tracking for streaming mode |
| `WithTracing` / `WithMetrics` / `WithLogging` | Observability node wrappers |

---

[⬅ Back to main README](../README.md)
