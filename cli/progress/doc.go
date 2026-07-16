// Package progress renders progress bars and spinners over an injected writer.
//
// It is the light, dependency-free counterpart to a full progress library: a
// [Bar] shows determinate work ("[####----] 4/10 40%") and a [Spinner] shows
// indeterminate work by cycling frames. Both write to an injected [io.Writer]
// and advance only when the caller updates them — position for the bar, an
// explicit tick for the spinner — so rendering is deterministic and needs no
// real clock or background goroutine. Styling flows through an optional
// [github.com/kbukum/gokit/cli/theme.Palette].
package progress
