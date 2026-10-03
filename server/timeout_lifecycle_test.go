package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kbukum/gokit/server"
)

func TestTimedOutRESTCleanupIsStillDrained(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := server.New(&server.Config{RequestTimeout: 1}, nil)
		cleanup := make(chan struct{})
		s.GinEngine().GET("/slow", func(c *gin.Context) {
			<-c.Request.Context().Done()
			<-cleanup
		})
		s.ApplyMiddleware()
		response := httptest.NewRecorder()
		s.Handler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/slow", http.NoBody))
		if response.Code != 503 {
			t.Fatalf("timeout status: %d", response.Code)
		}
		done := make(chan error, 1)
		go func() { done <- s.Stop(context.Background()) }()
		synctest.Wait()
		select {
		case err := <-done:
			t.Errorf("drain finished before canceled REST handler cleanup: %v", err)
			close(cleanup)
		default:
			close(cleanup)
			select {
			case err := <-done:
				if err != nil {
					t.Error(err)
				}
			case <-time.After(time.Second):
				t.Fatal("drain did not observe REST handler completion")
			}
		}
	})
}
