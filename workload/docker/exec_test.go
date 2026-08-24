package docker

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestExecSurfacesCreateAndAttachErrors(t *testing.T) {
	t.Parallel()

	t.Run("create error", func(t *testing.T) {
		t.Parallel()

		manager := newTestManager(t, func(req *http.Request) (int, string) {
			if dockerPath(req.URL.Path) != "/containers/id/exec" {
				return http.StatusNotFound, `{}`
			}
			return http.StatusInternalServerError, `{"message":"exec create failed"}`
		})

		_, err := manager.Exec(context.Background(), "id", []string{"echo", "ok"})
		if err == nil || !strings.Contains(err.Error(), "exec create") {
			t.Fatalf("expected exec create error, got %v", err)
		}
	})

	t.Run("attach error", func(t *testing.T) {
		t.Parallel()

		ctx, cancel := context.WithCancel(context.Background())
		manager := newTestManager(t, func(req *http.Request) (int, string) {
			if dockerPath(req.URL.Path) != "/containers/id/exec" {
				return http.StatusNotFound, `{}`
			}
			cancel()
			return http.StatusCreated, `{"Id":"exec-id"}`
		})

		_, err := manager.Exec(ctx, "id", []string{"echo", "ok"})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected cancellation before exec attach, got %v", err)
		}
	})
}

func TestExecHonorsCanceledContext(t *testing.T) {
	t.Parallel()

	manager := newTestManager(t, func(*http.Request) (int, string) {
		return http.StatusCreated, `{"Id":"exec-id"}`
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := manager.Exec(ctx, "id", []string{"echo", "ok"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled exec error, got %v", err)
	}
}
