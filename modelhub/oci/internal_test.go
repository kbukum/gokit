package oci

import (
	"testing"
	"time"
)

func TestNextLastOnlyFollowsSameTagsPath(t *testing.T) {
	t.Parallel()
	const reg, path = "https://registry.test:443", "/v2/ai/x/tags/list"
	for link, want := range map[string]string{
		`</v2/ai/x/tags/list?n=3&last=t2>; rel="next"`:                  "t2",
		`<https://registry.test/v2/ai/x/tags/list?last=t2>; rel="next"`: "t2",
		`<https://evil.test/v2/ai/x/tags/list?last=t2>; rel="next"`:     "",
		`</v2/ai/y/tags/list?last=t2>; rel="next"`:                      "",
		`</v2/ai/x/tags/list?last=t2>; rel="prev"`:                      "",
		`</v2/ai/x/tags/list?last=-bad>; rel="next"`:                    "",
		`</v2/ai/x/tags/list>; rel="next"`:                              "",
		``:                                                              "",
	} {
		if got, _ := nextLast(link, reg, path); got != want {
			t.Errorf("nextLast(%q) = %q, want %q", link, got, want)
		}
	}
}

func TestParseParams(t *testing.T) {
	t.Parallel()
	got := parseParams(`realm="https://auth.test/token", service="registry.test",scope=x, realm="dup"`)
	if got["realm"] != "https://auth.test/token" || got["service"] != "registry.test" || got["scope"] != "x" {
		t.Fatalf("parseParams() = %v", got)
	}
	if got := parseParams(`realm="unterminated`); got["realm"] != "" {
		t.Fatalf("unterminated = %v", got)
	}
}

func TestParseChallengeRefusesUnsafeRealms(t *testing.T) {
	t.Parallel()
	c := &Client{registry: "https://registry.test"}
	if ch, err := c.parseChallenge(`Bearer realm="https://auth.test/token",service="registry.test"`); err != nil || ch.realm != "https://auth.test:443/token" {
		t.Fatalf("parseChallenge() = %+v, %v", ch, err)
	}
	for _, h := range []string{
		`Basic realm="x"`,
		`Bearer realm="http://auth.test/token",service="s"`,
		`Bearer realm="file:///etc/passwd",service="s"`,
		`Bearer realm="https://auth.test/token"`,
		`Bearer service="s"`,
	} {
		if _, err := c.parseChallenge(h); err == nil {
			t.Errorf("parseChallenge(%q) accepted", h)
		}
	}
}

func TestTokenCacheExpiresAndStaysBounded(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	c := newTokenCache(2)
	c.put("a", token{value: "1", expires: now.Add(time.Minute)}, now)
	c.put("b", token{value: "2", expires: now.Add(2 * time.Minute)}, now)
	c.put("c", token{value: "3", expires: now.Add(3 * time.Minute)}, now)
	if _, ok := c.get("a", now); ok {
		t.Fatal("the oldest token survived eviction")
	}
	if v, ok := c.get("c", now); !ok || v != "3" {
		t.Fatal("the newest token was evicted")
	}
	if _, ok := c.get("b", now.Add(2*time.Minute)); ok {
		t.Fatal("an expired token was returned")
	}
	if len(c.tokens) > 2 {
		t.Fatalf("cache holds %d tokens", len(c.tokens))
	}
}
