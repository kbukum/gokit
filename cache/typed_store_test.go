package cache

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTypedStoreWithoutPrefixAndErrorPaths(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemoryStore(MemoryConfig{})
	typed := NewTypedStore[testState](store, "")
	state := testState{Count: 3}
	if err := typed.Save(ctx, "plain", &state, 0); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, ok, err := store.Get(ctx, "plain"); err != nil || !ok {
		t.Fatalf("Get unprefixed key ok=%v err=%v", ok, err)
	}

	loadErr := errors.New("load failed")
	failing := NewTypedStore[testState](&typedFailingStore{getErr: loadErr}, "")
	if _, err := failing.Load(ctx, "k"); !errors.Is(err, loadErr) {
		t.Fatalf("Load error = %v, want %v", err, loadErr)
	}

	if err := store.Set(ctx, "bad-json", []byte("{"), 0); err != nil {
		t.Fatalf("Set bad JSON: %v", err)
	}
	if _, err := typed.Load(ctx, "bad-json"); err == nil || !strings.Contains(err.Error(), "unmarshal") {
		t.Fatalf("Load invalid JSON error = %v", err)
	}

	saveErr := errors.New("save failed")
	if err := NewTypedStore[testState](&typedFailingStore{setErr: saveErr}, "").Save(ctx, "k", &state, 0); !errors.Is(err, saveErr) {
		t.Fatalf("Save error = %v, want %v", err, saveErr)
	}
	deleteErr := errors.New("delete failed")
	if err := NewTypedStore[testState](&typedFailingStore{deleteErr: deleteErr}, "").Delete(ctx, "k"); !errors.Is(err, deleteErr) {
		t.Fatalf("Delete error = %v, want %v", err, deleteErr)
	}
}

func TestTypedStoreSaveReportsMarshalError(t *testing.T) {
	t.Parallel()

	typed := NewTypedStore[unmarshalableState](NewMemoryStore(MemoryConfig{}), "")
	value := unmarshalableState{Fn: func() {}}
	if err := typed.Save(context.Background(), "k", &value, 0); err == nil || !strings.Contains(err.Error(), "marshal") {
		t.Fatalf("Save unmarshalable error = %v", err)
	}
}

func FuzzTypedStoreRoundTrip(f *testing.F) {
	f.Add("prefix", "key", 1)
	f.Add("", "", -1)
	f.Fuzz(func(t *testing.T, prefix, key string, count int) {
		store := NewMemoryStore(MemoryConfig{})
		typed := NewTypedStore[testState](store, prefix)
		want := testState{Count: count}
		if err := typed.Save(context.Background(), key, &want, 0); err != nil {
			t.Fatalf("Save: %v", err)
		}
		got, err := typed.Load(context.Background(), key)
		if err != nil {
			t.Fatalf("Load: %v", err)
		}
		if got == nil || got.Count != want.Count {
			t.Fatalf("Load = %+v, want %+v", got, want)
		}
	})
}

type typedFailingStore struct {
	getErr    error
	setErr    error
	deleteErr error
}

func (s *typedFailingStore) Get(context.Context, string) (value []byte, found bool, err error) {
	return nil, false, s.getErr
}

func (s *typedFailingStore) Set(context.Context, string, []byte, time.Duration) error {
	return s.setErr
}
func (s *typedFailingStore) Delete(context.Context, string) error         { return s.deleteErr }
func (s *typedFailingStore) Exists(context.Context, string) (bool, error) { return false, nil }

type unmarshalableState struct {
	Fn func() `json:"fn"`
}
