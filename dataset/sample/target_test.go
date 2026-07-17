package sample

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/dataset/payload"
	"github.com/kbukum/gokit/dataset/stage"
	"github.com/kbukum/gokit/stream"
)

func mustPayload(t *testing.T, data string) payload.Payload {
	t.Helper()
	p, err := payload.FromBytes([]byte(data), payload.DefaultLimits())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLocalTargetSplitsRealAndAI(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := NewLocalTarget("t", dir)
	items := stream.FromSlice([]Item{
		New("r.bin", stage.LabelReal, 0, mustPayload(t, "real")),
		New("a.bin", stage.LabelAI, 1, mustPayload(t, "ai")),
	})
	pub, err := target.Publish(context.Background(), items)
	if err != nil {
		t.Fatalf("Publish error: %v", err)
	}
	if pub.RecordsPublished != 2 {
		t.Fatalf("RecordsPublished = %d; want 2", pub.RecordsPublished)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "real", "r.bin")); err != nil || string(got) != "real" {
		t.Fatalf("real file = %q, %v", got, err)
	}
	if got, err := os.ReadFile(filepath.Join(dir, "ai", "a.bin")); err != nil || string(got) != "ai" {
		t.Fatalf("ai file = %q, %v", got, err)
	}
}

func TestLocalTargetRejectsTraversal(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	target := NewLocalTarget("t", dir)
	items := stream.FromSlice([]Item{
		New(filepath.Join("..", "escape.bin"), stage.LabelReal, 0, mustPayload(t, "x")),
	})
	if _, err := target.Publish(context.Background(), items); err == nil {
		t.Fatal("expected traversal to be rejected")
	}
}
