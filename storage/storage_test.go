package storage

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Config tests
// ---------------------------------------------------------------------------

func TestConfig_ApplyDefaults_SetsProvider(t *testing.T) {
	c := &Config{}
	c.ApplyDefaults()
	if c.Provider != DefaultProvider {
		t.Errorf("Provider = %q, want %q", c.Provider, DefaultProvider)
	}
}

func TestConfig_ApplyDefaults_SetsMaxFileSize(t *testing.T) {
	c := &Config{}
	c.ApplyDefaults()
	if c.MaxFileSize != DefaultMaxFileSize {
		t.Errorf("MaxFileSize = %d, want %d", c.MaxFileSize, DefaultMaxFileSize)
	}
}

func TestConfig_ApplyDefaults_SetsPresignedTTL(t *testing.T) {
	c := &Config{}
	c.ApplyDefaults()
	if c.PresignedTTL != DefaultPresignedTTL {
		t.Errorf("PresignedTTL = %v, want %v", c.PresignedTTL, DefaultPresignedTTL)
	}
}

func TestConfig_ApplyDefaults_DoesNotOverrideExisting(t *testing.T) {
	c := &Config{
		Provider:     "s3",
		MaxFileSize:  42,
		PresignedTTL: 5 * time.Minute,
	}
	c.ApplyDefaults()
	if c.Provider != "s3" {
		t.Errorf("Provider overridden to %q", c.Provider)
	}
	if c.MaxFileSize != 42 {
		t.Errorf("MaxFileSize overridden to %d", c.MaxFileSize)
	}
	if c.PresignedTTL != 5*time.Minute {
		t.Errorf("PresignedTTL overridden to %v", c.PresignedTTL)
	}
}

func TestConfig_Validate_EmptyProvider_Error(t *testing.T) {
	c := &Config{Provider: ""}
	if err := c.Validate(); err == nil {
		t.Error("expected error for empty provider")
	}
}

