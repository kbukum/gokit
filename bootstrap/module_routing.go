package bootstrap

import (
	"fmt"
	"net/http"
	"net/url"
	"slices"
	"strings"
)

type fallback struct {
	module  string
	handler http.Handler
}

func (w *moduleWiring) setFallback(listener, module string, handler http.Handler) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if prev, ok := w.fallbacks[listener]; ok {
		return fmt.Errorf("%w: %s already has a fallback from module %q", ErrRouteConflict, listener, prev.module)
	}
	w.fallbacks[listener] = fallback{module: module, handler: handler}
	return nil
}

// installFallbacks gives each listener its fallback, guarded so it never answers under a prefix a module route owns.
func (w *moduleWiring) installFallbacks() (err error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for listener, fb := range w.fallbacks {
		var reserved []string
		for pattern := range w.mounted[listener] {
			if prefix := routePrefix(pattern); prefix != "" && !slices.Contains(reserved, prefix) {
				reserved = append(reserved, prefix)
			}
		}
		if err := installFallback(w.listeners[listener], listener, fb, reserved); err != nil {
			return err
		}
	}
	return nil
}

func installFallback(l Listener, listener string, fb fallback, reserved []string) error {
	err := l.Fallback(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		if isReserved(r.URL.EscapedPath(), reserved) {
			http.NotFound(rw, r)
			return
		}
		fb.handler.ServeHTTP(rw, r)
	}))
	if err != nil {
		return fmt.Errorf("%w: %s fallback from module %q: %w", ErrRouteConflict, listener, fb.module, err)
	}
	return nil
}

// isReserved reports whether a path the mux left unmatched still belongs to a module prefix. http.ServeMux matches unescaped segments of the escaped path, so "/auth%2Fx" is one segment that no "/auth/" route matches; it is still under "/auth" once decoded and must not reach the fallback. A segment that does not decode is reserved too.
func isReserved(escaped string, reserved []string) bool {
	first, _, _ := strings.Cut(strings.TrimPrefix(escaped, "/"), "/")
	segment, err := url.PathUnescape(first)
	if err != nil {
		return true
	}
	segment = "/" + segment
	return slices.ContainsFunc(reserved, func(prefix string) bool {
		return segment == prefix || strings.HasPrefix(segment, prefix+"/")
	})
}

// routePrefix returns the first literal path segment of an http.ServeMux pattern ("[METHOD ][HOST]/[PATH]"), unescaped as the mux matches it, such as "/auth" for "POST /auth/login" or "POST /%61uth/login", or "" for the root, a wildcard segment or an invalid escape.
func routePrefix(pattern string) string {
	_, rest, found := strings.Cut(pattern, " ")
	if !found {
		rest = pattern
	}
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return ""
	}
	segment, _, _ := strings.Cut(rest[slash+1:], "/")
	if segment == "" || strings.Contains(segment, "{") {
		return ""
	}
	unescaped, err := url.PathUnescape(segment)
	if err != nil {
		return ""
	}
	return "/" + unescaped
}
