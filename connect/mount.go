package connect

import "net/http"

// HandlerMounter is implemented by any server that can mount HTTP handlers.
// This matches gokit/server.Server.Handle without importing the server package.
type HandlerMounter interface {
	Handle(pattern string, handler http.Handler)
}

// Mount mounts a Connect-Go handler at the given path on the server's ServeMux.
func Mount(srv HandlerMounter, path string, handler http.Handler) {
	srv.Handle(path, handler)
}

// MountServices mounts multiple Connect-Go services on the server.
func MountServices(srv HandlerMounter, services ...Service) {
	for _, svc := range services {
		srv.Handle(svc.Path(), svc.Handler())
	}
}
