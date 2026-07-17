package stage

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/stream"
)

func tagTransform(drop string) Transform[row, row] {
	return TransformFunc[row, row]{
		FuncName: "tag",
		Fn: func(_ context.Context, in row) (row, bool, error) {
			if in["name"] == drop {
				return nil, false, nil
			}
			out := row{}
			for k, v := range in {
				out[k] = v
			}
			out["tagged"] = true
			return out, true, nil
		},
	}
}

func TestApplyTransformMapsAndDrops(t *testing.T) {
	t.Parallel()
	p := rowPipeline(
		row{"name": "alice"},
		row{"name": "bob"},
	)
	out := ApplyTransform(p, tagTransform("bob"))
	rows, err := stream.Collect(context.Background(), out)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows; want 1 (bob dropped)", len(rows))
	}
	if rows[0]["tagged"] != true {
		t.Fatalf("row not tagged: %+v", rows[0])
	}
}

func TestApplyTransformPropagatesError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	p := rowPipeline(row{"name": "x"})
	out := ApplyTransform(p, TransformFunc[row, row]{
		FuncName: "err",
		Fn: func(_ context.Context, _ row) (row, bool, error) {
			return nil, false, sentinel
		},
	})
	_, err := stream.Collect(context.Background(), out)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestTransformFuncName(t *testing.T) {
	t.Parallel()
	tf := TransformFunc[row, row]{FuncName: "n"}
	if tf.Name() != "n" {
		t.Fatalf("Name = %q; want n", tf.Name())
	}
}
