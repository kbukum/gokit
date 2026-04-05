package redis

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/logger"
)

// ──────────────────────────────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────────────────────────────

// complexState exercises nested and diverse JSON types.
type complexState struct {
	Name     string            `json:"name"`
	Score    float64           `json:"score"`
	Active   bool              `json:"active"`
	Tags     []string          `json:"tags"`
	Metadata map[string]string `json:"metadata"`
	Nested   *nestedInfo       `json:"nested,omitempty"`
}

type nestedInfo struct {
	Level  int      `json:"level"`
	Values []int    `json:"values"`
	Inner  innerObj `json:"inner"`
}

type innerObj struct {
	Key string `json:"key"`
}

// ──────────────────────────────────────────────────────────────────────
// Config validation
// ──────────────────────────────────────────────────────────────────────

func TestConfig_ApplyDefaults(t *testing.T) {
	cfg := Config{}
	cfg.ApplyDefaults()

	if cfg.PoolSize != 10 {
		t.Errorf("expected PoolSize=10, got %d", cfg.PoolSize)
	}
	if cfg.MinIdleConns != 2 {
		t.Errorf("expected MinIdleConns=2, got %d", cfg.MinIdleConns)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", cfg.MaxRetries)
	}
	if cfg.DialTimeout != "5s" {
		t.Errorf("expected DialTimeout=5s, got %s", cfg.DialTimeout)
	}
	if cfg.ReadTimeout != "3s" {
		t.Errorf("expected ReadTimeout=3s, got %s", cfg.ReadTimeout)
	}
	if cfg.WriteTimeout != "3s" {
		t.Errorf("expected WriteTimeout=3s, got %s", cfg.WriteTimeout)
	}
}

func TestConfig_ApplyDefaults_PreservesExisting(t *testing.T) {
	cfg := Config{PoolSize: 50, DialTimeout: "10s"}
	cfg.ApplyDefaults()

	if cfg.PoolSize != 50 {
		t.Errorf("expected PoolSize preserved at 50, got %d", cfg.PoolSize)
	}
	if cfg.DialTimeout != "10s" {
		t.Errorf("expected DialTimeout preserved at 10s, got %s", cfg.DialTimeout)
	}
}

func TestConfig_Validate_DisabledSkips(t *testing.T) {
	cfg := Config{Enabled: false}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled config should not error: %v", err)
	}
}

func TestConfig_Validate_EmptyAddr(t *testing.T) {
	cfg := Config{Enabled: true, Addr: ""}
	cfg.ApplyDefaults()

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for empty addr")
	}
	if !contains(err.Error(), "addr is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfig_Validate_InvalidPoolSize(t *testing.T) {
	cfg := Config{Enabled: true, Addr: "localhost:6379", PoolSize: -1}
	cfg.DialTimeout = "5s"
	cfg.ReadTimeout = "3s"
	cfg.WriteTimeout = "3s"

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for negative pool size")
	}
	if !contains(err.Error(), "pool_size") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfig_Validate_InvalidDialTimeout(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Addr:         "localhost:6379",
		PoolSize:     10,
		DialTimeout:  "not-a-duration",
		ReadTimeout:  "3s",
		WriteTimeout: "3s",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid dial_timeout")
	}
	if !contains(err.Error(), "dial_timeout") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConfig_Validate_InvalidReadTimeout(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Addr:         "localhost:6379",
		PoolSize:     10,
		DialTimeout:  "5s",
		ReadTimeout:  "bad",
		WriteTimeout: "3s",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid read_timeout")
	}
}

func TestConfig_Validate_InvalidWriteTimeout(t *testing.T) {
	cfg := Config{
		Enabled:      true,
		Addr:         "localhost:6379",
		PoolSize:     10,
		DialTimeout:  "5s",
		ReadTimeout:  "3s",
		WriteTimeout: "bad",
	}

	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for invalid write_timeout")
	}
}

