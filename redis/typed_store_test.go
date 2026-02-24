package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/kbukum/gokit/logger"
)

// newTestClient creates a redis.Client backed by miniredis for testing.
func newTestClient(t *testing.T) (*Client, *miniredis.Miniredis) {
	t.Helper()
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	t.Cleanup(func() { mini.Close() })

	log := logger.NewDefault("redis-test")
	cfg := Config{
		Enabled: true,
		Addr:    mini.Addr(),
	}
	cfg.ApplyDefaults()

	client, err := New(cfg, log)
	if err != nil {
		t.Fatalf("failed to create redis client: %v", err)
	}
	t.Cleanup(func() { client.Close() })
	return client, mini
}

// --- TypedStore tests ---

func TestTypedStore_SaveAndLoad(t *testing.T) {
	client, _ := newTestClient(t)
	store := NewTypedStore[testState](client, "test")
	ctx := context.Background()

	state := testState{Count: 5, Tags: []string{"a", "b"}}
	if err := store.Save(ctx, "k1", &state, 0); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	got, err := store.Load(ctx, "k1")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil result")
	}
	if got.Count != 5 || len(got.Tags) != 2 {
		t.Fatalf("expected Count=5, Tags=2, got %+v", got)
	}
}

func TestTypedStore_LoadMissing(t *testing.T) {
	client, _ := newTestClient(t)
	store := NewTypedStore[testState](client, "test")

	got, err := store.Load(context.Background(), "nonexistent")
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing key, got %+v", got)
	}
}

func TestTypedStore_Delete(t *testing.T) {
	client, _ := newTestClient(t)
	store := NewTypedStore[testState](client, "test")
	ctx := context.Background()

	state := testState{Count: 1}
	store.Save(ctx, "k1", &state, 0)

	if err := store.Delete(ctx, "k1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	got, err := store.Load(ctx, "k1")
	if err != nil {
		t.Fatalf("Load after delete failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil after delete, got %+v", got)
	}
}

func TestTypedStore_TTL(t *testing.T) {
	client, mini := newTestClient(t)
	store := NewTypedStore[testState](client, "test")
	ctx := context.Background()

	state := testState{Count: 1}
	if err := store.Save(ctx, "k1", &state, 2*time.Second); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Should be present
	got, err := store.Load(ctx, "k1")
	if err != nil || got == nil {
		t.Fatalf("expected value before TTL, got %v, err %v", got, err)
	}

	// Fast-forward time in miniredis
	mini.FastForward(3 * time.Second)

	got, err = store.Load(ctx, "k1")
	if err != nil {
		t.Fatalf("Load after TTL failed: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil after TTL expiration, got %+v", got)
	}
}

func TestTypedStore_KeyPrefix(t *testing.T) {
	client, mini := newTestClient(t)
	store := NewTypedStore[testState](client, "myprefix")
	ctx := context.Background()

	state := testState{Count: 42}
	store.Save(ctx, "k1", &state, 0)

	// Verify the key in Redis uses the prefix
	raw, err := mini.Get("myprefix:k1")
	if err != nil {
		t.Fatalf("expected prefixed key in Redis, err: %v", err)
	}
	if raw == "" {
		t.Fatal("expected non-empty value at prefixed key")
	}
}

func TestTypedStore_NoPrefix(t *testing.T) {
	client, mini := newTestClient(t)
	store := NewTypedStore[testState](client, "")
	ctx := context.Background()

	state := testState{Count: 1}
	store.Save(ctx, "bare-key", &state, 0)

	raw, err := mini.Get("bare-key")
	if err != nil {
		t.Fatalf("expected bare key in Redis, err: %v", err)
	}
	if raw == "" {
		t.Fatal("expected non-empty value at bare key")
	}
}

func TestTypedStore_Overwrite(t *testing.T) {
	client, _ := newTestClient(t)
	store := NewTypedStore[testState](client, "test")
	ctx := context.Background()

	s1 := testState{Count: 1}
	s2 := testState{Count: 2}
	store.Save(ctx, "k1", &s1, 0)
	store.Save(ctx, "k1", &s2, 0)

	got, _ := store.Load(ctx, "k1")
	if got == nil || got.Count != 2 {
		t.Fatalf("expected Count=2, got %+v", got)
	}
}

// --- GetJSON/SetJSON tests ---

func TestGetJSON_SetJSON(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	val := testState{Count: 10, Tags: []string{"x", "y"}}
	if err := client.SetJSON(ctx, "json-key", val, 0); err != nil {
		t.Fatalf("SetJSON failed: %v", err)
	}

	var got testState
	if err := client.GetJSON(ctx, "json-key", &got); err != nil {
		t.Fatalf("GetJSON failed: %v", err)
	}
	if got.Count != 10 || len(got.Tags) != 2 {
		t.Fatalf("expected Count=10, Tags=2, got %+v", got)
	}
}

func TestGetJSON_Missing(t *testing.T) {
	client, _ := newTestClient(t)
	var got testState
	err := client.GetJSON(context.Background(), "missing", &got)
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

// --- test type ---

type testState struct {
	Count int      `json:"count"`
	Tags  []string `json:"tags,omitempty"`
}
