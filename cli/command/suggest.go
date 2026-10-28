package command

import (
	"slices"
	"strings"
)

// suggest returns the known words closest to word: those within an edit distance of a third of its length (at least 1, at most 3), or that start with it.
func suggest(word string, known []string) []string {
	limit := min(max(len(word)/3, 1), 3)
	best := limit + 1
	var out []string
	for _, k := range known {
		d := distance(word, k)
		if d > limit && len(word) >= 2 && strings.HasPrefix(k, word) {
			d = limit
		}
		if d > limit {
			continue
		}
		if d < best {
			best, out = d, out[:0]
		}
		if d <= best && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return out
}

// distance is the optimal string alignment distance: insertions, deletions, substitutions and swaps of adjacent letters each count once.
func distance(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	rows := make([][]int, len(ra)+1)
	for i := range rows {
		rows[i] = make([]int, len(rb)+1)
		rows[i][0] = i
	}
	for j := range rows[0] {
		rows[0][j] = j
	}
	for i := 1; i <= len(ra); i++ {
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			rows[i][j] = min(rows[i-1][j]+1, rows[i][j-1]+1, rows[i-1][j-1]+cost)
			if i > 1 && j > 1 && ra[i-1] == rb[j-2] && ra[i-2] == rb[j-1] {
				rows[i][j] = min(rows[i][j], rows[i-2][j-2]+1)
			}
		}
	}
	return rows[len(ra)][len(rb)]
}