func TestConfig_Validate_WithProvider_OK(t *testing.T) {
	c := &Config{Provider: "local"}
	if err := c.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConfig_Validate_NegativeMaxFileSize_ApplyDefaultsFirst(t *testing.T) {
	c := &Config{Provider: "local", MaxFileSize: -1}
	c.ApplyDefaults()
	if c.MaxFileSize != DefaultMaxFileSize {
		t.Errorf("negative MaxFileSize not replaced: got %d", c.MaxFileSize)
	}
}

// ---------------------------------------------------------------------------
// ByteClient tests
// ---------------------------------------------------------------------------

func TestByteClient_UploadDownload(t *testing.T) {
	ms := newMockStorage()
	bc := NewByteClient(ms)
	ctx := context.Background()

	if err := bc.Upload(ctx, "test.txt", []byte("hello bytes")); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	data, err := bc.Download(ctx, "test.txt")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if string(data) != "hello bytes" {
		t.Errorf("got %q, want %q", data, "hello bytes")
	}
}

func TestByteClient_Delete(t *testing.T) {
	ms := newMockStorage()
	bc := NewByteClient(ms)
	ctx := context.Background()

	_ = bc.Upload(ctx, "del.txt", []byte("bye"))
	if err := bc.Delete(ctx, "del.txt"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	exists, _ := bc.Exists(ctx, "del.txt")
	if exists {
		t.Error("file should not exist after delete")
	}
}

func TestByteClient_Exists(t *testing.T) {
	ms := newMockStorage()
	bc := NewByteClient(ms)
	ctx := context.Background()

	exists, _ := bc.Exists(ctx, "nope.txt")
	if exists {
		t.Error("should not exist")
	}
	_ = bc.Upload(ctx, "nope.txt", []byte("data"))
	exists, _ = bc.Exists(ctx, "nope.txt")
	if !exists {
		t.Error("should exist after upload")
	}
}

func TestByteClient_List(t *testing.T) {
	ms := newMockStorage()
	bc := NewByteClient(ms)
	ctx := context.Background()

	_ = bc.Upload(ctx, "prefix/a.txt", []byte("a"))
	_ = bc.Upload(ctx, "prefix/b.txt", []byte("bb"))
	_ = bc.Upload(ctx, "other/c.txt", []byte("ccc"))

	objs, err := bc.List(ctx, "prefix/")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(objs) != 2 {
		t.Errorf("expected 2 objects, got %d", len(objs))
	}
	for _, o := range objs {
		if o.Key == "" {
			t.Error("ObjectInfo.Key should not be empty")
		}
		if o.Size <= 0 {
			t.Errorf("ObjectInfo.Size should be > 0 for %q", o.Key)
		}
	}
}

func TestByteClient_Download_NotFound(t *testing.T) {
	ms := newMockStorage()
	bc := NewByteClient(ms)
	ctx := context.Background()

	_, err := bc.Download(ctx, "missing.txt")
	if err == nil {
		t.Error("expected error downloading missing file")
	}
}

func TestByteClient_EmptyData(t *testing.T) {
	ms := newMockStorage()
	bc := NewByteClient(ms)
	ctx := context.Background()

	if err := bc.Upload(ctx, "empty.bin", []byte{}); err != nil {
		t.Fatalf("Upload: %v", err)
	}
	data, err := bc.Download(ctx, "empty.bin")
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("expected empty, got %d bytes", len(data))
	}
}

// ---------------------------------------------------------------------------
// Factory / New tests
// ---------------------------------------------------------------------------

func TestNew_UnsupportedProvider(t *testing.T) {
	// Verify the factory map doesn't contain this provider.
	if _, ok := factories["nosuchprovider"]; ok {
		t.Error("factory should not exist for unsupported provider")
	}
}

func TestFactoryRegistry_AddRemove(t *testing.T) {
	original := len(factories)
	// We cannot use the real StorageFactory type without importing logger,
	// so just verify the map mechanics
	factories["test-provider"] = nil
	if len(factories) != original+1 {
		t.Error("add did not increase map size")
	}
	delete(factories, "test-provider")
	if len(factories) != original {
		t.Error("cleanup failed")
	}
}

// ---------------------------------------------------------------------------
// DownloadProvider name and availability
// ---------------------------------------------------------------------------

func TestDownloadProvider_Name(t *testing.T) {
	p := NewDownloadProvider("my-dl", newMockStorage())
	if got := p.Name(); got != "my-dl" {
		t.Errorf("Name() = %q, want %q", got, "my-dl")
	}
}

func TestDownloadProvider_IsAvailable_Nil(t *testing.T) {
	p := NewDownloadProvider("test", nil)
	if p.IsAvailable(context.Background()) {
		t.Error("expected false with nil storage")
	}
}

func TestDownloadProvider_Execute_Error(t *testing.T) {
	ms := newMockStorage()
	ms.failOn = "download"
	p := NewDownloadProvider("test", ms)
	_, err := p.Execute(context.Background(), DownloadRequest{Path: "file.txt"})
	if err == nil {
		t.Error("expected error")
	}
}

// ---------------------------------------------------------------------------
// DeleteProvider name and availability
// ---------------------------------------------------------------------------

func TestDeleteProvider_Name(t *testing.T) {
	p := NewDeleteProvider("my-del", newMockStorage())
	if got := p.Name(); got != "my-del" {
		t.Errorf("Name() = %q, want %q", got, "my-del")
	}
}

func TestDeleteProvider_IsAvailable_Nil(t *testing.T) {
	p := NewDeleteProvider("test", nil)
	if p.IsAvailable(context.Background()) {
		t.Error("expected false with nil storage")
	}
}

// ---------------------------------------------------------------------------
// ExistsProvider name and availability
// ---------------------------------------------------------------------------

func TestExistsProvider_Name(t *testing.T) {
	p := NewExistsProvider("my-exists", newMockStorage())
	if got := p.Name(); got != "my-exists" {
		t.Errorf("Name() = %q, want %q", got, "my-exists")
	}
}

func TestExistsProvider_IsAvailable_Nil(t *testing.T) {
	p := NewExistsProvider("test", nil)
	if p.IsAvailable(context.Background()) {
		t.Error("expected false with nil storage")
	}
}

// ---------------------------------------------------------------------------
// FileInfo struct tests
// ---------------------------------------------------------------------------

func TestFileInfo_Fields(t *testing.T) {
	now := time.Now()
	fi := FileInfo{
		Path:         "test/file.txt",
		Size:         1234,
		LastModified: now,
		ContentType:  "text/plain",
	}
	if fi.Path != "test/file.txt" {
		t.Errorf("Path = %q", fi.Path)
	}
	if fi.Size != 1234 {
		t.Errorf("Size = %d", fi.Size)
	}
	if fi.ContentType != "text/plain" {
		t.Errorf("ContentType = %q", fi.ContentType)
	}
}

// ---------------------------------------------------------------------------
// Mock storage: URL and List completeness
// ---------------------------------------------------------------------------

func TestMockStorage_URL(t *testing.T) {
	ms := newMockStorage()
	u, err := ms.URL(context.Background(), "path/to/file.txt")
	if err != nil {
		t.Fatalf("URL: %v", err)
	}
	if u != "https://example.com/path/to/file.txt" {
		t.Errorf("URL = %q", u)
	}
}

func TestMockStorage_List_EmptyPrefix(t *testing.T) {
	ms := newMockStorage()
	ms.data["a.txt"] = []byte("a")
	ms.data["b.txt"] = []byte("b")

	files, err := ms.List(context.Background(), "")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(files) != 2 {
		t.Errorf("expected 2 files, got %d", len(files))
	}
}

// ---------------------------------------------------------------------------
// Upload provider: large payload
// ---------------------------------------------------------------------------

func TestUploadProvider_LargePayload(t *testing.T) {
	ms := newMockStorage()
	p := NewUploadProvider("test", ms)

	data := make([]byte, 1024*1024) // 1MB
	for i := range data {
		data[i] = byte(i % 256)
	}

	_, err := p.Execute(context.Background(), UploadRequest{
		Path:   "large.bin",
		Reader: bytes.NewReader(data),
	})
	if err != nil {
		t.Fatalf("Execute large: %v", err)
	}
	if !bytes.Equal(ms.data["large.bin"], data) {
		t.Error("large payload mismatch")
	}
}

// ---------------------------------------------------------------------------
// Download provider: read full content
// ---------------------------------------------------------------------------

func TestDownloadProvider_FullContentRead(t *testing.T) {
	ms := newMockStorage()
	content := []byte("full content to verify complete reading")
	ms.data["full.txt"] = content
	p := NewDownloadProvider("test", ms)

	resp, err := p.Execute(context.Background(), DownloadRequest{Path: "full.txt"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	defer resp.Body.Close()

	got, _ := io.ReadAll(resp.Body)
	if !bytes.Equal(got, content) {
		t.Errorf("content mismatch: got %d bytes, want %d", len(got), len(content))
	}
}
