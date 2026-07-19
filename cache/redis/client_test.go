package redis

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/kbukum/gokit/logging"
)

func TestNewRejectsDisabledAndInvalidConfig(t *testing.T) {
	t.Parallel()

	if _, err := New(Config{}, logging.NewDefault("test")); err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("New disabled error = %v", err)
	}
	if _, err := New(Config{Enabled: true}, logging.NewDefault("test")); err == nil || !strings.Contains(err.Error(), "addr") {
		t.Fatalf("New invalid config error = %v", err)
	}
}

func TestNewAppliesOptionalDurations(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t)
	addr := client.cfg.Addr
	cfg := Config{
		Enabled:         true,
		Addr:            addr,
		ConnMaxIdleTime: "2m",
		PoolTimeout:     "4s",
		ConnMaxLifetime: "5m",
		MinRetryBackoff: "10ms",
		MaxRetryBackoff: "20ms",
	}
	client, err := New(cfg, logging.NewDefault("test"))
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	opts := client.Unwrap().Options()
	if opts.ConnMaxIdleTime != 2*time.Minute || opts.PoolTimeout != 4*time.Second || opts.ConnMaxLifetime != 5*time.Minute {
		t.Fatalf("connection durations = idle %v pool %v lifetime %v", opts.ConnMaxIdleTime, opts.PoolTimeout, opts.ConnMaxLifetime)
	}
	if opts.MinRetryBackoff != 10*time.Millisecond || opts.MaxRetryBackoff != 20*time.Millisecond {
		t.Fatalf("retry backoffs = %v %v", opts.MinRetryBackoff, opts.MaxRetryBackoff)
	}
}

func TestClientPingAndJSON(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t)
	ctx := context.Background()
	if err := client.Ping(ctx); err != nil {
		t.Fatalf("Ping: %v", err)
	}

	want := redisState{Count: 4}
	if err := client.SetJSON(ctx, "json", want, 0); err != nil {
		t.Fatalf("SetJSON: %v", err)
	}
	var got redisState
	if err := client.GetJSON(ctx, "json", &got); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if got != want {
		t.Fatalf("GetJSON = %+v, want %+v", got, want)
	}
}

func TestClientJSONErrors(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t)
	ctx := context.Background()
	var got redisState
	if err := client.GetJSON(ctx, "missing", &got); err == nil || !errors.Is(err, goredis.Nil) {
		t.Fatalf("GetJSON missing error = %v", err)
	}
	if err := client.Set(ctx, "bad", []byte("{"), 0); err != nil {
		t.Fatalf("Set bad JSON: %v", err)
	}
	if err := client.GetJSON(ctx, "bad", &got); err == nil || !strings.Contains(err.Error(), "unmarshal") {
		t.Fatalf("GetJSON invalid error = %v", err)
	}
	if err := client.SetJSON(ctx, "bad-marshal", redisUnmarshalable{Fn: func() {}}, 0); err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Fatalf("SetJSON unmarshalable error = %v", err)
	}
}

func TestClientRedisErrorsAndCloseIdempotence(t *testing.T) {
	t.Parallel()

	client, _ := newTestClient(t)
	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	if err := ((*Client)(nil)).Close(); err != nil {
		t.Fatalf("nil Close: %v", err)
	}
	if _, _, err := client.Get(context.Background(), "k"); err == nil {
		t.Fatal("Get on closed client succeeded")
	}
	if _, err := client.Exists(context.Background(), "k"); err == nil {
		t.Fatal("Exists on closed client succeeded")
	}
}

type redisState struct {
	Count int `json:"count"`
}

type redisUnmarshalable struct {
	Fn func() `json:"fn"`
}
