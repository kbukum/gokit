package render

import (
	"strings"
	"unicode/utf8"
)

// OutputKV is a key-value display block for headers and summaries.
//
// Keys are right-aligned to the widest key so values line up in a column.
// Construct one with [NewOutputKV]; rendering is pure via [OutputKV.String].
type OutputKV struct {
	keys   []string
	values []string
}

// NewOutputKV creates an empty key-value output block.
func NewOutputKV() *OutputKV {
	return &OutputKV{}
}

// Add appends a key-value pair and returns the receiver.
func (k *OutputKV) Add(key, value string) *OutputKV {
	k.keys = append(k.keys, key)
	k.values = append(k.values, value)
	return k
}

// String renders each pair as an indented, right-aligned "key:  value" line.
func (k *OutputKV) String() string {
	maxKey := 0
	for _, key := range k.keys {
		maxKey = max(maxKey, utf8.RuneCountInString(key))
	}
	var b strings.Builder
	for i, key := range k.keys {
		pad := maxKey - utf8.RuneCountInString(key)
		if pad < 0 {
			pad = 0
		}
		b.WriteString("  ")
		b.WriteString(strings.Repeat(" ", pad))
		b.WriteString(key)
		b.WriteString(":  ")
		b.WriteString(k.values[i])
		b.WriteByte('\n')
	}
	return b.String()
}
