package util

import (
	"fmt"
	"strings"
)

// Ambiguity carries the candidates a shorthand matched when more than one qualified.
type Ambiguity[T any] struct {
	// Input is the shorthand that matched more than one candidate.
	Input string
	// Matches are every candidate whose key equalled Input, in encounter order.
	Matches []T
}

// Error implements error, rendering an actionable "did you mean one of …?" message.
func (a *Ambiguity[T]) Error() string {
	var b strings.Builder
	b.WriteString("'")
	b.WriteString(a.Input)
	b.WriteString("' is ambiguous; matched ")
	for i, m := range a.Matches {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprint(&b, m)
	}
	return b.String()
}

// ResolveUnique resolves input to the unique candidate whose key equals it.
//
// keyOf projects each candidate to the string compared against input. Comparison
// is exact and case-sensitive. It returns (candidate, true, nil) when exactly one
// candidate matches, (zero, false, nil) when none do, and (zero, false, *Ambiguity)
// when two or more share the key. Callers wanting fuzzy resolution use Nearest.
func ResolveUnique[T any](input string, candidates []T, keyOf func(T) string) (match T, found bool, err error) {
	var matches []T
	for _, candidate := range candidates {
		if keyOf(candidate) == input {
			matches = append(matches, candidate)
		}
	}
	switch len(matches) {
	case 0:
		var zero T
		return zero, false, nil
	case 1:
		return matches[0], true, nil
	default:
		var zero T
		return zero, false, &Ambiguity[T]{Input: input, Matches: matches}
	}
}
