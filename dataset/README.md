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
| [`record`](record) | `record.New` and the sorted-key `Record`; `ReadCSV`/`ReadJSONArray`/`ReadJSONLines`, `WriteCSV`/`WriteJSONArray`/`WriteJSONLines`, low-level `ParseCSV`/…, `Filter`/`SelectColumns`, and the `FileSource`/`FileTarget` record item family |
| [`sample`](sample) | the blob item family: labeled, offset-carrying `sample.Item` over a bounded payload; `NewSliceSource`/`NewDirSource` and a real/AI-splitting `LocalTarget` |
| [`schema`](schema) | `schema.Compile`/`schema.JSON` and `Schema.Validate` — fail-closed on any non-conforming record — plus `Schema.Validator()` adapting it to a `stage.Validator[record.Record]` |
| [`stage`](stage) | generic `Source[T]`, `Transform[I,O]`, `Target[T]`, the opt-in capability model (`Keyed`/`Bounded`/`Labeled`/`Offsetted`/`Resumable`), the pluggable `Validator[T]`, and the `CacheKey`/`MaxItems`/`LabelOf`/`OffsetOf`/`Resume` helpers |
| [`manifest`](manifest) | `manifest.New`/`manifest.Load` cache with `SourceEntry`, `SourceStats`, and the canonical `CacheStatusFor` query |
| [`collect`](collect) | the generic `collect.Collector[T]` engine: a bounded worker pool with `StreamBuffer` backpressure, per-source timeout/cancel, offset resume, real/AI stats, and a pluggable validator, configured via `Config`, `Result`, and a `Progress`/`NullProgress` callback with injected clock |

## Bounds & safety

- Every read and payload is bounded by `payload.Limits` (8 MiB in-memory cap,
  64-record stream buffer by default); oversized payloads spill to a file rather
  than being held in memory.
- Untrusted CSV/JSON records and schema validation **fail closed**; `record`'s
  readers and `Schema.Validate` have `Fuzz` targets.
- The collector streams sources through a bounded worker pool: the work and event
  channels are sized by `Limits.StreamBuffer` for backpressure, each source is
  bounded by `Config.SourceTimeout` and `context.Context` cancellation, and a
  resumable source that made partial progress is recorded as `partial` so a later
  run continues it from its offset.

## Quick start

```go
target := stage.NewSliceTarget[record.Record]("mem")
c := collect.New(
    collect.WithSources(stage.NewSliceSource("people", records)),
    collect.WithValidator(personSchema.Validator()),
    collect.WithTargets[record.Record](target),
    collect.WithConfig[record.Record](collect.Config{OutputDir: "build"}),
)
result, err := c.Run(ctx)
```
