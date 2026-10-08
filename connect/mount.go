package connect

import (
	"fmt"
	"net/http"
)

// HandlerMounter is implemented by any server that can mount HTTP handlers.
// This matches gokit/server.Server.Handle without importing the server package.
type HandlerMounter interface {
	Handle(pattern string, handler http.Handler) error
}

// Mount mounts a Connect-Go handler at the given path on the server's ServeMux.
func Mount(srv HandlerMounter, path string, handler http.Handler) error {
	return srv.Handle(path, handler)
}

// MountServices mounts multiple Connect-Go services on the server, stopping at the first that fails.
func MountServices(srv HandlerMounter, services ...Service) error {
	for _, svc := range services {
		if err := srv.Handle(svc.Path(), svc.Handler()); err != nil {
			return fmt.Errorf("connect: mount %s: %w", svc.Path(), err)
		}
	}
	return nil
}
