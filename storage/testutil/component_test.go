package testutil

import (
	"context"
	"io"
	"strings"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/storage"
	"github.com/kbukum/gokit/testutil"
)

func TestComponent_Interfaces(t *testing.T) {
	comp := NewComponent()
	var _ component.Component = comp
	var _ testutil.TestComponent = comp
	var _ storage.Storage = comp
}

func TestComponent_Lifecycle(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()

	if comp.Storage() != nil {
		t.Error("Storage() should be nil before Start")
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if comp.Storage() == nil {
		t.Error("Storage() should not be nil after Start")
	}

	health := comp.Health(ctx)
	if health.Status != component.StatusHealthy {
		t.Errorf("Health = %q, want %q", health.Status, component.StatusHealthy)
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
}

func TestComponent_UploadDownload(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()
	comp.Start(ctx)
	defer comp.Stop(ctx)

	if err := comp.Upload(ctx, "test.txt", strings.NewReader("hello world")); err != nil {
		t.Fatalf("Upload failed: %v", err)
	}

	exists, _ := comp.Exists(ctx, "test.txt")
	if !exists {
		t.Error("Exists should return true after Upload")
	}

	rc, err := comp.Download(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}
	defer rc.Close()

	data, _ := io.ReadAll(rc)
	if string(data) != "hello world" {
		t.Errorf("Download = %q, want %q", string(data), "hello world")
	}
}

func TestComponent_DeleteListURL(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()
	comp.Start(ctx)
	defer comp.Stop(ctx)

	comp.Upload(ctx, "dir/a.txt", strings.NewReader("a"))
	comp.Upload(ctx, "dir/b.txt", strings.NewReader("b"))
	comp.Upload(ctx, "other.txt", strings.NewReader("c"))

	files, _ := comp.List(ctx, "dir/")
	if len(files) != 2 {
		t.Errorf("List(dir/) = %d files, want 2", len(files))
	}

	url, _ := comp.URL(ctx, "dir/a.txt")
	if url != "mem://dir/a.txt" {
		t.Errorf("URL = %q, want %q", url, "mem://dir/a.txt")
	}

	comp.Delete(ctx, "dir/a.txt")
	exists, _ := comp.Exists(ctx, "dir/a.txt")
	if exists {
		t.Error("Exists should return false after Delete")
	}
}

func TestComponent_ResetSnapshotRestore(t *testing.T) {
	comp := NewComponent()
	ctx := context.Background()
	comp.Start(ctx)
	defer comp.Stop(ctx)

	comp.Upload(ctx, "a.txt", strings.NewReader("data-a"))
	comp.Upload(ctx, "b.txt", strings.NewReader("data-b"))

	snap, err := comp.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() failed: %v", err)
	}

	// Modify state
	comp.Upload(ctx, "c.txt", strings.NewReader("data-c"))
	comp.Delete(ctx, "a.txt")

	// Restore
	if err := comp.Restore(ctx, snap); err != nil {
		t.Fatalf("Restore() failed: %v", err)
	}

	exists, _ := comp.Exists(ctx, "a.txt")
	if !exists {
		t.Error("'a.txt' should exist after Restore")
	}
	exists, _ = comp.Exists(ctx, "c.txt")
	if exists {
		t.Error("'c.txt' should not exist after Restore")
	}

	// Reset
	if err := comp.Reset(ctx); err != nil {
		t.Fatalf("Reset() failed: %v", err)
	}
	files, _ := comp.List(ctx, "")
	if len(files) != 0 {
		t.Errorf("List after Reset = %d files, want 0", len(files))
	}
}
