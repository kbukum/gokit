package testutil

import (
	"context"
	"slices"
	"testing"

	"github.com/kbukum/gokit/process"
)

func TestArgvRecorderRecordsLiteralArgs(t *testing.T) {
	t.Parallel()
	recorder := NewArgvRecorder(t)
	if _, err := process.Run(context.Background(), recorder.Command("one", "$(touch nope)")); err != nil {
		t.Fatalf("run recorder: %v", err)
	}
	if got := recorder.Args(t); !slices.Equal(got, []string{"one", "$(touch nope)"}) {
		t.Fatalf("recorded argv = %#v", got)
	}
}
