package testutil

import (
	"context"
	"errors"
	"net/http/httptest"
	"time"
)

// CloseHTTPServer drains an owned loopback server, then closes remaining connections on cancellation or deadline. The drain failure remains inspectable. Handlers must honor request cancellation; arbitrary Go code cannot be forcibly stopped.
func CloseHTTPServer(ctx context.Context, srv *httptest.Server) error {
	if srv == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	err := srv.Config.Shutdown(ctx)
	if err != nil {
		err = errors.Join(err, srv.Config.Close())
		srv.CloseClientConnections()
	}
	srv.Close()
	return err
}
