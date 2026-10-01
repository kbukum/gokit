package wire

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/contracttest/golden"
	apperrors "github.com/kbukum/gokit/errors"
)

// TestWireFixtures_EncodeMatchesPublished proves gokit encodes exactly the bytes
// that are published. Run with GOLDEN_REGEN=1 to (re)generate the committed
// fixtures after an intentional contract change.
func TestWireFixtures_EncodeMatchesPublished(t *testing.T) {
	t.Parallel()

	if os.Getenv("GOLDEN_REGEN") == "1" {
		regenerate(t)
		return
	}

	published, err := LoadFixtures()
	if err != nil {
		t.Fatalf("load fixtures: %v", err)
	}

	for _, c := range WireCases() {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			want, ok := published[c.Name]
			if !ok {
				t.Fatalf("no published fixture for %q (run GOLDEN_REGEN=1)", c.Name)
			}
			got, renderErr := RenderFixture(c)
			if renderErr != nil {
				t.Fatalf("render: %v", renderErr)
			}
			golden.AssertJSON(t, mustMarshal(t, got), string(mustMarshal(t, want)))
		})
	}
}

// TestWireFixtures_ProblemJSONDecodes proves the published problem+json bytes
// decode back into the full vocabulary without loss (the HTTP decode side).
func TestWireFixtures_ProblemJSONDecodes(t *testing.T) {
	t.Parallel()

	published, err := LoadFixtures()
	if err != nil {
		t.Fatalf("load fixtures: %v", err)
	}

	for _, c := range WireCases() {
		t.Run(c.Name, func(t *testing.T) {
			t.Parallel()
			f := published[c.Name]

			var pd apperrors.ProblemDetail
			if decodeErr := json.Unmarshal(f.ProblemJSON, &pd); decodeErr != nil {
				t.Fatalf("decode problem+json: %v", decodeErr)
			}

			if string(pd.Code) != f.Vocabulary.Code {
				t.Errorf("code = %q, want %q", pd.Code, f.Vocabulary.Code)
			}
			if pd.Detail != f.Vocabulary.Message {
				t.Errorf("detail = %q, want %q", pd.Detail, f.Vocabulary.Message)
			}
			if pd.Reason != f.Vocabulary.Reason {
				t.Errorf("reason = %q, want %q", pd.Reason, f.Vocabulary.Reason)
			}
			if pd.TraceID != f.Vocabulary.TraceID {
				t.Errorf("traceId = %q, want %q", pd.TraceID, f.Vocabulary.TraceID)
			}
			if pd.Retryable != f.Vocabulary.Retryable {
				t.Errorf("retryable = %v, want %v", pd.Retryable, f.Vocabulary.Retryable)
			}
			if pd.Status != f.HTTPStatus {
				t.Errorf("status = %d, want %d", pd.Status, f.HTTPStatus)
			}
			wantSeconds := float64(f.Vocabulary.RetryAfterMs) / 1000
			if pd.RetryAfterSeconds != wantSeconds {
				t.Errorf("retryAfter = %v, want %v", pd.RetryAfterSeconds, wantSeconds)
			}
			if len(pd.Violations) != len(f.Vocabulary.Violations) {
				t.Fatalf("violations len = %d, want %d", len(pd.Violations), len(f.Vocabulary.Violations))
			}
			for i := range pd.Violations {
				if pd.Violations[i] != f.Vocabulary.Violations[i] {
					t.Errorf("violation[%d] = %+v, want %+v", i, pd.Violations[i], f.Vocabulary.Violations[i])
				}
			}
		})
	}
}

func regenerate(t *testing.T) {
	t.Helper()
	dir := filepath.Join("testdata", "wire")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, c := range WireCases() {
		f, err := RenderFixture(c)
		if err != nil {
			t.Fatalf("render %q: %v", c.Name, err)
		}
		data, err := json.MarshalIndent(f, "", "  ")
		if err != nil {
			t.Fatalf("marshal %q: %v", c.Name, err)
		}
		data = append(data, '\n')
		if err := os.WriteFile(filepath.Join(dir, c.Name+".json"), data, 0o644); err != nil {
			t.Fatalf("write %q: %v", c.Name, err)
		}
	}
	t.Logf("regenerated %d fixtures", len(WireCases()))
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return data
}
