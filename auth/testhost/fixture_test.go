package testhost

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/codec"
	kitfs "github.com/kbukum/gokit/fs"
)

func TestFixtureIsPrivateBoundedAndReusable(t *testing.T) {
	t.Parallel()
	fixture, err := NewFixture()
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fixture.json")
	data, err := codec.Encode(codec.CompactJSON(), fixture)
	if err != nil {
		t.Fatal(err)
	}
	if err := kitfs.WriteAtomic(path, []byte(data), "fixture"); err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadFixture(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Password != fixture.Password || loaded.ControlToken != fixture.ControlToken || !bytes.Equal(loaded.DigestKey, fixture.DigestKey) {
		t.Fatal("restart changed fixture credentials")
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFixture(path); err == nil {
		t.Fatal("readable fixture secrets accepted")
	}
}

func TestFixtureRejectsMissingOrOversizedMaterial(t *testing.T) {
	t.Parallel()
	for _, content := range []string{"{}", `{"password":"short"}`, string(make([]byte, 4097))} {
		path := filepath.Join(t.TempDir(), "fixture.json")
		if err := kitfs.WriteAtomic(path, []byte(content), "fixture"); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadFixture(path); err == nil {
			t.Fatal("invalid fixture accepted")
		}
	}
}
