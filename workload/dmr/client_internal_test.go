package dmr

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestReadLimitedRejectsOversized(t *testing.T) {
	t.Parallel()
	if _, err := readLimited(strings.NewReader("123456"), 4, "body"); err == nil {
		t.Fatal("expected oversized body to be rejected")
	}
	data, err := readLimited(strings.NewReader("1234"), 4, "body")
	if err != nil {
		t.Fatalf("read within limit: %v", err)
	}
	if string(data) != "1234" {
		t.Fatalf("unexpected data %q", string(data))
	}
}

func TestDecodeModelListBounded(t *testing.T) {
	t.Parallel()
	// A payload larger than the limit is rejected rather than parsed.
	big := "[" + strings.Repeat(`{"id":"x"},`, 100) + `{"id":"y"}]`
	if _, err := decodeModelList(strings.NewReader(big), 8); err == nil {
		t.Fatal("expected oversized model list to be rejected")
	}
	objs, err := decodeModelList(strings.NewReader(`[{"id":"a"},{"id":"b"}]`), 1<<20)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objs))
	}
}

func TestDecodeModelListWrappers(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`{"data":[{"id":"a"}]}`, `{"models":[{"id":"a"}]}`} {
		objs, err := decodeModelList(strings.NewReader(body), 1<<20)
		if err != nil {
			t.Fatalf("decode %q: %v", body, err)
		}
		if len(objs) != 1 || objs[0].ID != "a" {
			t.Fatalf("unexpected objects for %q: %+v", body, objs)
		}
	}
}

// An object carrying neither "data" nor "models" must be rejected rather than
// treated as an empty list, so a success-shaped error payload cannot masquerade
// as success.
func TestDecodeModelListRejectsMissingArray(t *testing.T) {
	t.Parallel()
	for _, body := range []string{`{"error":"failed"}`, `{}`, `null`} {
		if _, err := decodeModelList(strings.NewReader(body), 1<<20); err == nil {
			t.Fatalf("expected rejection for %q", body)
		}
	}
	// An explicit empty array remains a valid empty list.
	objs, err := decodeModelList(strings.NewReader(`{"data":[]}`), 1<<20)
	if err != nil {
		t.Fatalf("empty data array: %v", err)
	}
	if len(objs) != 0 {
		t.Fatalf("expected empty list, got %+v", objs)
	}
}

func TestDrainPullStreamCapExceededIsError(t *testing.T) {
	t.Parallel()
	// A stream longer than the limit must be reported as a failure, never as a
	// completed pull.
	stream := strings.NewReader(strings.Repeat(`{"type":"progress"}`+"\n", 100))
	err := drainPullStream(stream, 16)
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected limit-exceeded error, got %v", err)
	}
}

func TestDrainPullStreamGenuineEOF(t *testing.T) {
	t.Parallel()
	stream := strings.NewReader(`{"type":"progress"}` + "\n" + `{"type":"success","message":"Model pulled successfully"}` + "\n")
	if err := drainPullStream(stream, 1<<20); err != nil {
		t.Fatalf("expected clean completion, got %v", err)
	}
}

// DMR reports a failed pull with an in-band {"type":"error"} event and then a
// clean EOF; it must be surfaced as a failure, not swallowed as success.
func TestDrainPullStreamTypeErrorEvent(t *testing.T) {
	t.Parallel()
	stream := strings.NewReader(`{"type":"progress"}` + "\n" + `{"type":"error","message":"Error: disk full"}` + "\n")
	err := drainPullStream(stream, 1<<20)
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Fatalf("expected type:error event surfaced as failure, got %v", err)
	}
}

// A stream that reaches a clean EOF without a terminal success event is a
// truncated pull and must be reported as a failure, never as success.
func TestDrainPullStreamEOFBeforeSuccess(t *testing.T) {
	t.Parallel()
	stream := strings.NewReader(`{"type":"progress"}` + "\n" + `{"type":"progress"}` + "\n")
	err := drainPullStream(stream, 1<<20)
	if err == nil || !strings.Contains(err.Error(), "before a success") {
		t.Fatalf("expected EOF-before-success failure, got %v", err)
	}
}

// A transport/decode error (not a DMR plain-text terminal error) must preserve
// its cause for errors.Is inspection rather than collapsing to a plain string.
func TestDrainPullStreamPreservesReadErrorCause(t *testing.T) {
	t.Parallel()
	want := errors.New("boom")
	// A partial JSON object followed by a read error is not a json.SyntaxError,
	// so the underlying cause must be wrapped.
	r := io.MultiReader(strings.NewReader(`{"type":`), &errReader{err: want})
	err := drainPullStream(r, 1<<20)
	if !errors.Is(err, want) {
		t.Fatalf("expected wrapped read error cause, got %v", err)
	}
}

// After the 200 header DMR may append a plain-text error tail, which reaches the
// JSON decoder as a syntax error. The tail read must stay subject to the same
// stream cap (through the shared LimitedReader) rather than reading the body
// unbounded past the advertised limit.
func TestDrainPullStreamPlainTextTailBounded(t *testing.T) {
	t.Parallel()
	stream := strings.NewReader(`{"type":"progress"}` + "\n" + strings.Repeat("x", 1<<20))
	err := drainPullStream(stream, 30)
	if err == nil {
		t.Fatal("expected terminal plain-text error to surface")
	}
	if len(err.Error()) > 200 {
		t.Fatalf("tail read exceeded the stream cap: error length %d", len(err.Error()))
	}
}

type errReader struct{ err error }

func (e *errReader) Read([]byte) (int, error) { return 0, e.err }
