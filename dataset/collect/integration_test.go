package collect_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kbukum/gokit/dataset/collect"
	"github.com/kbukum/gokit/dataset/payload"
	"github.com/kbukum/gokit/dataset/record"
	"github.com/kbukum/gokit/dataset/sample"
	"github.com/kbukum/gokit/dataset/schema"
	"github.com/kbukum/gokit/dataset/stage"
)

func TestCollectRecordsEndToEnd(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	inPath := filepath.Join(dir, "in.jsonl")
	if err := os.WriteFile(inPath, []byte(`{"name":"a"}`+"\n"+`{"name":"b"}`+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(dir, "out.jsonl")

	s, err := schema.Compile(schema.JSON{
		"type":     "object",
		"required": []any{"name"},
	})
	if err != nil {
		t.Fatal(err)
	}

	c := collect.New(
		collect.WithSources(record.NewFileSource("in", inPath, record.FormatJSONLines, payload.DefaultLimits())),
		collect.WithValidator(s.Validator()),
		collect.WithTargets[record.Record](record.NewFileTarget("out", outPath, record.FormatJSONLines)),
		collect.WithConfig[record.Record](collect.Config{OutputDir: dir}),
	)
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.TotalItems != 2 {
		t.Fatalf("TotalItems = %d; want 2", res.TotalItems)
	}
	if _, err := os.Stat(outPath); err != nil {
		t.Fatalf("output not written: %v", err)
	}
}

func TestCollectBlobSamplesEndToEnd(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	outDir := filepath.Join(dir, "out")

	mk := func(name string, label stage.Label, offset int, data string) sample.Item {
		p, err := payload.FromBytes([]byte(data), payload.DefaultLimits())
		if err != nil {
			t.Fatal(err)
		}
		return sample.New(name, label, offset, p)
	}

	c := collect.New(
		collect.WithSources(sample.NewSliceSource("blobs", []sample.Item{
			mk("r0.bin", stage.LabelReal, 0, "r0"),
			mk("a0.bin", stage.LabelAI, 1, "a0"),
			mk("r1.bin", stage.LabelReal, 2, "r1"),
		})),
		collect.WithTargets[sample.Item](sample.NewLocalTarget("local", outDir)),
		collect.WithConfig[sample.Item](collect.Config{OutputDir: dir}),
	)
	res, err := c.Run(context.Background())
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if res.TotalItems != 3 || res.RealItems != 2 || res.AIItems != 1 {
		t.Fatalf("stats = total %d real %d ai %d; want 3/2/1", res.TotalItems, res.RealItems, res.AIItems)
	}
	if _, err := os.Stat(filepath.Join(outDir, "real", "r0.bin")); err != nil {
		t.Fatalf("real item not written: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, "ai", "a0.bin")); err != nil {
		t.Fatalf("ai item not written: %v", err)
	}
}
