package middleware_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kbukum/gokit/server/middleware"
)

func TestTimeoutResponseBufferBoundary(t *testing.T) {
	t.Parallel()
	const limit = 10 * 1024 * 1024
	for _, size := range []int{limit, limit + 1} {
		writes := make(chan error, 1)
		handler := middleware.Timeout(time.Second)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, err := w.Write(make([]byte, size))
			writes <- err
		}))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
		writeErr := <-writes
		if size == limit {
			if writeErr != nil || response.Body.Len() != limit {
				t.Fatalf("exact boundary failed: %v, %d", writeErr, response.Body.Len())
			}
		} else if writeErr == nil || response.Code != 503 {
			t.Fatalf("oversized buffered response accepted: %v, %d", writeErr, response.Code)
		}
	}
}

func TestTimeoutRejectsWritesAfterCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var writer http.ResponseWriter
		release := make(chan struct{})
		handler := middleware.Timeout(time.Millisecond)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			writer = w
			<-release
		}))
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", http.NoBody))
		synctest.Wait()
		if _, err := writer.Write([]byte("late response")); !errors.Is(err, http.ErrHandlerTimeout) {
			t.Errorf("late write: %v", err)
		}
		close(release)
	})
}
