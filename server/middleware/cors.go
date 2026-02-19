package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORSConfig holds CORS middleware configuration.
type CORSConfig struct {
	AllowedOrigins   []string `yaml:"allowed_origins" mapstructure:"allowed_origins"`
	AllowedMethods   []string `yaml:"allowed_methods" mapstructure:"allowed_methods"`
	AllowedHeaders   []string `yaml:"allowed_headers" mapstructure:"allowed_headers"`
	AllowCredentials bool     `yaml:"allow_credentials" mapstructure:"allow_credentials"`
}

// CORS returns middleware that sets CORS headers and handles OPTIONS preflight.
func CORS(cfg *CORSConfig) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			setCORSHeaders(w.Header(), r.Header.Get("Origin"), cfg)
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// GinCORS returns a Gin middleware for CORS.
// Prefer using CORS() at the server level via ApplyMiddleware() which covers
// all routes. Use this only when you need CORS on the Gin engine directly.
func GinCORS(cfg *CORSConfig) gin.HandlerFunc {
	return GinWrap(CORS(cfg))
}

// setCORSHeaders writes CORS response headers if the origin is allowed.
func setCORSHeaders(h http.Header, origin string, cfg *CORSConfig) {
	if origin == "" || !isAllowedOrigin(origin, cfg.AllowedOrigins) {
		return
	}
	h.Set("Access-Control-Allow-Origin", origin)
	if len(cfg.AllowedMethods) > 0 {
		h.Set("Access-Control-Allow-Methods", strings.Join(cfg.AllowedMethods, ", "))
	}
	if len(cfg.AllowedHeaders) > 0 {
		h.Set("Access-Control-Allow-Headers", strings.Join(cfg.AllowedHeaders, ", "))
	}
	if cfg.AllowCredentials {
		h.Set("Access-Control-Allow-Credentials", "true")
	}
}

func isAllowedOrigin(origin string, allowed []string) bool {
	for _, a := range allowed {
		if origin == a || a == "*" {
			return true
		}
	}
	return false
}
