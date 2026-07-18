// Package value provides operations over the canonical codec value tree.
package value

// ArrayStrategy chooses how two arrays found at the same key combine during a merge.
// The strategy is selected per key by the caller (see [MergeWith]);
// the merge mechanism itself is policy-free and does not know what any key "means".
type ArrayStrategy int

const (
	// Replace makes the overlay array replace the base array wholesale (last-wins). It is the default.
	Replace ArrayStrategy = iota
	// Concat appends the overlay array to the base array (concatenation).
	Concat
)

// Merge deep-merges overlay onto base, replacing arrays wholesale.
//
// Objects (map[string]any) merge recursively;
// on a key collision the overlay value wins (last-wins scalars).
// Every array ([]any) is replaced by the overlay.
// Use [MergeWith] to concatenate selected arrays instead. Neither input is mutated.
func Merge(base, overlay any) any {
	return MergeWith(base, overlay, func(string) ArrayStrategy { return Replace })
}

// MergeWith deep-merges overlay onto base, choosing an array strategy per key.
//
// Objects merge recursively; on a key collision the overlay value wins.
// When both sides hold an array at the same key,
// arrayStrategy is consulted with that key to decide [Replace] vs [Concat].
// Type mismatches (for example object vs scalar) resolve to the overlay. Neither input is mutated.
func MergeWith(base, overlay any, arrayStrategy func(key string) ArrayStrategy) any {
	return merger{arrayStrategy: arrayStrategy}.merge(base, overlay, "", false)
}

// merger carries the array-merge strategy across the recursive merge.
type merger struct {
	arrayStrategy func(string) ArrayStrategy
}

func (m merger) merge(base, overlay any, key string, keyed bool) any {
	baseObj, baseIsObj := base.(map[string]any)
	overlayObj, overlayIsObj := overlay.(map[string]any)
	if baseIsObj && overlayIsObj {
		merged := make(map[string]any, len(baseObj)+len(overlayObj))
		for k, v := range baseObj {
			merged[k] = v
		}
		for k, ov := range overlayObj {
			if bv, ok := merged[k]; ok {
				merged[k] = m.merge(bv, ov, k, true)
			} else {
				merged[k] = ov
			}
		}
		return merged
	}

	baseArr, baseIsArr := base.([]any)
	overlayArr, overlayIsArr := overlay.([]any)
	if baseIsArr && overlayIsArr {
		strategy := Replace
		if keyed {
			strategy = m.arrayStrategy(key)
		}
		if strategy == Concat {
			out := make([]any, 0, len(baseArr)+len(overlayArr))
			out = append(out, baseArr...)
			out = append(out, overlayArr...)
			return out
		}
		return overlayArr
	}

	return overlay
}
