// Package record owns the dataset kit's row vocabulary and its codecs: the [Record] type plus CSV,
// JSON-array, and JSON-lines readers and writers, and the [Filter]/[SelectColumns] combinators.
//
// Readers stream bounded file reads through [github.com/kbukum/gokit/fs]
// and emit [github.com/kbukum/gokit/stream] pipelines; they fail closed on malformed
// or oversized input. Writers drain a pipeline to an injected io.Writer.
//
// A record field [Value] is a decoded-JSON leaf; its any element type is a deliberate,
// documented exception to the no-any rule, matching [github.com/kbukum/gokit/codec] Value
// and schema.JSON.
package record
