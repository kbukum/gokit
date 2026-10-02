package sse

import (
	"strings"
	"testing"
)

func TestDecoderExactSmallLineLimits(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{8, 1024, 4096} {
		for _, ending := range []string{"\n", "\r", "\r\n"} {
			for _, extra := range []int{0, 1} {
				line := "data:" + strings.Repeat("x", limit-5+extra)
				decoder := NewDecoder(strings.NewReader(line+ending+ending), WithMaxLineSize(limit))
				event, err := decoder.Next()
				if extra == 0 && (err != nil || len(event.Data) != limit-5) {
					t.Fatalf("exact %d-byte line: %v", limit, err)
				}
				if extra == 1 && err == nil {
					t.Fatalf("accepted %d-byte line with limit %d", len(line), limit)
				}
			}
		}
	}
}
