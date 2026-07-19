package qdrant

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/kbukum/gokit/vectorstore"
)

// Register registers a configured Qdrant backend into the given registry.
func Register(reg *vectorstore.FactoryRegistry, cfg Config) error {
	if reg == nil {
		return fmt.Errorf("qdrant: vectorstore registry is nil")
	}
	c := cfg
	c.ApplyDefaults()
	if err := c.Validate(); err != nil {
		return err
	}
	return reg.Register(ProviderName, func(common vectorstore.Config) (vectorstore.Store, error) {
		metric := c.Metric
		if common.Metric != "" {
			metric = common.Metric
		}
		return NewStore(Config{URL: c.URL, APIKey: c.APIKey, Metric: metric, Timeout: c.Timeout})
	})
}

// Store implements vectorstore.Store using Qdrant's REST API.
type Store struct {
	baseURL string
	apiKey  string
	metric  string
	client  *http.Client
}

// NewStore creates a Qdrant vector store from config.
func NewStore(cfg Config) (*Store, error) {
	cfg.ApplyDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &Store{baseURL: strings.TrimRight(cfg.URL, "/"), apiKey: cfg.APIKey, metric: cfg.Metric, client: &http.Client{Timeout: cfg.Timeout}}, nil
}

// EnsureCollection ensures a collection exists, creating it if missing.
func (s *Store) EnsureCollection(ctx context.Context, collection string, dimensions int) error {
	if err := validateCollection(collection); err != nil {
		return err
	}
	distance, err := qdrantDistance(s.metric)
	if err != nil {
		return err
	}
	resp, err := s.do(ctx, http.MethodGet, "/collections/"+collection, nil)
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusNotFound {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return fmt.Errorf("qdrant: close collection response: %w", closeErr)
		}
		body := map[string]map[string]any{"vectors": {"size": dimensions, "distance": distance}}
		createResp, createErr := s.doJSON(ctx, http.MethodPut, "/collections/"+collection, body)
		if createErr != nil {
			return createErr
		}
		defer createResp.Body.Close() //nolint:errcheck // close error on read response is safe to ignore
		return expectStatus(createResp, "create collection")
	}
	defer resp.Body.Close() //nolint:errcheck // close error on read response is safe to ignore
	return expectStatus(resp, "check collection")
}

// Upsert inserts or updates a vector point.
func (s *Store) Upsert(ctx context.Context, collection string, point vectorstore.Point) error {
	if err := validateCollection(collection); err != nil {
		return err
	}
	pid, err := pointIDFromString(point.ID)
	if err != nil {
		return err
	}
	payloadJSON, err := payloadToJSON(point.Payload)
	if err != nil {
		return err
	}
	body := map[string]any{"points": []map[string]any{{"id": pid, "vector": point.Vector, "payload": payloadJSON}}}
	resp, err := s.doJSON(ctx, http.MethodPut, "/collections/"+collection+"/points?wait=true", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // close error on read response is safe to ignore
	return expectStatus(resp, "upsert point")
}

// Search searches for similar vectors.
func (s *Store) Search(ctx context.Context, collection string, query vectorstore.SearchQuery) ([]vectorstore.SearchResult, error) {
	if err := validateCollection(collection); err != nil {
		return nil, err
	}
	if query.Limit < 0 {
		return nil, fmt.Errorf("query limit must be non-negative, got %d", query.Limit)
	}
	body := map[string]any{"vector": query.Vector, "limit": query.Limit, "with_payload": true}
	filterJSON, err := filterToJSON(query.Filter)
	if err != nil {
		return nil, err
	}
	if filterJSON != nil {
		body["filter"] = filterJSON
	}
	resp, err := s.doJSON(ctx, http.MethodPost, "/collections/"+collection+"/points/search", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close() //nolint:errcheck // close error on read response is safe to ignore
	if err := expectStatus(resp, "search points"); err != nil {
		return nil, err
	}
	var decoded struct {
		Result []struct {
			ID      json.RawMessage            `json:"id"`
			Score   float32                    `json:"score"`
			Payload map[string]json.RawMessage `json:"payload"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("qdrant: decode search response: %w", err)
	}
	results := make([]vectorstore.SearchResult, 0, len(decoded.Result))
	for _, point := range decoded.Result {
		id, err := pointIDToString(point.ID)
		if err != nil {
			return nil, err
		}
		payload, err := payloadFromJSON(point.Payload)
		if err != nil {
			return nil, err
		}
		results = append(results, vectorstore.SearchResult{ID: id, Score: point.Score, Payload: payload})
	}
	return results, nil
}

// Delete deletes a point by ID.
func (s *Store) Delete(ctx context.Context, collection, id string) error {
	if err := validateCollection(collection); err != nil {
		return err
	}
	pid, err := pointIDFromString(id)
	if err != nil {
		return err
	}
	resp, err := s.doJSON(ctx, http.MethodPost, "/collections/"+collection+"/points/delete?wait=true", map[string]any{"points": []pointID{pid}})
	if err != nil {
		return err
	}
	defer resp.Body.Close() //nolint:errcheck // close error on read response is safe to ignore
	return expectStatus(resp, "delete point")
}

func (s *Store) doJSON(ctx context.Context, method, path string, body any) (*http.Response, error) {
	encoded, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("qdrant: encode request: %w", err)
	}
	return s.do(ctx, method, path, bytes.NewReader(encoded))
}

func (s *Store) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, s.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("qdrant: create request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if s.apiKey != "" {
		req.Header.Set("api-key", s.apiKey)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qdrant: request failed: %w", err)
	}
	return resp, nil
}

func expectStatus(resp *http.Response, op string) error {
	if resp.StatusCode < 400 {
		return nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("qdrant: %s failed with status %d and unreadable body: %w", op, resp.StatusCode, err)
	}
	return fmt.Errorf("qdrant: %s failed with status %d: %s", op, resp.StatusCode, string(body))
}

func validateCollection(collection string) error {
	if collection == "" || collection == "." || collection == ".." {
		return fmt.Errorf("qdrant: collection must be a non-empty safe URL path segment")
	}
	for _, ch := range collection {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '.' || ch == '_' || ch == '-' {
			continue
		}
		return fmt.Errorf("qdrant: collection must be a non-empty safe URL path segment")
	}
	return nil
}

var _ vectorstore.Store = (*Store)(nil)
