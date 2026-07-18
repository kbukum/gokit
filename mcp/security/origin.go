package security

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateAllowedOrigin normalizes
// and rejects any Origin value that is not a bare scheme://host[:port] over http or https,
// failing closed on paths, queries, fragments, credentials, or opaque URLs.
func ValidateAllowedOrigin(origin string) (string, error) {
	parsed, err := url.Parse(origin)
	if err != nil {
		return "", fmt.Errorf("invalid allowed origin %q: %w", origin, err)
	}
	if parsed.Opaque != "" {
		return "", fmt.Errorf("invalid allowed origin %q: expected hierarchical URL", origin)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("invalid allowed origin %q: scheme must be http or https", origin)
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("invalid allowed origin %q: missing host", origin)
	}
	if parsed.User != nil {
		return "", fmt.Errorf("origin must not contain user info: %s", origin)
	}
	if parsed.Path != "" && parsed.Path != "/" {
		return "", fmt.Errorf("origin must not contain a path: %s", origin)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("origin must not contain query or fragment: %s", origin)
	}
	parsed.Scheme = scheme
	parsed.Host = strings.ToLower(parsed.Host)
	parsed.Path = ""
	parsed.RawPath = ""
	parsed.ForceQuery = false
	return parsed.String(), nil
}
