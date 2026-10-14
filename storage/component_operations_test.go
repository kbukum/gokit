package storage

import (
	"errors"
	"strings"
	"testing"
)

func TestComponentOperationsFollowLifecycle(t *testing.T) {
	t.Parallel()
	probeErr := errors.New("backend failed")
	c := NewComponent(stubRegistry(t, &stubStorage{urlErr: probeErr}), Config{Provider: ProviderLocal, Enabled: true}, nil)
	for _, state := range []string{"unstarted", "started", "stopped"} {
		if state == "started" {
			if err := c.Start(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
		if state == "stopped" {
			if err := c.Stop(t.Context()); err != nil {
				t.Fatal(err)
			}
		}
		uploadErr := c.Upload(t.Context(), "key", strings.NewReader("value"))
		reader, downloadErr := c.Download(t.Context(), "key")
		if reader != nil {
			if err := reader.Close(); err != nil {
				t.Fatal(err)
			}
		}
		deleteErr := c.Delete(t.Context(), "key")
		_, existsErr := c.Exists(t.Context(), "key")
		_, listErr := c.List(t.Context(), "key")
		_, urlErr := c.URL(t.Context(), "key")
		if state == "started" {
			if err := errors.Join(uploadErr, downloadErr, deleteErr, existsErr, listErr); err != nil {
				t.Fatal(err)
			}
			if !errors.Is(urlErr, probeErr) {
				t.Fatal("backend cause lost")
			}
		} else {
			for _, err := range []error{uploadErr, downloadErr, deleteErr, existsErr, listErr, urlErr} {
				if err == nil {
					t.Fatalf("%s operation appeared successful", state)
				}
			}
		}
	}
}
