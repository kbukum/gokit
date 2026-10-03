package server

import (
	"io/fs"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/server/spa"
)

// MountSPA installs a fallback behind registered Gin and mounted RPC routes. Build files are trusted; use os.Root.FS for on-disk confinement.
func (s *Server) MountSPA(files fs.FS, cfg spa.Config) error {
	handler, err := spa.New(files, cfg)
	if err != nil {
		return err
	}
	s.engine.NoRoute(func(c *gin.Context) { handler.ServeHTTP(c.Writer, c.Request) })
	return nil
}
