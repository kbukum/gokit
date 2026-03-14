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

### Optional Nodes & Error Policies

Nodes can be marked `optional` — if the component is not registered, a placeholder
is inserted that returns `ErrUnavailable`. The engine skips dependents per-cycle
without removing them from the graph.

```yaml
nodes:
  - component: ser
    optional: true                              # placeholder if not registered
    on_error: skip                              # skip | continue | fail
    schedule: { interval_sec: 3, min_buffer_sec: 1 }
  - component: signal_compositor
    depends_on: [ser]
  - component: notes
    depends_on: [signal_compositor]
```

| `on_error` | Behavior |
|------------|----------|
| `skip` (default) | Dependents are skipped this cycle |
| `continue` | Dependents run regardless |
| `fail` | Halt the entire pipeline |

### Schedule Config

Schedules use integer seconds in YAML for readability:

```yaml
schedule: { interval_sec: 30, min_buffer_sec: 15 }
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

## Cascade: Staged Execution Pipeline

For multi-stage processing where each stage is a sub-DAG with its own
configuration, conditional advancement, and early exit.

```go
cascade := dag.NewCascade[Input, Result]().

    // Stage 1: cheap, fast checks
    Stage("quick-check", func(b *dag.StageBuilder[Input, Result], input Input) {
        b.AddNode("metadata", metadataAnalyzer)
        b.Timeout(time.Second)
        b.AdvanceWhen(func(r Result) bool {
            return r.Confidence < 0.95 // advance only if uncertain
        })
    }).

    // Stage 2: deeper analysis (parallel within stage)
    Stage("deep-analysis", func(b *dag.StageBuilder[Input, Result], input Input) {
        b.AddNode("frequency", frequencyAnalyzer)
        b.AddNode("statistical", statisticalAnalyzer)
        // No edges = parallel execution within stage
        b.Timeout(5 * time.Second)
    }).

    // Final stage: always runs with all accumulated results
    FinalStage("fusion", func(b *dag.StageBuilder[Input, Result], input Input) {
        b.AddNode("fusion", fusionEngine)
    }).

    // Global configuration
    MergeStrategy(mergeResults).
    OrderNodesBy(dag.OrderByCost()).
    MaxConcurrency(4).
    OnStageFailure(dag.SkipToFinal()).
    Build()

result, trace := cascade.Execute(ctx, input)
fmt.Println(trace.StagesExecuted) // ["quick-check"]
fmt.Println(trace.EarlyExit)     // true — confident at stage 1
```

### Cascade Features

- **Dynamic node composition** — stage builder receives input, adds nodes conditionally
- **Advance conditions** — `AdvanceWhen` decides whether to continue to next stage
- **Internal edges** — dependency ordering within a stage (`b.Edge("a", "b")`)
- **Per-stage timeout** — independent timeout for each stage
- **Node failure policies** — `ContinueWithPartial()` or abort on node failure
- **Stage failure policies** — `Abort()`, `SkipToFinal()`, or `ContinueOnFailure()`
- **Ordering strategies** — `OrderByCost()`, `OrderByLatency()`, `WeightedScore(weights)` using `provider.Meta`
- **Execution trace** — `CascadeTrace` with per-node timing, cost aggregation, stages executed/skipped

### Ordering Strategies

OrderBy affects scheduling when multiple nodes are ready simultaneously
and resources are constrained (`MaxConcurrency`). Strategies read `provider.Meta`:

```go
// Cheapest nodes first (reads "cost" from provider.Meta)
OrderNodesBy(dag.OrderByCost())

// Fastest nodes first (reads "latency_ms" from provider.Meta)
OrderNodesBy(dag.OrderByLatency())

// Multi-objective weighted scoring
OrderNodesBy(dag.WeightedScore(map[string]float64{
    "cost": 0.5, "latency_ms": 0.3, "reliability": 0.2,
}))
```

## Key Types & Functions

| Name | Description |
|------|-------------|
| `State` | Thread-safe key-value store for passing data between nodes |
| `Port[T]` | Compile-time typed accessor for State |
| `Read` / `TryRead` / `Write` | Typed state access via ports |
| `Node` | Interface: `Name()` + `Run(ctx, state)` |
| `FromProvider[I,O]()` | Bridges `provider.RequestResponse[I,O]` into a Node |
| `NewUnavailableNode()` | Placeholder node that returns `ErrUnavailable` |
| `Graph` / `Edge` | Declares nodes, edges, and node metadata (`NodeDefs`) |
| `BuildLevels()` | Kahn's algorithm — groups nodes by dependency level |
| `Engine` | Executes graphs via `ExecuteBatch()` or `ExecuteStreaming()` |
| `AsTool[I,O]()` | Wraps a DAG pipeline as `provider.RequestResponse[I,O]` |
| `Registry` | Named node lookup for dynamic graph construction |
| `Pipeline` / `NodeDef` | YAML-defined graph definitions with includes, optional, on_error |
| `Session` | Per-session state and schedule tracking for streaming mode |
| `NewCascade[I,O]()` | Creates a staged execution pipeline builder |
| `StageBuilder[I,O]` | Per-stage config: AddNode, Edge, Timeout, AdvanceWhen, OnFailure |
| `Cascade[I,O]` | Executable staged pipeline with `Execute(ctx, input)` |
| `CascadeTrace` | Execution details: stages, node results, cost, early exit |
| `OrderByCost` / `OrderByLatency` / `WeightedScore` | Node ordering strategies |
| `WithTracing` / `WithMetrics` / `WithLogging` | Observability node wrappers |

---

[⬅ Back to main README](../README.md)
