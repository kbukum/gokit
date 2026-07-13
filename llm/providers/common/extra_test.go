package common

import (
	"encoding/json"
	"testing"
)

func TestMergeExtra(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		extra   json.RawMessage
		wantErr bool
		check   func(t *testing.T, body map[string]any)
	}{
		{
			name:  "nil extra is no-op",
			extra: nil,
			check: func(t *testing.T, body map[string]any) {
				if len(body) != 1 {
					t.Fatalf("body mutated: %v", body)
				}
			},
		},
		{
			name:  "empty object is no-op",
			extra: json.RawMessage(`{}`),
			check: func(t *testing.T, body map[string]any) {
				if len(body) != 1 {
					t.Fatalf("body mutated: %v", body)
				}
			},
		},
		{
			name:  "merges scalar and nested members",
			extra: json.RawMessage(`{"think":false,"opts":{"a":1}}`),
			check: func(t *testing.T, body map[string]any) {
				if body["think"] != false {
					t.Fatalf("think = %v", body["think"])
				}
				if _, ok := body["opts"].(map[string]any); !ok {
					t.Fatalf("opts not decoded as object: %T", body["opts"])
				}
			},
		},
		{
			name:    "non-object extra fails closed",
			extra:   json.RawMessage(`[1,2,3]`),
			wantErr: true,
		},
		{
			name:    "malformed json fails closed",
			extra:   json.RawMessage(`{"a":`),
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := map[string]any{"model": "m"}
			err := MergeExtra(body, tt.extra)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, body)
			}
		})
	}
}
