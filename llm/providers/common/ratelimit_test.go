package common

import (
	"net/http"
	"testing"
)

func TestParseRateLimitHeaders(t *testing.T) {
	h := http.Header{}
	h.Set("x-ratelimit-limit-requests", "100")
	h.Set("x-ratelimit-remaining-requests", "42")
	h.Set("Retry-After", "30")

	info := ParseRateLimitHeaders(h)
	if info == nil {
		t.Fatal("expected non-nil RateLimitInfo")
	}
	if info.Limit != 100 {
		t.Errorf("expected limit 100, got %d", info.Limit)
	}
	if info.Remaining != 42 {
		t.Errorf("expected remaining 42, got %d", info.Remaining)
	}
	if info.RetryAfter.Seconds() != 30 {
		t.Errorf("expected retry after 30s, got %v", info.RetryAfter)
	}
}

func TestParseRateLimitHeaders_None(t *testing.T) {
	info := ParseRateLimitHeaders(http.Header{})
	if info != nil {
		t.Error("expected nil when no headers present")
	}
}
