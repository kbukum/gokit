# dataset

A streaming dataset-collection framework: bounded sources feed typed transforms
and targets, with fail-closed schema validation, a manifest/cache layer, and
spill-to-file for oversized payloads. It is the cross-kit Go mirror of rskit's
`rskit-dataset`, kept **light**: heavy image/media transforms stay rskit-only by
design.

`dataset` is a sub-module (`github.com/kbukum/gokit/dataset`) so it can reuse the
`schema` sub-module. It builds on gokit's canonical owners rather than a parallel
stack — record streams are [`stream`](../stream) pipelines, bounded file reads go
through [`fs`](../fs), and validation reuses [`schema`](../schema).

It is split into focused sub-packages by concern:

| Package | Surface |
|---|---|
| [`payload`](payload) | `Payload`/`Item` with an in-memory cap and file spill; `FromBytes`, `FromFile`, and the bounding `Limits` (`WithDefaults`) |
| [`record`](record) | `record.New` and the sorted-key `Record` (`Get`, `Select`, `Fields`, `ToJSON`); `ReadCSV`/`ReadJSONArray`/`ReadJSONLines`, `WriteCSV`/`WriteJSONArray`/`WriteJSONLines`, low-level `ParseCSV`/`ParseJSONArray`/`ParseJSONLines`, and `Filter`/`SelectColumns` |
| [`schema`](schema) | `schema.Compile`/`schema.JSON` and `Schema.Validate` — fail-closed on any non-conforming record |
| [`stage`](stage) | generic `Source[T]`, `Transform[I,O]`, `Target[T]` (opt-in `Keyed`/`Bounded` sources), `ApplyTransform`, and the `CacheKey`/`MaxItems` helpers |
| [`manifest`](manifest) | `manifest.New`/`manifest.Load` cache with `SourceEntry`, `SourceStats`, `CacheStatus` |
| [`collect`](collect) | `collect.New` `Collector` orchestrating sources, transforms, validation, caching, and publishing via `Config`, `Result`, and a `Progress`/`NullProgress` callback with injected clock |

## Bounds & safety

- Every read and payload is bounded by `payload.Limits` (8 MiB in-memory cap,
  64-record stream buffer by default); oversized payloads spill to a file rather
  than being held in memory.
- Untrusted CSV/JSON records and schema validation **fail closed**; `record`'s
  readers and `Schema.Validate` have `Fuzz` targets.
- Sources stream lazily and stop on `context.Context` cancellation; the collector
  bounds each source with `Config.SourceTimeout`.

## Quick start

```go
target := stage.NewSliceTarget[record.Record]("mem")
c := collect.New(
    collect.WithSources(stage.NewSliceSource("people", records)),
    collect.WithSchema(personSchema),
    collect.WithTargets(target),
    collect.WithConfig(collect.Config{OutputDir: "build"}),
)
result, err := c.Run(ctx)
```
