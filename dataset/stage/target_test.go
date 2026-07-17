package stage

import (
	"context"
	"testing"
)

func TestSliceTargetPublish(t *testing.T) {
	t.Parallel()
	target := NewSliceTarget[row]("mem")
	p := rowPipeline(row{"a": 1}, row{"a": 2})
	res, err := target.Publish(context.Background(), p)
	if err != nil {
		t.Fatalf("Publish error: %v", err)
	}
	if res.RecordsPublished != 2 {
		t.Fatalf("RecordsPublished = %d; want 2", res.RecordsPublished)
	}
	if res.TargetName != "mem" {
		t.Fatalf("TargetName = %q; want mem", res.TargetName)
	}
	if len(target.Records()) != 2 {
		t.Fatalf("target holds %d rows; want 2", len(target.Records()))
	}
}

func TestSliceTargetName(t *testing.T) {
	t.Parallel()
	if NewSliceTarget[row]("x").Name() != "x" {
		t.Fatal("Name mismatch")
	}
}
