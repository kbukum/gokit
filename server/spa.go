package server

import (
	"fmt"
	"io/fs"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/server/spa"
	"github.com/kbukum/gokit/util"
)

// Fallback serves requests that no Gin route or mounted handler matches, behind the server's middleware. A later call replaces the handler. It returns [ErrInvalidRoute] for a nil or typed-nil handler, so a bad fallback fails at registration instead of on the first unmatched request.
func (s *Server) Fallback(handler http.Handler) error {
	if util.IsNil(handler) {
		return fmt.Errorf("%w: nil fallback handler", ErrInvalidRoute)
	}
	s.engine.NoRoute(func(c *gin.Context) {
		// Gin presets 404 for NoRoute; the fallback decides its own status.
		c.Status(http.StatusOK)
		handler.ServeHTTP(c.Writer, c.Request)
	})
	return nil
}

// MountSPA serves a single-page app as the [Server.Fallback]. Build files are trusted; use os.Root.FS for on-disk confinement.
func (s *Server) MountSPA(files fs.FS, cfg spa.Config) error {
	handler, err := spa.New(files, cfg)
	if err != nil {
		return err
	}
	return s.Fallback(handler)
}