func TestConfig_Validate_Success(t *testing.T) {
	cfg := Config{Enabled: true, Addr: "localhost:6379"}
	cfg.ApplyDefaults()

	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid config should not error: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────
// New() error paths
// ──────────────────────────────────────────────────────────────────────

func TestNew_DisabledConfig(t *testing.T) {
	log := logger.NewDefault("test")
	cfg := Config{Enabled: false}

	_, err := New(cfg, log)
	if err == nil {
		t.Fatal("expected error for disabled config")
	}
	if !contains(err.Error(), "disabled") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNew_InvalidConfig(t *testing.T) {
	log := logger.NewDefault("test")
	cfg := Config{Enabled: true, Addr: ""}
	cfg.ApplyDefaults()

	_, err := New(cfg, log)
	if err == nil {
		t.Fatal("expected error for invalid config")
	}
}

// ──────────────────────────────────────────────────────────────────────
// Key prefix isolation
// ──────────────────────────────────────────────────────────────────────

func TestTypedStore_PrefixIsolation(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	storeA := NewTypedStore[testState](client, "svcA")
	storeB := NewTypedStore[testState](client, "svcB")

	valA := testState{Count: 100, Tags: []string{"a"}}
	valB := testState{Count: 200, Tags: []string{"b"}}

	storeA.Save(ctx, "shared", &valA, 0)
	storeB.Save(ctx, "shared", &valB, 0)

	gotA, _ := storeA.Load(ctx, "shared")
	gotB, _ := storeB.Load(ctx, "shared")

	if gotA.Count != 100 {
		t.Errorf("storeA: expected Count=100, got %d", gotA.Count)
	}
	if gotB.Count != 200 {
		t.Errorf("storeB: expected Count=200, got %d", gotB.Count)
	}
}

func TestTypedStore_FullKey(t *testing.T) {
	client, _ := newTestClient(t)
	store := NewTypedStore[testState](client, "prefix")

	got := store.fullKey("mykey")
	if got != "prefix:mykey" {
		t.Fatalf("expected 'prefix:mykey', got %q", got)
	}
}

func TestTypedStore_FullKey_Empty(t *testing.T) {
	client, _ := newTestClient(t)
	store := NewTypedStore[testState](client, "")

	got := store.fullKey("mykey")
	if got != "mykey" {
		t.Fatalf("expected 'mykey', got %q", got)
	}
}

// ──────────────────────────────────────────────────────────────────────
// TypedStore — complex types JSON round-trip
// ──────────────────────────────────────────────────────────────────────

func TestTypedStore_ComplexType_RoundTrip(t *testing.T) {
	client, _ := newTestClient(t)
	store := NewTypedStore[complexState](client, "complex")
	ctx := context.Background()

	val := complexState{
		Name:   "test-entity",
		Score:  99.5,
		Active: true,
		Tags:   []string{"go", "redis", "testing"},
		Metadata: map[string]string{
			"env":    "test",
			"region": "us-east",
		},
		Nested: &nestedInfo{
			Level:  3,
			Values: []int{10, 20, 30},
			Inner:  innerObj{Key: "deep"},
		},
	}

	if err := store.Save(ctx, "c1", &val, 0); err != nil {
		t.Fatalf("Save complex: %v", err)
	}

	got, err := store.Load(ctx, "c1")
	if err != nil {
		t.Fatalf("Load complex: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil")
	}

	if got.Name != "test-entity" {
		t.Errorf("Name mismatch: %s", got.Name)
	}
	if got.Score != 99.5 {
		t.Errorf("Score mismatch: %f", got.Score)
	}
	if !got.Active {
		t.Error("Active should be true")
	}
	if len(got.Tags) != 3 {
		t.Errorf("expected 3 tags, got %d", len(got.Tags))
	}
	if got.Metadata["region"] != "us-east" {
		t.Errorf("Metadata mismatch: %v", got.Metadata)
	}
	if got.Nested == nil || got.Nested.Level != 3 {
		t.Errorf("Nested mismatch: %v", got.Nested)
	}
	if got.Nested.Inner.Key != "deep" {
		t.Errorf("Inner.Key mismatch: %s", got.Nested.Inner.Key)
	}
}

func TestTypedStore_NilNested(t *testing.T) {
	client, _ := newTestClient(t)
	store := NewTypedStore[complexState](client, "complex")
	ctx := context.Background()

	val := complexState{Name: "minimal", Nested: nil}
	store.Save(ctx, "c2", &val, 0)

	got, _ := store.Load(ctx, "c2")
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if got.Nested != nil {
		t.Errorf("expected nil Nested, got %v", got.Nested)
	}
}

func TestTypedStore_EmptyCollections(t *testing.T) {
	client, _ := newTestClient(t)
	store := NewTypedStore[complexState](client, "empty")
	ctx := context.Background()

	val := complexState{
		Tags:     []string{},
		Metadata: map[string]string{},
	}
	store.Save(ctx, "e1", &val, 0)

	got, _ := store.Load(ctx, "e1")
	if got == nil {
		t.Fatal("expected non-nil")
	}
	if len(got.Tags) != 0 {
		t.Errorf("expected empty Tags, got %v", got.Tags)
	}
	if len(got.Metadata) != 0 {
		t.Errorf("expected empty Metadata, got %v", got.Metadata)
	}
}

// ──────────────────────────────────────────────────────────────────────
// TTL behavior
// ──────────────────────────────────────────────────────────────────────

func TestTypedStore_TTL_ZeroMeansNoPersist(t *testing.T) {
	client, mini := newTestClient(t)
	store := NewTypedStore[testState](client, "ttl")
	ctx := context.Background()

	val := testState{Count: 1}
	store.Save(ctx, "k1", &val, 0)

	// 0 TTL = no expiration, key should survive any fast-forward
	mini.FastForward(1 * time.Hour)
	got, _ := store.Load(ctx, "k1")
	if got == nil {
		t.Fatal("key with 0 TTL should not expire")
	}
}

func TestTypedStore_TTL_ExpiresExactly(t *testing.T) {
	client, mini := newTestClient(t)
	store := NewTypedStore[testState](client, "ttl")
	ctx := context.Background()

	val := testState{Count: 1}
	store.Save(ctx, "k1", &val, 5*time.Second)

	// Still alive at 4s
	mini.FastForward(4 * time.Second)
	got, _ := store.Load(ctx, "k1")
	if got == nil {
		t.Fatal("key should still be alive at 4s (TTL=5s)")
	}

	// Expired at 6s
	mini.FastForward(2 * time.Second)
	got, _ = store.Load(ctx, "k1")
	if got != nil {
		t.Fatal("key should be expired after 6s (TTL=5s)")
	}
}

func TestClient_SetWithTTL(t *testing.T) {
	client, mini := newTestClient(t)
	ctx := context.Background()

	client.Set(ctx, "ttl-key", "value", 3*time.Second)

	val, err := client.Get(ctx, "ttl-key")
	if err != nil || val != "value" {
		t.Fatalf("expected 'value', got %q, err=%v", val, err)
	}

	mini.FastForward(4 * time.Second)
	val, _ = client.Get(ctx, "ttl-key")
	if val != "" {
		t.Fatalf("expected empty after TTL, got %q", val)
	}
}

// ──────────────────────────────────────────────────────────────────────
// Error handling
// ──────────────────────────────────────────────────────────────────────

func TestClient_GetOnClosedConnection(t *testing.T) {
	client, mini := newTestClient(t)
	ctx := context.Background()

	mini.Close()

	_, err := client.Get(ctx, "key")
	if err == nil {
		t.Fatal("expected error on closed connection")
	}
}

func TestClient_SetOnClosedConnection(t *testing.T) {
	client, mini := newTestClient(t)
	ctx := context.Background()

	mini.Close()

	err := client.Set(ctx, "key", "val", 0)
	if err == nil {
		t.Fatal("expected error on closed connection")
	}
}

func TestGetJSON_InvalidJSON(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	// Store invalid JSON
	client.Set(ctx, "bad-json", "not{valid}json", 0)

	var dest testState
	err := client.GetJSON(ctx, "bad-json", &dest)
	if err == nil {
		t.Fatal("expected unmarshal error for invalid JSON")
	}
	if !contains(err.Error(), "unmarshal") {
		t.Fatalf("expected unmarshal error, got: %v", err)
	}
}

func TestSetJSON_UnmarshalableValue(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	// channels cannot be JSON-marshaled
	ch := make(chan int)
	err := client.SetJSON(ctx, "bad", ch, 0)
	if err == nil {
		t.Fatal("expected marshal error for channel")
	}
	if !contains(err.Error(), "marshal") {
		t.Fatalf("expected marshal error, got: %v", err)
	}
}

func TestTypedStore_LoadCorruptedJSON(t *testing.T) {
	client, _ := newTestClient(t)
	store := NewTypedStore[testState](client, "corrupt")
	ctx := context.Background()

	// Write raw invalid JSON under the prefixed key
	client.Set(ctx, "corrupt:k1", "{bad json", 0)

	_, err := store.Load(ctx, "k1")
	if err == nil {
		t.Fatal("expected unmarshal error for corrupted JSON")
	}
	if !contains(err.Error(), "unmarshal") {
		t.Fatalf("expected unmarshal error, got: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────
// Concurrent get/set
// ──────────────────────────────────────────────────────────────────────

func TestTypedStore_ConcurrentAccess(t *testing.T) {
	client, _ := newTestClient(t)
	store := NewTypedStore[testState](client, "conc")
	ctx := context.Background()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			val := testState{Count: n, Tags: []string{"concurrent"}}
			key := "k1"
			if err := store.Save(ctx, key, &val, 0); err != nil {
				t.Errorf("goroutine %d Save failed: %v", n, err)
			}
			if _, err := store.Load(ctx, key); err != nil {
				t.Errorf("goroutine %d Load failed: %v", n, err)
			}
		}(i)
	}

	wg.Wait()

	// Final read should succeed and be one of the written values
	got, err := store.Load(ctx, "k1")
	if err != nil {
		t.Fatalf("final Load failed: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil after concurrent writes")
	}
}

func TestClient_ConcurrentOperations(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			key := "conc-key"
			client.Set(ctx, key, "value", 0)
			client.Get(ctx, key)
			client.Exists(ctx, key)
		}(i)
	}

	wg.Wait()
}

// ──────────────────────────────────────────────────────────────────────
// Component lifecycle
// ──────────────────────────────────────────────────────────────────────

func TestComponent_Name(t *testing.T) {
	log := logger.NewDefault("test")
	comp := NewComponent(Config{}, log)
	if comp.Name() != "redis" {
		t.Fatalf("expected name='redis', got %q", comp.Name())
	}
}

func TestComponent_StartWithEmptyAddr(t *testing.T) {
	log := logger.NewDefault("test")
	comp := NewComponent(Config{Addr: ""}, log)

	err := comp.Start(context.Background())
	if err != nil {
		t.Fatalf("empty addr should start in no-op mode: %v", err)
	}
	if comp.Client() != nil {
		t.Fatal("client should be nil in no-op mode")
	}
}

func TestComponent_HealthUninitialized(t *testing.T) {
	log := logger.NewDefault("test")
	comp := NewComponent(Config{}, log)

	h := comp.Health(context.Background())
	if h.Status != component.StatusUnhealthy {
		t.Fatalf("expected unhealthy, got %s", h.Status)
	}
	if !contains(h.Message, "not initialized") {
		t.Fatalf("expected 'not initialized' message, got %q", h.Message)
	}
}

func TestComponent_StartAndHealth(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(func() { mini.Close() })

	log := logger.NewDefault("test")
	cfg := Config{Enabled: true, Addr: mini.Addr()}
	cfg.ApplyDefaults()

	comp := NewComponent(cfg, log)
	ctx := context.Background()

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if comp.Client() == nil {
		t.Fatal("client should be non-nil after start")
	}

	h := comp.Health(ctx)
	if h.Status != component.StatusHealthy {
		t.Fatalf("expected healthy after start, got %s", h.Status)
	}
}

func TestComponent_StopNilClient(t *testing.T) {
	log := logger.NewDefault("test")
	comp := NewComponent(Config{}, log)

	// Stop without Start should be safe
	if err := comp.Stop(context.Background()); err != nil {
		t.Fatalf("Stop on nil client should not error: %v", err)
	}
}

func TestComponent_StartStop(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}
	t.Cleanup(func() { mini.Close() })

	log := logger.NewDefault("test")
	cfg := Config{Enabled: true, Addr: mini.Addr()}
	cfg.ApplyDefaults()

	comp := NewComponent(cfg, log)
	ctx := context.Background()

	comp.Start(ctx)
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

func TestComponent_HealthAfterServerDown(t *testing.T) {
	mini, err := miniredis.Run()
	if err != nil {
		t.Fatalf("miniredis: %v", err)
	}

	log := logger.NewDefault("test")
	cfg := Config{Enabled: true, Addr: mini.Addr()}
	cfg.ApplyDefaults()

	comp := NewComponent(cfg, log)
	ctx := context.Background()
	comp.Start(ctx)

	// Kill the server
	mini.Close()

	h := comp.Health(ctx)
	if h.Status != component.StatusUnhealthy {
		t.Fatalf("expected unhealthy after server down, got %s", h.Status)
	}
	if !contains(h.Message, "ping failed") {
		t.Fatalf("expected ping failed message, got %q", h.Message)
	}
}

func TestComponent_Describe(t *testing.T) {
	log := logger.NewDefault("test")
	cfg := Config{Addr: "redis.example.com:6379"}
	comp := NewComponent(cfg, log)

	desc := comp.Describe()
	if desc.Name != "Redis" {
		t.Errorf("expected Name='Redis', got %q", desc.Name)
	}
	if desc.Type != "redis" {
		t.Errorf("expected Type='redis', got %q", desc.Type)
	}
	if !contains(desc.Details, "redis.example.com:6379") {
		t.Errorf("expected addr in details, got %q", desc.Details)
	}
}

// ──────────────────────────────────────────────────────────────────────
// Adapter (provider.Provider) interface
// ──────────────────────────────────────────────────────────────────────

func TestClient_Name(t *testing.T) {
	client, _ := newTestClient(t)
	// The test helper uses default Config with Name=""
	if client.Name() != "" {
		t.Fatalf("expected empty name from default config, got %q", client.Name())
	}
}

func TestClient_IsAvailable(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	if !client.IsAvailable(ctx) {
		t.Fatal("expected IsAvailable=true with miniredis")
	}
}

func TestClient_IsAvailable_AfterClose(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()
	client.Close()

	if client.IsAvailable(ctx) {
		t.Fatal("expected IsAvailable=false after Close")
	}
}

func TestClient_CloseIdempotent(t *testing.T) {
	client, _ := newTestClient(t)

	if err := client.Close(); err != nil {
		t.Fatalf("first Close failed: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close failed: %v", err)
	}
}

func TestClient_CloseNil(t *testing.T) {
	var c *Client
	if err := c.Close(); err != nil {
		t.Fatalf("Close on nil client should not error: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────
// Client basic operations
// ──────────────────────────────────────────────────────────────────────

func TestClient_Exists(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	client.Set(ctx, "e1", "val", 0)

	count, err := client.Exists(ctx, "e1")
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 existing key, got %d", count)
	}

	count, err = client.Exists(ctx, "missing")
	if err != nil {
		t.Fatalf("Exists error: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected 0, got %d", count)
	}
}

func TestClient_Del(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	client.Set(ctx, "d1", "v1", 0)
	client.Set(ctx, "d2", "v2", 0)

	if err := client.Del(ctx, "d1", "d2"); err != nil {
		t.Fatalf("Del error: %v", err)
	}

	v1, _ := client.Get(ctx, "d1")
	v2, _ := client.Get(ctx, "d2")
	if v1 != "" || v2 != "" {
		t.Fatalf("expected empty after Del, got %q, %q", v1, v2)
	}
}

func TestClient_Ping(t *testing.T) {
	client, _ := newTestClient(t)
	if err := client.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestClient_PingClosed(t *testing.T) {
	client, mini := newTestClient(t)
	mini.Close()

	err := client.Ping(context.Background())
	if err == nil {
		t.Fatal("expected error on closed connection")
	}
}

func TestClient_Unwrap(t *testing.T) {
	client, _ := newTestClient(t)
	rdb := client.Unwrap()
	if rdb == nil {
		t.Fatal("Unwrap should return non-nil")
	}
}

// ──────────────────────────────────────────────────────────────────────
// Security: key injection
// ──────────────────────────────────────────────────────────────────────

func TestTypedStore_KeyInjection(t *testing.T) {
	client, mini := newTestClient(t)
	store := NewTypedStore[testState](client, "safe")
	ctx := context.Background()

	// Try keys with special characters
	injectionKeys := []string{
		"../../etc/passwd",
		"key\x00null",
		"key with spaces",
		"key:with:colons",
		"key\nwith\nnewlines",
		"key\twith\ttabs",
		"*",
		"?",
	}

	for _, key := range injectionKeys {
		val := testState{Count: 1}
		err := store.Save(ctx, key, &val, 0)
		if err != nil {
			// Some keys may be rejected by Redis, that's fine
			continue
		}

		got, err := store.Load(ctx, key)
		if err != nil {
			continue
		}
		if got == nil || got.Count != 1 {
			t.Errorf("key %q: expected Count=1, got %v", key, got)
		}

		// Verify the actual key in Redis has the prefix
		expectedKey := "safe:" + key
		raw, redisErr := mini.Get(expectedKey)
		if redisErr != nil {
			continue
		}
		if raw == "" {
			t.Errorf("key %q: expected value at prefixed key in Redis", key)
		}
	}
}

// ──────────────────────────────────────────────────────────────────────
// JSON round-trip edge cases (GetJSON/SetJSON)
// ──────────────────────────────────────────────────────────────────────

func TestSetJSON_GetJSON_ComplexType(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	val := complexState{
		Name:     "json-test",
		Score:    3.14,
		Active:   false,
		Tags:     []string{"a"},
		Metadata: map[string]string{"k": "v"},
		Nested:   &nestedInfo{Level: 1, Values: []int{1}, Inner: innerObj{Key: "x"}},
	}

	client.SetJSON(ctx, "complex-json", val, 0)

	var got complexState
	client.GetJSON(ctx, "complex-json", &got)

	if got.Name != "json-test" || got.Score != 3.14 || got.Active {
		t.Errorf("basic fields mismatch: %+v", got)
	}
	if got.Nested == nil || got.Nested.Inner.Key != "x" {
		t.Errorf("nested mismatch: %+v", got.Nested)
	}
}

func TestSetJSON_GetJSON_NullValue(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	// Store JSON null
	client.SetJSON(ctx, "null-key", nil, 0)

	raw, err := client.Get(ctx, "null-key")
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	if raw != "null" {
		t.Fatalf("expected 'null', got %q", raw)
	}
}

func TestSetJSON_GetJSON_StringValue(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	client.SetJSON(ctx, "str-key", "hello world", 0)

	var got string
	client.GetJSON(ctx, "str-key", &got)
	if got != "hello world" {
		t.Fatalf("expected 'hello world', got %q", got)
	}
}

func TestSetJSON_GetJSON_ArrayValue(t *testing.T) {
	client, _ := newTestClient(t)
	ctx := context.Background()

	data := []int{1, 2, 3, 4, 5}
	client.SetJSON(ctx, "arr-key", data, 0)

	var got []int
	client.GetJSON(ctx, "arr-key", &got)
	if len(got) != 5 || got[0] != 1 || got[4] != 5 {
		t.Fatalf("array mismatch: %v", got)
	}
}

// ──────────────────────────────────────────────────────────────────────
// TypedStore: marshal error
// ──────────────────────────────────────────────────────────────────────

type unmarshalableType struct {
	Ch chan int `json:"ch"`
}

func TestTypedStore_SaveMarshalError(t *testing.T) {
	client, _ := newTestClient(t)
	store := NewTypedStore[unmarshalableType](client, "bad")
	ctx := context.Background()

	val := unmarshalableType{Ch: make(chan int)}
	err := store.Save(ctx, "k1", &val, 0)
	if err == nil {
		t.Fatal("expected marshal error for channel type")
	}
	if !contains(err.Error(), "marshal") {
		t.Fatalf("expected marshal error, got: %v", err)
	}
}

// ──────────────────────────────────────────────────────────────────────
// TypedStore: raw JSON integrity
// ──────────────────────────────────────────────────────────────────────

func TestTypedStore_RawJSONInRedis(t *testing.T) {
	client, mini := newTestClient(t)
	store := NewTypedStore[testState](client, "raw")
	ctx := context.Background()

	val := testState{Count: 42, Tags: []string{"x", "y"}}
	store.Save(ctx, "k1", &val, 0)

	raw, err := mini.Get("raw:k1")
	if err != nil {
		t.Fatalf("miniredis Get error: %v", err)
	}

	var decoded testState
	if err := json.Unmarshal([]byte(raw), &decoded); err != nil {
		t.Fatalf("raw JSON unmarshal error: %v", err)
	}
	if decoded.Count != 42 || len(decoded.Tags) != 2 {
		t.Fatalf("decoded mismatch: %+v", decoded)
	}
}

// ──────────────────────────────────────────────────────────────────────
// helpers
// ──────────────────────────────────────────────────────────────────────

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
