package storage

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"testing"
	"time"
)

// mockStorage implements Storage for testing.
type mockStorage struct {
	data   map[string][]byte
	failOn string // method name to fail on
}

func newMockStorage() *mockStorage {
	return &mockStorage{data: make(map[string][]byte)}
}

func (m *mockStorage) Upload(_ context.Context, path string, reader io.Reader) error {
	if m.failOn == "upload" {
		return fmt.Errorf("mock upload error")
	}
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	m.data[path] = data
	return nil
}

func (m *mockStorage) Download(_ context.Context, path string) (io.ReadCloser, error) {
	if m.failOn == "download" {
		return nil, fmt.Errorf("mock download error")
	}
	data, ok := m.data[path]
	if !ok {
		return nil, fmt.Errorf("not found: %s", path)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (m *mockStorage) Delete(_ context.Context, path string) error {
	if m.failOn == "delete" {
		return fmt.Errorf("mock delete error")
	}
	delete(m.data, path)
	return nil
}

func (m *mockStorage) Exists(_ context.Context, path string) (bool, error) {
	if m.failOn == "exists" {
		return false, fmt.Errorf("mock exists error")
	}
	_, ok := m.data[path]
	return ok, nil
}

func (m *mockStorage) URL(_ context.Context, path string) (string, error) {
	return "https://example.com/" + path, nil
}

func (m *mockStorage) List(_ context.Context, prefix string) ([]FileInfo, error) {
	var files []FileInfo
	for k, v := range m.data {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			files = append(files, FileInfo{Path: k, Size: int64(len(v)), LastModified: time.Now()})
		}
	}
	return files, nil
}

// --- UploadProvider tests ---

func TestUploadProvider_Name(t *testing.T) {
	p := NewUploadProvider("s3-upload", newMockStorage())
	if got := p.Name(); got != "s3-upload" {
		t.Errorf("Name() = %q, want %q", got, "s3-upload")
	}
}

func TestUploadProvider_IsAvailable(t *testing.T) {
	p := NewUploadProvider("test", newMockStorage())
	if !p.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=true")
	}
	p2 := NewUploadProvider("test", nil)
	if p2.IsAvailable(context.Background()) {
		t.Error("expected IsAvailable=false with nil storage")
	}
}

func TestUploadProvider_Execute_Success(t *testing.T) {
	ms := newMockStorage()
	p := NewUploadProvider("test", ms)

	_, err := p.Execute(context.Background(), UploadRequest{
		Path:   "test.txt",
		Reader: bytes.NewReader([]byte("hello")),
	})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if string(ms.data["test.txt"]) != "hello" {
		t.Errorf("expected data 'hello', got %q", string(ms.data["test.txt"]))
	}
}

func TestUploadProvider_Execute_Error(t *testing.T) {
	ms := newMockStorage()
	ms.failOn = "upload"
	p := NewUploadProvider("test", ms)

	_, err := p.Execute(context.Background(), UploadRequest{
		Path:   "test.txt",
		Reader: bytes.NewReader([]byte("hello")),
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- DownloadProvider tests ---

func TestDownloadProvider_Execute_Success(t *testing.T) {
	ms := newMockStorage()
	ms.data["file.txt"] = []byte("content")
	p := NewDownloadProvider("test", ms)

	resp, err := p.Execute(context.Background(), DownloadRequest{Path: "file.txt"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if string(data) != "content" {
		t.Errorf("expected 'content', got %q", string(data))
	}
}

func TestDownloadProvider_Execute_NotFound(t *testing.T) {
	ms := newMockStorage()
	p := NewDownloadProvider("test", ms)

	_, err := p.Execute(context.Background(), DownloadRequest{Path: "missing.txt"})
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

// --- DeleteProvider tests ---

func TestDeleteProvider_Execute_Success(t *testing.T) {
	ms := newMockStorage()
	ms.data["file.txt"] = []byte("content")
	p := NewDeleteProvider("test", ms)

	_, err := p.Execute(context.Background(), DeleteRequest{Path: "file.txt"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if _, ok := ms.data["file.txt"]; ok {
		t.Error("expected file to be deleted")
	}
}

func TestDeleteProvider_Execute_Error(t *testing.T) {
	ms := newMockStorage()
	ms.failOn = "delete"
	p := NewDeleteProvider("test", ms)

	_, err := p.Execute(context.Background(), DeleteRequest{Path: "file.txt"})
	if err == nil {
		t.Fatal("expected error")
	}
}

// --- ExistsProvider tests ---

func TestExistsProvider_Execute_Exists(t *testing.T) {
	ms := newMockStorage()
	ms.data["file.txt"] = []byte("content")
	p := NewExistsProvider("test", ms)

	resp, err := p.Execute(context.Background(), ExistsRequest{Path: "file.txt"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if !resp.Exists {
		t.Error("expected Exists=true")
	}
}

func TestExistsProvider_Execute_NotExists(t *testing.T) {
	ms := newMockStorage()
	p := NewExistsProvider("test", ms)

	resp, err := p.Execute(context.Background(), ExistsRequest{Path: "missing.txt"})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if resp.Exists {
		t.Error("expected Exists=false")
	}
}

func TestExistsProvider_Execute_Error(t *testing.T) {
	ms := newMockStorage()
	ms.failOn = "exists"
	p := NewExistsProvider("test", ms)

	_, err := p.Execute(context.Background(), ExistsRequest{Path: "file.txt"})
	if err == nil {
		t.Fatal("expected error")
	}
}
