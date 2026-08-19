package util

import "strings"

// DefaultSuggestionDistance is the default maximum edit distance for a suggestion
// to be offered. Two catches single transpositions, one insertion plus one deletion,
// and most realistic typos while rejecting unrelated tokens.
const DefaultSuggestionDistance = 2

// Nearest returns the candidate nearest to input within DefaultSuggestionDistance,
// or "" and false when none is close enough.
//
//	Nearest("fmt", []string{"format", "test", "lint"}) == ("format", true)
func Nearest(input string, candidates []string) (string, bool) {
	return NearestWithin(input, candidates, DefaultSuggestionDistance)
}

// NearestWithin returns the candidate nearest to input within maxDistance edits.
//
// Matching is case-insensitive. A candidate qualifies either by an Optimal String
// Alignment (restricted Damerau-Levenshtein) distance within maxDistance — counting
// an adjacent transposition as a single edit — or, as a fallback, by being an
// abbreviation of input (input of at least two characters is a subsequence of a
// candidate no more than four times its length, e.g. "fmt" → "format"). Ties break
// toward a candidate sharing input's leading character, then lexicographically, so
// the result is deterministic regardless of iteration order.
func NearestWithin(input string, candidates []string, maxDistance int) (string, bool) {
	lowerInput := strings.ToLower(input)
	var best string
	bestScore := -1
	for _, candidate := range candidates {
		lowerCandidate := strings.ToLower(candidate)
		score, ok := matchScore(lowerInput, lowerCandidate, maxDistance)
		if !ok {
			continue
		}
		if bestScore == -1 || score < bestScore ||
			(score == bestScore && prefers(candidate, best, lowerInput)) {
			best = candidate
			bestScore = score
		}
	}
	return best, bestScore != -1
}

// matchScore scores a candidate against input, or reports not-close-enough.
func matchScore(input, candidate string, maxDistance int) (int, bool) {
	inputLen := len([]rune(input))
	candidateLen := len([]rune(candidate))
	if absDiff(inputLen, candidateLen) <= maxDistance {
		distance := osaDistance(input, candidate)
		if distance <= maxDistance {
			return distance, true
		}
	}
	if inputLen >= 2 && candidateLen <= inputLen*4 && isSubsequence(input, candidate) {
		return maxDistance, true
	}
	return 0, false
}

// isSubsequence reports whether every rune of needle appears in haystack in order.
func isSubsequence(needle, haystack string) bool {
	hay := []rune(haystack)
	i := 0
	for _, target := range needle {
		found := false
		for i < len(hay) {
			c := hay[i]
			i++
			if c == target {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// prefers is the tie-break preference: favor a candidate sharing input's leading
// character, then the lexicographically smaller name.
func prefers(candidate, incumbent, lowerInput string) bool {
	leading, hasLeading := firstRune(lowerInput)
	candidatePrefix := hasLeading && leadingCharMatches(candidate, leading)
	incumbentPrefix := hasLeading && leadingCharMatches(incumbent, leading)
	switch {
	case candidatePrefix && !incumbentPrefix:
		return true
	case !candidatePrefix && incumbentPrefix:
		return false
	default:
		return candidate < incumbent
	}
}

func leadingCharMatches(s string, leading rune) bool {
	first, ok := firstRune(s)
	if !ok {
		return false
	}
	return strings.EqualFold(string(first), string(leading))
}

func firstRune(s string) (rune, bool) {
	for _, r := range s {
		return r, true
	}
	return 0, false
}

func absDiff(a, b int) int {
	if a > b {
		return a - b
	}
	return b - a
}

// osaDistance is the Optimal String Alignment distance between two strings, where
// an adjacent transposition costs one edit and no substring is edited twice.
func osaDistance(a, b string) int {
	ar := []rune(a)
	br := []rune(b)
	if len(ar) == 0 {
		return len(br)
	}
	if len(br) == 0 {
		return len(ar)
	}
	cols := len(br) + 1
	twoPrev := make([]int, cols)
	prev := make([]int, cols)
	current := make([]int, cols)
	for j := 0; j < cols; j++ {
		prev[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		current[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 0
			if ar[i-1] != br[j-1] {
				cost = 1
			}
			value := min(prev[j]+1, min(current[j-1]+1, prev[j-1]+cost))
			if i > 1 && j > 1 && ar[i-1] == br[j-2] && ar[i-2] == br[j-1] {
				value = min(value, twoPrev[j-2]+1)
			}
			current[j] = value
		}
		twoPrev, prev, current = prev, current, twoPrev
	}
	return prev[len(br)]
}
