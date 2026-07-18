package providers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// FetchJSON performs a bearer-authenticated GET request
// and decodes the JSON response body into result.
//
// result is a deliberate opaque value:
// it is a JSON unmarshal target whose concrete shape is provider-specific,
// so it cannot be given a closed type here (same contract as [json.Unmarshal]).
func FetchJSON(ctx context.Context, client *http.Client, endpoint, accessToken string, result any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := resolveClient(client).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body))
	}
	return json.Unmarshal(body, result)
}

// StrVal extracts a string value from a JSON-decoded map. Returns "" if the key is missing
// or not a string.
func StrVal(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

// BoolVal extracts a bool value from a JSON-decoded map. Returns false if the key is missing
// or not a bool.
func BoolVal(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
	return v
}

// NestedMap traverses a dot-separated path in a JSON-decoded map. For example,
// NestedMap(m, "data.user") returns m["data"]["user"]. Returns nil if any segment is missing
// or not a map.
func NestedMap(m map[string]any, path string) map[string]any {
	if path == "" {
		return m
	}
	parts := strings.Split(path, ".")
	current := m
	for _, part := range parts {
		next, ok := current[part].(map[string]any)
		if !ok {
			return nil
		}
		current = next
	}
	return current
}
