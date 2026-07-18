package sample

import (
	"github.com/kbukum/gokit/dataset/payload"
	"github.com/kbukum/gokit/dataset/stage"
)

// Item is a blob dataset item: a named payload tagged real or AI
// and carrying the source offset it was fetched at. It implements [stage.Labeled]
// and [stage.Offsetted] so the generic collector aggregates real/AI stats
// and resumes partial runs by offset.
type Item struct {
	name    string
	label   stage.Label
	offset  int
	payload payload.Payload
}

// New returns an item with the given name, label, offset, and payload.
func New(name string, label stage.Label, offset int, p payload.Payload) Item {
	return Item{name: name, label: label, offset: offset, payload: p}
}

// Name returns the item's file name.
func (i Item) Name() string { return i.name }

// Label reports whether the item is real or AI.
func (i Item) Label() stage.Label { return i.label }

// SourceOffset returns the item's offset within its source.
func (i Item) SourceOffset() (int, bool) { return i.offset, true }

// Payload returns the item's bounded byte payload.
func (i Item) Payload() payload.Payload { return i.payload }
