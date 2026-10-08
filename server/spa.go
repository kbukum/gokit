package server

import (
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/server/spa"
	"github.com/kbukum/gokit/util"
)

// Fallback serves requests that no Gin route or mounted handler matches, behind the server's middleware. A later call replaces the handler. Like [Server.Handle], it panics on a nil or typed-nil handler, so a bad fallback fails at registration instead of on the first unmatched request.
func (s *Server) Fallback(handler http.Handler) {
	if util.IsNil(handler) {
		panic("server: nil fallback handler")
	}
	s.engine.NoRoute(func(c *gin.Context) {
		// Gin presets 404 for NoRoute; the fallback decides its own status.
		c.Status(http.StatusOK)
		handler.ServeHTTP(c.Writer, c.Request)
	})
}

// MountSPA serves a single-page app as the [Server.Fallback]. Build files are trusted; use os.Root.FS for on-disk confinement.
func (s *Server) MountSPA(files fs.FS, cfg spa.Config) error {
	handler, err := spa.New(files, cfg)
	if err != nil {
		return err
	}
	s.Fallback(handler)
	return nil
}
