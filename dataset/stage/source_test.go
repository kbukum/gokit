package stage

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/dataset/record"
	"github.com/kbukum/gokit/stream"
)

type keyedSource struct {
	Source[record.Record]
	key string
}

func (k keyedSource) CacheKey() string { return k.key }

func TestSliceSource(t *testing.T) {
	t.Parallel()
	src := NewSliceSource("s", []record.Record{record.New(map[string]record.Value{"a": 1})})
	if src.Name() != "s" {
		t.Fatalf("Name = %q; want s", src.Name())
	}
	records, err := stream.Collect(context.Background(), src.Stream(context.Background()))
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("got %d records; want 1", len(records))
	}
}

func TestSliceSourceMaxItems(t *testing.T) {
	t.Parallel()
	src := NewSliceSource("s", []record.Record{{}, {}})
	n, ok := MaxItems(src)
	if !ok || n != 2 {
		t.Fatalf("MaxItems = %d, %v; want 2, true", n, ok)
	}
}

func TestCacheKeyDefaultsToName(t *testing.T) {
	t.Parallel()
	src := NewSliceSource("plain", []record.Record{})
	if CacheKey(src) != "plain" {
		t.Fatalf("CacheKey = %q; want plain", CacheKey(src))
	}
}

func TestCacheKeyUsesKeyed(t *testing.T) {
	t.Parallel()
	src := keyedSource{Source: NewSliceSource("plain", []record.Record{}), key: "fingerprint"}
	if CacheKey[record.Record](src) != "fingerprint" {
		t.Fatalf("CacheKey = %q; want fingerprint", CacheKey[record.Record](src))
	}
}
