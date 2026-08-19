package fs

import "sort"

// FsChangeBatch is a coalesced set of filesystem changes observed within one debounce
// window. Paths are deduplicated and sorted. RescanRequested reports whether the
// underlying watcher dropped events (typically a queue overflow), signaling that the
// consumer should re-evaluate the tree rather than trust the path set alone.
type FsChangeBatch struct {
	paths  []string
	rescan bool
}

// newFsChangeBatch builds a batch from a set of changed paths and a rescan flag,
// returning a sorted, deduplicated path list.
func newFsChangeBatch(pathSet map[string]struct{}, rescan bool) FsChangeBatch {
	paths := make([]string, 0, len(pathSet))
	for path := range pathSet {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	return FsChangeBatch{paths: paths, rescan: rescan}
}

// Paths returns the sorted, deduplicated changed paths.
func (b FsChangeBatch) Paths() []string { return b.paths }

// RescanRequested reports whether a dropped-event signal was seen during the window.
func (b FsChangeBatch) RescanRequested() bool { return b.rescan }

// Any reports whether any changed path satisfies predicate.
func (b FsChangeBatch) Any(predicate func(string) bool) bool {
	for _, path := range b.paths {
		if predicate(path) {
			return true
		}
	}
	return false
}

// Len returns the number of changed paths.
func (b FsChangeBatch) Len() int { return len(b.paths) }

// IsEmpty reports whether the batch carries neither a changed path nor a rescan
// signal, so a rescan-only batch is not empty.
func (b FsChangeBatch) IsEmpty() bool {
	return len(b.paths) == 0 && !b.rescan
}
