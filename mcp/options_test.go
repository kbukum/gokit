package mcp

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/authz"
	"github.com/kbukum/gokit/mcp/security"
)

// applyOptions builds a serverConfig from opts, mirroring NewServer's assembly.
func applyOptions(opts ...ServerOption) *serverConfig {
	cfg := &serverConfig{}
	for _, o := range opts {
		o(cfg)
	}
	return cfg
}

func TestWithPolicyReplacesPolicy(t *testing.T) {
	t.Parallel()
	policy := security.Policy{
		AllowedTools:   security.ToSet([]string{"add"}),
		MaxInputBytes:  16,
		MaxResultBytes: 32,
	}
	cfg := applyOptions(WithPolicy(policy))
	if !cfg.policy.AllowsTool("add") || cfg.policy.AllowsTool("del") {
		t.Errorf("WithPolicy did not install the allow-list: %+v", cfg.policy.AllowedTools)
	}
	if cfg.policy.MaxInputBytes != 16 || cfg.policy.MaxResultBytes != 32 {
		t.Errorf("WithPolicy did not install size limits: %+v", cfg.policy)
	}
}

func TestGranularOptionsOverridePolicy(t *testing.T) {
	t.Parallel()
	base := security.Policy{MaxInputBytes: 8, MaxResultBytes: 8}
	decider := authz.DeciderFunc(func(_ context.Context, _ authz.Request) (authz.Decision, error) {
		return authz.Decision{Allowed: true}, nil
	})
	cfg := applyOptions(
		WithPolicy(base),
		WithMaxInputBytes(100),
		WithAuthzDecider(decider),
		WithAllowedPrompts("p1"),
		WithAllowedResources("file:///a"),
	)
	if cfg.policy.MaxInputBytes != 100 {
		t.Errorf("granular WithMaxInputBytes must override policy: got %d", cfg.policy.MaxInputBytes)
	}
	if cfg.policy.MaxResultBytes != 8 {
		t.Errorf("untouched policy field must survive: got %d", cfg.policy.MaxResultBytes)
	}
	if cfg.policy.Decider == nil {
		t.Error("WithAuthzDecider not applied")
	}
	if !cfg.policy.AllowsPrompt("p1") || cfg.policy.AllowsPrompt("other") {
		t.Errorf("WithAllowedPrompts not applied: %+v", cfg.policy.AllowedPrompts)
	}
	if !cfg.policy.AllowsResource("file:///a") || cfg.policy.AllowsResource("file:///b") {
		t.Errorf("WithAllowedResources not applied: %+v", cfg.policy.AllowedResources)
	}
}
