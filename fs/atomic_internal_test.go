package fs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFailedPersistenceNeverPublishesNewFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	temp := filepath.Join(dir, "temp")
	file, err := os.Create(temp)
	if err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(dir, "secret")
	if err := writeAndPersist(file, []byte("synthetic"), atomicTarget{tempPath: temp, dest: dest, createOnly: true}); !errors.Is(err, os.ErrClosed) {
		t.Fatalf("write cause lost: %v", err)
	}
	if _, err := os.Stat(dest); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed write published a destination: %v", err)
	}
}
