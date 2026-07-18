// Package codec provides pluggable structured-text codecs over a shared value tree.
//
// A [Codec] encodes
// and decodes one on-disk/text format (JSON, TOML, …) through a single canonical in-memory model,
// [Value]. Any package that reads or writes a config file, manifest,
// or document reuses these codecs instead of re-implementing "bounded read → parse → typed error" per format.
//
// # Value model
//
// [Value] is the canonical format-neutral tree produced by decoding JSON into an untyped Go value:
// nested map[string]any, []any, float64, string, bool, and nil. It is a deliberate,
// documented opaque-value exception to the no-any rule —
// the tree's leaf values are genuinely heterogeneous and cannot be given a closed type.
// Formats without a JSON equivalent (notably TOML datetimes) are not part of the model;
// represent such values as strings.
//
// # Runtime selection
//
// [Codec] is an interface, so a codec can be selected at runtime —
// for example by file extension via [CodecForPath]. The generic conveniences [Encode]
// and [Decode] take a [Codec] and any Go value, routing it through the value tree
// so callers can use ordinary structs and slices without touching [Value] directly.
//
// # Framing
//
// The github.com/kbukum/gokit/codec/framing subpackage carries one codec-encoded value per length-delimited frame over any blocking io.Reader/io.Writer,
// with every read bounded so a hostile peer cannot force an unbounded allocation.
package codec
