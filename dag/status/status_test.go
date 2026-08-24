package status_test

import (
	"testing"

	"github.com/kbukum/gokit/dag/status"
)

func TestStatusString(t *testing.T) {
	t.Parallel()
	cases := map[status.Status]string{
		status.Completed:      "completed",
		status.Failed:         "failed",
		status.Skipped:        "skipped",
		status.Unavailable:    "unavailable",
		status.DepUnavailable: "skipped:dep_unavailable",
		status.DepFailed:      "skipped:dep_failed",
		status.DepSkipped:     "skipped:dep_not_ready",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("Status(%q).String() = %q, want %q", string(s), got, want)
		}
	}
}

func TestStatusClassification(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name                       string
		s                          status.Status
		terminal, skipped, success bool
	}{
		{"completed", status.Completed, true, false, true},
		{"failed", status.Failed, true, false, false},
		{"skipped", status.Skipped, false, true, false},
		{"unavailable", status.Unavailable, false, false, false},
		{"dep_unavailable", status.DepUnavailable, false, true, false},
		{"dep_failed", status.DepFailed, false, true, false},
		{"dep_skipped", status.DepSkipped, false, true, false},
		{"unknown", status.Status("bogus"), false, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.s.IsTerminal(); got != tt.terminal {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.terminal)
			}
			if got := tt.s.IsSkipped(); got != tt.skipped {
				t.Errorf("IsSkipped() = %v, want %v", got, tt.skipped)
			}
			if got := tt.s.IsSuccess(); got != tt.success {
				t.Errorf("IsSuccess() = %v, want %v", got, tt.success)
			}
		})
	}
}
