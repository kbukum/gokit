// Package sample is the blob item family of the dataset kit: a labeled,
// offset-carrying [Item] whose bytes live in a bounded
// [github.com/kbukum/gokit/dataset/payload] payload, plus the sources and
// target that stream it through the generic collector. It is the gokit analog
// of rskit's blob DataItem, kept light: item bytes are read and written through
// the payload owner and confined to the output directory through
// [github.com/kbukum/gokit/fs] path safety.
//
// It is split by concern:
//
//   - item.go — the [Item] value and its capability implementations.
//   - source.go — in-memory and directory sources producing items.
//   - target.go — [LocalTarget], which writes items to real/ and ai/
//     subdirectories by label.
package sample
