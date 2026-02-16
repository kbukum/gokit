package supabase

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/skillsenselab/gokit/storage"
)

func init() {
	storage.RegisterFactory(storage.ProviderSupabase, func(cfg storage.Config) (storage.Storage, error) {
		return NewStorage(Config{
			URL:       cfg.URL,
			Bucket:    cfg.Bucket,
			AccessKey: cfg.AccessKey,
			SecretKey: cfg.SecretKey,
		})
	})
}

// Config holds Supabase-specific configuration.
type Config struct {
	// URL is the Supabase project URL (e.g., https://xyz.supabase.co).
	URL string

	// Bucket is the storage bucket name.
	Bucket string

	// AccessKey is used for authentication (optional, reserved for future use).
	AccessKey string

	// SecretKey is the service-role key used as Bearer token.
	SecretKey string
}

// Storage implements storage.Storage using the Supabase Storage REST API.
type Storage struct {
	baseURL    string
	bucket     string
	accessKey  string
	secretKey  string
	httpClient *http.Client
}

// NewStorage creates a new Supabase storage client.
func NewStorage(cfg Config) (*Storage, error) {
	base := strings.TrimRight(cfg.URL, "/") + "/storage/v1"
	return &Storage{
		baseURL:   base,
		bucket:    cfg.Bucket,
		accessKey: cfg.AccessKey,
		secretKey: cfg.SecretKey,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
	}, nil
}

// Upload writes data from reader to Supabase storage.
func (s *Storage) Upload(ctx context.Context, path string, reader io.Reader) error {
	u := fmt.Sprintf("%s/object/%s/%s", s.baseURL, s.bucket, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, reader)
	if err != nil {
		return fmt.Errorf("storage: supabase create request: %w", err)
	}
	s.setHeaders(req)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("x-upsert", "true")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: supabase upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("storage: supabase upload failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// Download returns a reader for the object at the given path.
func (s *Storage) Download(ctx context.Context, path string) (io.ReadCloser, error) {
	u := fmt.Sprintf("%s/object/%s/%s", s.baseURL, s.bucket, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("storage: supabase create request: %w", err)
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage: supabase download: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		resp.Body.Close()
		return nil, fmt.Errorf("storage: file not found: %s", path)
	}
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("storage: supabase download failed (status %d): %s", resp.StatusCode, string(body))
	}
	return resp.Body, nil
}

// Delete removes an object. Returns nil if the object does not exist.
func (s *Storage) Delete(ctx context.Context, path string) error {
	u := fmt.Sprintf("%s/object/%s/%s", s.baseURL, s.bucket, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return fmt.Errorf("storage: supabase create request: %w", err)
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("storage: supabase delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("storage: supabase delete failed (status %d): %s", resp.StatusCode, string(body))
	}
	return nil
}

// Exists checks whether an object exists.
func (s *Storage) Exists(ctx context.Context, path string) (bool, error) {
	u := fmt.Sprintf("%s/object/%s/%s", s.baseURL, s.bucket, path)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, u, nil)
	if err != nil {
		return false, fmt.Errorf("storage: supabase create request: %w", err)
	}
	s.setHeaders(req)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("storage: supabase head: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return false, nil
	}
	if resp.StatusCode >= 400 {
		return false, fmt.Errorf("storage: supabase exists check failed (status %d)", resp.StatusCode)
	}
	return true, nil
}

// URL returns a public URL for the object.
func (s *Storage) URL(_ context.Context, path string) (string, error) {
	return fmt.Sprintf("%s/object/public/%s/%s", s.baseURL, s.bucket, path), nil
}

// List returns metadata for all objects whose path starts with prefix.
func (s *Storage) List(ctx context.Context, prefix string) ([]storage.FileInfo, error) {
	u := fmt.Sprintf("%s/object/list/%s", s.baseURL, s.bucket)

	folder := ""
	search := ""
	if prefix != "" {
		if idx := strings.LastIndex(prefix, "/"); idx >= 0 {
			folder = prefix[:idx+1]
			search = prefix[idx+1:]
		} else {
			search = prefix
		}
	}

	reqBody := map[string]interface{}{
		"prefix": folder,
		"limit":  1000,
	}
	if search != "" {
		reqBody["search"] = search
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("storage: supabase marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("storage: supabase create request: %w", err)
	}
	s.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("storage: supabase list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("storage: supabase list failed (status %d): %s", resp.StatusCode, string(body))
	}

	var items []struct {
		Name     string `json:"name"`
		Metadata struct {
			Size        int64  `json:"size"`
			ContentType string `json:"mimetype"`
		} `json:"metadata"`
		UpdatedAt string `json:"updated_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("storage: supabase decode response: %w", err)
	}

	var files []storage.FileInfo
	for _, item := range items {
		fi := storage.FileInfo{
			Path:        folder + item.Name,
			Size:        item.Metadata.Size,
			ContentType: item.Metadata.ContentType,
		}
		if item.UpdatedAt != "" {
			if t, err := time.Parse(time.RFC3339, item.UpdatedAt); err == nil {
				fi.LastModified = t
			}
		}
		files = append(files, fi)
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})

	return files, nil
}

func (s *Storage) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", s.secretKey))
}

// SignedURL returns a pre-signed URL valid for the specified duration.
func (s *Storage) SignedURL(ctx context.Context, path string, expiry time.Duration) (string, error) {
	u := fmt.Sprintf("%s/object/sign/%s/%s", s.baseURL, s.bucket, path)

	body := fmt.Sprintf(`{"expiresIn": %d}`, int(expiry.Seconds()))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, strings.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("storage: supabase create sign request: %w", err)
	}
	s.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("storage: supabase sign request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("storage: supabase sign failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		SignedURL string `json:"signedURL"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("storage: supabase decode sign response: %w", err)
	}

	if result.SignedURL == "" {
		return "", fmt.Errorf("storage: supabase sign returned empty URL")
	}

	// The signedURL from Supabase is a relative path, prepend the base URL.
	if !strings.HasPrefix(result.SignedURL, "http") {
		return s.baseURL + result.SignedURL, nil
	}
	return result.SignedURL, nil
}

// compile-time check
var _ storage.Storage = (*Storage)(nil)
var _ storage.SignedURLProvider = (*Storage)(nil)
