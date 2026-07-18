package authz

// Checker is the lightweight authorization interface.
// Group 05 canonical RBAC + ABAC evaluation is implemented by Engine;
// Checker remains useful for simple boolean permission gates.
//
// subject is typically a role name, user ID, or group — whatever makes sense
// for the project's authorization model.
//
// permission is the required permission string (e.g., "article:write").
type Checker interface {
	HasPermission(subject, permission string) bool
}

// CheckerFunc is an adapter to use ordinary functions as Checker.
type CheckerFunc func(subject, permission string) bool

// HasPermission implements Checker.
func (f CheckerFunc) HasPermission(subject, permission string) bool {
	return f(subject, permission)
}

// MapChecker is a simple in-memory Checker backed by a map of subject → permission patterns.
// Supports wildcard matching via MatchPattern and defaults to deny when no subject
// or permission pattern matches.
type MapChecker struct {
	permissions map[string][]string
}

// NewMapChecker creates a Checker from a static map of subject → permission patterns.
//
// Example:
//
//	checker := authz.NewMapChecker(map[string][]string{
//	    "admin":  {"*:*"},
//	    "editor": {"article:*", "media:read"},
//	})
func NewMapChecker(permissions map[string][]string) *MapChecker {
	return &MapChecker{permissions: permissions}
}

// HasPermission implements Checker.
func (c *MapChecker) HasPermission(subject, required string) bool {
	patterns, ok := c.permissions[subject]
	if !ok {
		return false
	}
	return MatchAny(patterns, required)
}
