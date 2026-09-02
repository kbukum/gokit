package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/kbukum/gokit/storage"
)

// Head returns metadata for a single object using the Supabase object info endpoint.
func (s *Storage) Head(ctx context.Context, path string) (storage.FileInfo, error) {
	u := fmt.Sprintf("%s/object/info/%s/%s", s.baseURL, s.bucket, escapeObjectPath(path))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, http.NoBody)
	if err != nil {
		return storage.FileInfo{}, fmt.Errorf("storage: supabase create request: %w", err)
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return storage.FileInfo{}, fmt.Errorf("storage: supabase head: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck // Error on close is safe to ignore for read operations

	if resp.StatusCode == http.StatusNotFound {
		return storage.FileInfo{}, storage.NotFoundError(path)
	}
	if resp.StatusCode >= 400 {
		body := readErrorBody(resp.Body)
		return storage.FileInfo{}, fmt.Errorf("storage: supabase head failed (status %d): %s", resp.StatusCode, string(body))
	}

	var item objectInfo
	if err := decodeJSONBody(resp.Body, &item); err != nil {
		return storage.FileInfo{}, fmt.Errorf("storage: supabase decode response: %w", err)
	}
	return item.toFileInfo(path), nil
}

// Copy duplicates an object within the bucket using the Supabase copy endpoint.
func (s *Storage) Copy(ctx context.Context, srcPath, dstPath string) error {
	if err := storage.CheckDistinctPaths("Copy", srcPath, dstPath); err != nil {
		return err
	}
	return s.objectOp(ctx, "copy", srcPath, dstPath)
}

// Rename moves an object within the bucket using the Supabase move endpoint.
func (s *Storage) Rename(ctx context.Context, srcPath, dstPath string) error {
	if err := storage.CheckDistinctPaths("Rename", srcPath, dstPath); err != nil {
		return err
	}
	return s.objectOp(ctx, "move", srcPath, dstPath)
}

// objectOp issues a copy or move request against the Supabase storage API.
func (s *Storage) objectOp(ctx context.Context, op, srcPath, dstPath string) error {
	u := fmt.Sprintf("%s/object/%s", s.baseURL, op)
	payload := map[string]string{
		"bucketId":       s.bucket,
		"sourceKey":      srcPath,
		"destinationKey": dstPath,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("storage: supabase marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("storage: supabase create request: %w", err)
	}
	s.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: supabase %s: %w", op, err)
	}
	defer resp.Body.Close() //nolint:errcheck // Error on close is safe to ignore for read operations

	if resp.StatusCode == http.StatusNotFound {
		return storage.NotFoundError(srcPath)
	}
	if resp.StatusCode >= 400 {
		respBody := readErrorBody(resp.Body)
		return fmt.Errorf("storage: supabase %s failed (status %d): %s", op, resp.StatusCode, string(respBody))
	}
	return nil
}

// objectInfo mirrors the object metadata returned by the Supabase info endpoint.
type objectInfo struct {
	Name     string `json:"name"`
	Metadata struct {
		Size        int64  `json:"size"`
		ContentType string `json:"mimetype"`
	} `json:"metadata"`
	UpdatedAt string `json:"updated_at"`
}

// toFileInfo converts a decoded info response into a storage.FileInfo. The
// Supabase eTag is not a guaranteed content hash, so it is not surfaced as a
// Checksum; FileInfo.Checksum is left empty until a verified checksum is
// available.
func (o objectInfo) toFileInfo(path string) storage.FileInfo {
	fi := storage.FileInfo{
		Path:        path,
		Size:        o.Metadata.Size,
		ContentType: o.Metadata.ContentType,
	}
	if o.UpdatedAt != "" {
		if t, err := time.Parse(time.RFC3339, o.UpdatedAt); err == nil {
			fi.LastModified = t
		}
	}
	return fi
}
