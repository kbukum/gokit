//go:build !windows

package process

import "testing"

func TestSignalsRejectNonpositivePID(t *testing.T) {
	t.Parallel()
	for _, pid := range []int{0, -1} {
		for _, group := range []bool{false, true} {
			if err := signalPID(pid, 0, group); err == nil {
				t.Fatalf("accepted invalid signal target pid=%d group=%t", pid, group)
			}
		}
	}
}
