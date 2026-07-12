package common

import (
	"net/http"
	"strconv"
	"time"
)

// RateLimitInfo holds rate limit metadata extracted from HTTP response headers.
type RateLimitInfo struct {
	// Limit is the max requests allowed in the window.
	Limit int
	// Remaining is the number of requests left in the current window.
	Remaining int
	// ResetAt is when the rate limit window resets.
	ResetAt time.Time
	// RetryAfter is how long to wait before retrying (for 429 responses).
	RetryAfter time.Duration
}

// ParseRateLimitHeaders extracts rate limit info from response headers.
// Supports OpenAI-style headers (x-ratelimit-*) and standard Retry-After.
func ParseRateLimitHeaders(headers http.Header) *RateLimitInfo {
	info := &RateLimitInfo{}
	found := false

	if v := headers.Get("x-ratelimit-limit-requests"); v != "" {
		info.Limit, _ = strconv.Atoi(v)
		found = true
	}
	if v := headers.Get("x-ratelimit-remaining-requests"); v != "" {
		info.Remaining, _ = strconv.Atoi(v)
		found = true
	}
	if v := headers.Get("x-ratelimit-reset-requests"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			info.ResetAt = time.Now().Add(d)
			found = true
		}
	}
	if v := headers.Get("Retry-After"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil {
			info.RetryAfter = time.Duration(secs) * time.Second
			found = true
		}
	}

	if !found {
		return nil
	}
	return info
}
