package server

import (
	"strings"
)

// System route paths registered by gokit (health, info, metrics).
var systemPaths = map[string]bool{
	"/health":  true,
	"/info":    true,
	"/metrics": true,
}

// formatHandlerName extracts a clean handler name from Gin's full handler path.
// Gin stores handlers like:
//   "github.com/yourorg/yourservice/internal/api/port.(*UserPort).List-fm"
// We extract: "UserPort.List"
func formatHandlerName(fullPath string) string {
	// Remove -fm suffix Gin adds to method values
	name := strings.TrimSuffix(fullPath, "-fm")

	// Get the last segment after /
	if idx := strings.LastIndex(name, "/"); idx >= 0 {
		name = name[idx+1:]
	}

	// Clean up Go receiver notation: "(*UserPort).List" → "UserPort.List"
	name = strings.ReplaceAll(name, "(*", "")
	name = strings.ReplaceAll(name, ")", "")

	// Handle closure names like "Server.RegisterDefaultEndpoints.Health.func1"
	// Simplify to just the meaningful part
	if strings.Contains(name, ".func") {
		parts := strings.Split(name, ".")
		// Find the last meaningful name before funcN
		for i := len(parts) - 1; i >= 0; i-- {
			if !strings.HasPrefix(parts[i], "func") {
				name = strings.ToLower(parts[i])
				break
			}
		}
	}

	// Remove package prefix: "port.UserPort.List" → "UserPort.List"
	parts := strings.SplitN(name, ".", 2)
	if len(parts) == 2 {
		// If first part looks like a package name (lowercase, no capital letters), skip it
		hasUpper := false
		for _, c := range parts[0] {
			if c >= 'A' && c <= 'Z' {
				hasUpper = true
				break
			}
		}
		if !hasUpper && len(parts[1]) > 0 {
			name = parts[1]
		}
	}

	return name
}

// methodOrder returns a sort key for HTTP methods (GET first, DELETE last).
func methodOrder(method string) int {
	switch method {
	case "GET":
		return 0
	case "POST":
		return 1
	case "PUT":
		return 2
	case "PATCH":
		return 3
	case "DELETE":
		return 4
	default:
		return 5
	}
}
