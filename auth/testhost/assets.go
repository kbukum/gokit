package testhost

import (
	"os"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/server/spa"
)

func (h *Host) mountAssets() error {
	if h.config.AssetsDir == "" {
		return nil
	}
	root, err := os.OpenRoot(h.config.AssetsDir)
	if err != nil {
		return err
	}
	h.assets = root
	handler, err := spa.New(root.FS(), spa.Config{ReservedPrefixes: []string{"/auth", "/events", "/_test", "/gokit.auth.v1.IdentityService"}})
	if err != nil {
		return err
	}
	h.server.GinEngine().NoRoute(gin.WrapH(handler))
	return nil
}
