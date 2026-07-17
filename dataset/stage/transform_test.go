package stage

import (
	"context"
	"errors"
	"testing"

	"github.com/kbukum/gokit/dataset/record"
	"github.com/kbukum/gokit/stream"
)

func tagTransform(drop string) Transform[record.Record, record.Record] {
	return TransformFunc[record.Record, record.Record]{
		FuncName: "tag",
		Fn: func(_ context.Context, in record.Record) (record.Record, bool, error) {
			if v, _ := in.Get("name"); v == drop {
				return record.Record{}, false, nil
			}
			fields := in.Fields()
			fields["tagged"] = true
			return record.New(fields), true, nil
		},
	}
}

func TestApplyTransformMapsAndDrops(t *testing.T) {
	t.Parallel()
	p := recordPipeline(
		map[string]record.Value{"name": "alice"},
		map[string]record.Value{"name": "bob"},
	)
	out := ApplyTransform(p, tagTransform("bob"))
	records, err := stream.Collect(context.Background(), out)
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records; want 1 (bob dropped)", len(records))
	}
	if v, _ := records[0].Get("tagged"); v != true {
		t.Fatalf("record not tagged: %+v", records[0])
	}
}

func TestApplyTransformPropagatesError(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("boom")
	p := recordPipeline(map[string]record.Value{"name": "x"})
	out := ApplyTransform(p, TransformFunc[record.Record, record.Record]{
		FuncName: "err",
		Fn: func(_ context.Context, _ record.Record) (record.Record, bool, error) {
			return record.Record{}, false, sentinel
		},
	})
	_, err := stream.Collect(context.Background(), out)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected sentinel error, got %v", err)
	}
}

func TestTransformFuncName(t *testing.T) {
	t.Parallel()
	tf := TransformFunc[record.Record, record.Record]{FuncName: "n"}
	if tf.Name() != "n" {
		t.Fatalf("Name = %q; want n", tf.Name())
	}
}
