package testutil

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/component"
	"github.com/kbukum/gokit/git"
	gotestutil "github.com/kbukum/gokit/testutil"
)

func TestComponent_Interfaces(t *testing.T) {
	comp := NewComponent()
	var _ component.Component = comp
	var _ gotestutil.TestComponent = comp
}

func TestComponent_Lifecycle(t *testing.T) {
	ctx := context.Background()
	comp := NewComponent()

	if comp.Root() != "" {
		t.Errorf("Root() before Start = %q, want empty", comp.Root())
	}
	if comp.Repo() != nil {
		t.Fatal("Repo() before Start should be nil")
	}

	if err := comp.Start(ctx); err != nil {
		t.Fatalf("Start() failed: %v", err)
	}

	if comp.Root() == "" {
		t.Fatal("Root() after Start() returned empty")
	}
	if comp.Repo() == nil {
		t.Fatal("Repo() after Start() returned nil")
	}

	health := comp.Health(ctx)
	if health.Status != component.StatusHealthy {
		t.Fatalf("Health().Status = %q, want %q", health.Status, component.StatusHealthy)
	}

	if _, err := comp.Repo().Exec("rev-parse", "--is-inside-work-tree"); err != nil {
		t.Fatalf("Exec() failed: %v", err)
	}

	root := comp.Root()
	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("root stat error = %v, want not exist", err)
	}
}

func TestComponent_ResetSnapshotRestore(t *testing.T) {
	ctx := context.Background()
	comp := NewComponent()
	gotestutil.T(t).Setup(comp)

	firstFile := filepath.Join(comp.Root(), "README.md")
	if err := os.WriteFile(firstFile, []byte("one\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	snapshot, err := comp.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() failed: %v", err)
	}

	if err := os.Remove(firstFile); err != nil {
		t.Fatalf("Remove() failed: %v", err)
	}
	secondFile := filepath.Join(comp.Root(), "notes.txt")
	if err := os.WriteFile(secondFile, []byte("two\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}

	if err := comp.Restore(ctx, snapshot); err != nil {
		t.Fatalf("Restore() failed: %v", err)
	}
	if _, err := os.Stat(firstFile); err != nil {
		t.Fatalf("first file stat failed after restore: %v", err)
	}
	if _, err := os.Stat(secondFile); !os.IsNotExist(err) {
		t.Fatalf("second file stat error = %v, want not exist", err)
	}

	if err := comp.Reset(ctx); err != nil {
		t.Fatalf("Reset() failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(comp.Root(), ".git")); err != nil {
		t.Fatalf(".git stat failed after reset: %v", err)
	}
	if _, err := os.Stat(firstFile); !os.IsNotExist(err) {
		t.Fatalf("first file stat after reset = %v, want not exist", err)
	}
}

func TestComponent_StopRemovesSnapshots(t *testing.T) {
	ctx := context.Background()
	comp := NewComponent()
	gotestutil.T(t).Setup(comp)

	snapshot, err := comp.Snapshot(ctx)
	if err != nil {
		t.Fatalf("Snapshot() failed: %v", err)
	}

	snapshotRoot, ok := snapshot.(string)
	if !ok {
		t.Fatalf("snapshot type = %T, want string", snapshot)
	}
	if _, err := os.Stat(snapshotRoot); err != nil {
		t.Fatalf("snapshot stat failed: %v", err)
	}

	if err := comp.Stop(ctx); err != nil {
		t.Fatalf("Stop() failed: %v", err)
	}
	if _, err := os.Stat(snapshotRoot); !os.IsNotExist(err) {
		t.Fatalf("snapshot stat after Stop = %v, want not exist", err)
	}
}

func TestBuilder(t *testing.T) {
	remoteRoot := filepath.Join(t.TempDir(), "remote.git")
	if _, err := git.InitBare(remoteRoot); err != nil {
		t.Fatalf("InitBare() failed: %v", err)
	}

	builder := NewBuilder(t).
		WithFile("README.md", "hello\n").
		WithCommit("initial commit").
		WithBranch("feature/demo").
		WithCheckout("feature/demo").
		WithTag("v1.0.0", "release").
		WithRemote("origin", remoteRoot)

	repo := builder.Repo()
	if repo == nil {
		t.Fatal("Repo() returned nil")
	}
	if builder.Root() == "" {
		t.Fatal("Root() returned empty")
	}
	if _, err := os.Stat(filepath.Join(builder.Root(), "README.md")); err != nil {
		t.Fatalf("README.md stat failed: %v", err)
	}

	head, err := repo.Head()
	if err != nil {
		t.Fatalf("Head() failed: %v", err)
	}
	if head.Name != "refs/heads/feature/demo" {
		t.Fatalf("Head().Name = %q, want %q", head.Name, "refs/heads/feature/demo")
	}

	tags, err := repo.ListTags()
	if err != nil {
		t.Fatalf("ListTags() failed: %v", err)
	}
	if len(tags) != 1 || tags[0].Name != "v1.0.0" {
		t.Fatalf("ListTags() = %+v, want v1.0.0", tags)
	}

	remotes, err := repo.ListRemotes()
	if err != nil {
		t.Fatalf("ListRemotes() failed: %v", err)
	}
	if len(remotes) != 1 || remotes[0].Name != "origin" || remotes[0].URL != remoteRoot {
		t.Fatalf("ListRemotes() = %+v, want origin %q", remotes, remoteRoot)
	}
}
