package dag

import (
	"testing"

	"github.com/kbukum/gokit/dag/status"
)

func TestNodeResult_Helpers(t *testing.T) {
	tests := []struct {
		status     status.Status
		isTerminal bool
		isSkipped  bool
		isSuccess  bool
	}{
		{status.Completed, true, false, true},
		{status.Failed, true, false, false},
		{status.Skipped, false, true, false},
		{status.Unavailable, false, false, false},
		{status.DepUnavailable, false, true, false},
		{status.DepFailed, false, true, false},
	}

	for _, tt := range tests {
		nr := NodeResult{Status: tt.status}
		if nr.IsTerminal() != tt.isTerminal {
			t.Errorf("%s: IsTerminal=%v, want %v", tt.status, nr.IsTerminal(), tt.isTerminal)
		}
		if nr.IsSkipped() != tt.isSkipped {
			t.Errorf("%s: IsSkipped=%v, want %v", tt.status, nr.IsSkipped(), tt.isSkipped)
		}
		if nr.IsSuccess() != tt.isSuccess {
			t.Errorf("%s: IsSuccess=%v, want %v", tt.status, nr.IsSuccess(), tt.isSuccess)
		}
	}
}
