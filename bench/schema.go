package bench

// The schema version and URL are a shared cross-kit contract: the sibling rskit
// bench module (core/rskit-bench/src/schema.rs) emits the identical values so a
// gokit-produced and an rskit-produced result are interchangeable. Any change
// here must be mirrored in rskit in the same step, including the metric
// direction field spelling (see direction.go).

// SchemaVersion is the current Bench JSON schema version.
const SchemaVersion = "1.0"

// SchemaURL is the schema URL for Bench JSON output.
const SchemaURL = "https://gokit.dev/bench/v1/schema.json"
