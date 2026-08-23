package security

// Shared HTTP authentication scheme names. These are the canonical spellings
// used in the Authorization / WWW-Authenticate headers, exposed here so transport
// middleware references one vocabulary instead of scattering string literals
// (e.g. server/middleware auth defaults to BearerAuthScheme).
const (
	// BasicAuthScheme is the HTTP "Basic" authentication scheme name.
	BasicAuthScheme = "Basic"

	// BearerAuthScheme is the HTTP "Bearer" authentication scheme name.
	BearerAuthScheme = "Bearer"
)
