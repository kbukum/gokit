package dag

import (
	"testing"
)

func TestNodeResult_Helpers(t *testing.T) {
	tests := []struct {
		status     string
		isTerminal bool
		isSkipped  bool
		isSuccess  bool
	}{
		{StatusCompleted, true, false, true},
		{StatusFailed, true, false, false},
		{StatusSkipped, false, true, false},
		{StatusUnavailable, false, false, false},
		{StatusDepUnavailable, false, true, false},
		{StatusDepFailed, false, true, false},
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
