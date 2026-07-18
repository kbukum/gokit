package query

import "regexp"

// safeIdentifier matches a SQL identifier that is safe to interpolate into a query fragment: one or more segments of a letter/underscore followed by letters, digits, or underscores, joined by single dots (e.g. "name", "users.id", "public.users.created_at"). Anything containing whitespace, quotes, or SQL metacharacters is rejected.
var safeIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)

// isSafeIdentifier reports whether field is a syntactically safe SQL identifier.
//
// Column and table names cannot be passed as bound parameters, so they are interpolated into query fragments directly. Every such interpolation is gated on this whitelist so a caller-supplied field can never inject SQL; clauses with an unsafe identifier are skipped by the builder (fail closed).
func isSafeIdentifier(field string) bool {
	return safeIdentifier.MatchString(field)
}
