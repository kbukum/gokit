package storage

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

type createStore struct {
	*bareStore
	created map[string]string
}

func (c *createStore) Create(_ context.Context, path string, reader io.Reader) error {
	if _, ok := c.created[path]; ok {
		return ExistsError(path)
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	c.created[path] = string(data)
	return nil
}

func TestCreateUsesTheAtomicProvider(t *testing.T) {
	t.Parallel()
	s := &createStore{bareStore: newBareStore(), created: map[string]string{}}
	if err := Create(t.Context(), s, "key", strings.NewReader("first")); err != nil {
		t.Fatal(err)
	}
	err := Create(t.Context(), s, "key", strings.NewReader("second"))
	if !errors.Is(err, ErrExists) || !strings.Contains(err.Error(), "key") {
		t.Fatalf("occupied key was not reported with its path: %v", err)
	}
	if s.created["key"] != "first" {
		t.Fatal("refused create replaced the object")
	}
}

func TestCreateHasNoRacyFallback(t *testing.T) {
	t.Parallel()
	s := newBareStore()
	err := Create(t.Context(), s, "key", strings.NewReader("value"))
	if !errors.Is(err, ErrCreateUnsupported) || !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("unsupported backend was not refused: %v", err)
	}
	if exists, existsErr := s.Exists(t.Context(), "key"); exists || existsErr != nil {
		t.Fatal("unsupported create wrote through a fallback", existsErr)
	}
}

func TestCreateValidatesArguments(t *testing.T) {
	t.Parallel()
	s := &createStore{bareStore: newBareStore(), created: map[string]string{}}
	for name, call := range map[string]func() error{
		"nil store":  func() error { return Create(t.Context(), nil, "key", strings.NewReader("v")) },
		"empty path": func() error { return Create(t.Context(), s, "", strings.NewReader("v")) },
		"nil reader": func() error { return Create(t.Context(), s, "key", nil) },
	} {
		if err := call(); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	if len(s.created) != 0 {
		t.Fatal("invalid create reached the backend")
	}
}
