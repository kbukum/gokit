package mcp

import (
	"context"
	"log/slog"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestServerOptionsOverlay(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	subscribed := false
	cfg := &serverConfig{
		instructions:            "be careful",
		logger:                  logger,
		rootsListChangedHandler: func(context.Context, *sdkmcp.RootsListChangedRequest) {},
		progressHandler:         func(context.Context, *sdkmcp.ProgressNotificationServerRequest) {},
		subscribeHandler:        func(context.Context, *sdkmcp.SubscribeRequest) error { subscribed = true; return nil },
		unsubscribeHandler:      func(context.Context, *sdkmcp.UnsubscribeRequest) error { return nil },
	}
	opts := cfg.serverOptions()
	if opts.Instructions != "be careful" {
		t.Errorf("instructions not applied: %q", opts.Instructions)
	}
	if opts.Logger != logger {
		t.Error("logger not applied")
	}
	if opts.RootsListChangedHandler == nil || opts.ProgressNotificationHandler == nil {
		t.Error("roots/progress handlers not wired")
	}
	if opts.SubscribeHandler == nil || opts.UnsubscribeHandler == nil {
		t.Fatal("subscribe handlers not wired")
	}
	if err := opts.SubscribeHandler(context.Background(), nil); err != nil || !subscribed {
		t.Error("subscribe handler not the injected one")
	}
}

func TestServerOptionsPreservesBase(t *testing.T) {
	t.Parallel()
	base := &sdkmcp.ServerOptions{Instructions: "base", PageSize: 42}
	cfg := &serverConfig{baseServerOpts: base}
	opts := cfg.serverOptions()
	if opts.PageSize != 42 {
		t.Errorf("base option lost: PageSize=%d", opts.PageSize)
	}
	if opts.Instructions != "base" {
		t.Errorf("base instructions overwritten: %q", opts.Instructions)
	}
	// Overriding must not mutate the caller-supplied base struct.
	cfg.instructions = "override"
	opts = cfg.serverOptions()
	if opts.Instructions != "override" {
		t.Errorf("instructions override not applied: %q", opts.Instructions)
	}
	if base.Instructions != "base" {
		t.Errorf("base struct was mutated: %q", base.Instructions)
	}
}
