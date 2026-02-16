package connect

import (
	"net/http"

	"github.com/kbukum/gokit/server"
)

// Mount mounts a Connect-Go handler at the given path on the server's ServeMux.
func Mount(srv *server.Server, path string, handler http.Handler) {
	srv.Handle(path, handler)
}

// MountServices mounts multiple Connect-Go services on the server.
func MountServices(srv *server.Server, services ...Service) {
	for _, svc := range services {
		srv.Handle(svc.Path(), svc.Handler())
	}
}
