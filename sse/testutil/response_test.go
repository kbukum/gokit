package testutil_test

import (
	"errors"
	"net"
	"os"
	"testing"
	"time"

	"github.com/kbukum/gokit/sse/testutil"
)

func TestResponseWriterRecordsAndInjectsFailures(t *testing.T) {
	t.Parallel()
	w := testutil.NewResponseWriter()
	if n, err := w.WriteString("event"); err != nil || n != 5 || w.Body.String() != "event" {
		t.Fatalf("recording: n=%d error=%v body=%s", n, err, w.Body)
	}
	deadline := time.Unix(1, 0)
	if err := w.SetWriteDeadline(deadline); err != nil || w.Deadlines.Load() != 1 {
		t.Fatalf("deadline recording: %v", err)
	}
	if err := w.FlushError(); err != nil {
		t.Fatal(err)
	}
	failure := errors.New("injected transport failure")
	w.WriteError, w.DeadlineError, w.FlushErrorValue = failure, failure, failure
	if n, err := w.WriteString("discard"); n != 0 || !errors.Is(err, failure) || w.Body.String() != "event" {
		t.Fatalf("write injection: n=%d error=%v", n, err)
	}
	if err := w.SetWriteDeadline(deadline); !errors.Is(err, failure) || w.Deadlines.Load() != 2 {
		t.Fatalf("deadline injection: %v", err)
	}
	if err := w.FlushError(); !errors.Is(err, failure) {
		t.Fatalf("flush injection: %v", err)
	}
}

func TestResponseWriterPeerDeadlineInterruptsWrite(t *testing.T) {
	t.Parallel()
	writer, reader := net.Pipe()
	defer writer.Close()
	defer reader.Close()
	w := testutil.NewResponseWriter()
	w.Peer = writer
	if err := w.SetWriteDeadline(time.Unix(1, 0)); err != nil {
		t.Fatal(err)
	}
	if n, err := w.WriteString("blocked"); n != 0 || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("blocked peer deadline: n=%d error=%v", n, err)
	}
}
