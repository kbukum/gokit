package embedded_test

import (
	"testing"

	"github.com/kbukum/gokit/git/embedded"
)

func TestConfigGetAndGetAll(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	createRemote(t, dir)
	runGit(t, dir, "config", "--add", "remote.origin.fetch", "+refs/tags/*:refs/tags/*")
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}

	got, err := repo.ConfigGet("remote.origin.fetch")
	if err != nil {
		t.Fatalf("ConfigGet() error: %v", err)
	}
	if got != "+refs/tags/*:refs/tags/*" {
		t.Fatalf("ConfigGet() = %q", got)
	}
	gotAll, err := repo.ConfigGetAll("remote.origin.fetch")
	if err != nil {
		t.Fatalf("ConfigGetAll() error: %v", err)
	}
	if len(gotAll) != 2 {
		t.Fatalf("ConfigGetAll() returned %d values, want 2", len(gotAll))
	}
}

func TestConfigErrors(t *testing.T) {
	t.Parallel()

	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"missing", "remote..url"} {
		if _, err := repo.ConfigGet(key); err == nil {
			t.Fatalf("ConfigGet(%q) expected error", key)
		}
		if _, err := repo.ConfigGetAll(key); err == nil {
			t.Fatalf("ConfigGetAll(%q) expected error", key)
		}
		if err := repo.ConfigSet(key, "value"); err == nil {
			t.Fatalf("ConfigSet(%q) expected error", key)
		}
	}
	if _, err := repo.ConfigGet("tool.missing"); err == nil {
		t.Fatal("ConfigGet(missing value) expected error")
	}
}

func TestConfigSet(t *testing.T) {
	t.Parallel()
	dir := initTestRepo(t)
	repo, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.ConfigSet("tooling.editor", "vim"); err != nil {
		t.Fatalf("ConfigSet() error: %v", err)
	}
	got, err := repo.ConfigGet("tooling.editor")
	if err != nil {
		t.Fatalf("ConfigGet() error: %v", err)
	}
	if got != "vim" {
		t.Fatalf("ConfigGet() = %q, want vim", got)
	}
	reopened, err := embedded.Open(dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, err = reopened.ConfigGet("tooling.editor")
	if err != nil {
		t.Fatalf("ConfigGet() after reopen error: %v", err)
	}
	if got != "vim" {
		t.Fatalf("ConfigGet() after reopen = %q, want vim", got)
	}
}
