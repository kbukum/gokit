package stage

import (
	"context"
	"testing"

	"github.com/kbukum/gokit/stream"
)

type keyedSource struct {
	Source[row]
	key string
}

func (k keyedSource) CacheKey() string { return k.key }

func TestSliceSource(t *testing.T) {
	t.Parallel()
	src := NewSliceSource("s", []row{{"a": 1}})
	if src.Name() != "s" {
		t.Fatalf("Name = %q; want s", src.Name())
	}
	rows, err := stream.Collect(context.Background(), src.Stream(context.Background()))
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("got %d rows; want 1", len(rows))
	}
}

func TestSliceSourceMaxItems(t *testing.T) {
	t.Parallel()
	src := NewSliceSource("s", []row{{}, {}})
	n, ok := MaxItems(src)
	if !ok || n != 2 {
		t.Fatalf("MaxItems = %d, %v; want 2, true", n, ok)
	}
}

func TestCacheKeyDefaultsToName(t *testing.T) {
	t.Parallel()
	src := NewSliceSource("plain", []row{})
	if CacheKey(src) != "plain" {
		t.Fatalf("CacheKey = %q; want plain", CacheKey(src))
	}
}

func TestCacheKeyUsesKeyed(t *testing.T) {
	t.Parallel()
	src := keyedSource{Source: NewSliceSource("plain", []row{}), key: "fingerprint"}
	if CacheKey[row](src) != "fingerprint" {
		t.Fatalf("CacheKey = %q; want fingerprint", CacheKey[row](src))
	}
}

type resumableSource struct {
	Source[row]
	offset  int
	fetched int
	called  bool
}

func (r *resumableSource) SetResumeState(offset, fetched int) {
	r.offset, r.fetched, r.called = offset, fetched, true
}

func TestResumeSetsStateWhenResumable(t *testing.T) {
	t.Parallel()
	src := &resumableSource{Source: NewSliceSource("s", []row{})}
	Resume[row](src, 12, 3)
	if !src.called || src.offset != 12 || src.fetched != 3 {
		t.Fatalf("Resume state = {%d, %d, %v}; want {12, 3, true}", src.offset, src.fetched, src.called)
	}
}

func TestResumeNoOpWhenNotResumable(t *testing.T) {
	t.Parallel()
	// A plain source without Resumable must not panic.
	Resume[row](NewSliceSource("s", []row{}), 5, 1)
}
