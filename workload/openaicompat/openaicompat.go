package openaicompat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/workload"
)

const maxBody = 4 << 20

// Error is a non-2xx answer from the server. It carries the status only: a response body can echo credentials.
type Error struct {
	Status int
}

func (e *Error) Error() string { return fmt.Sprintf("openaicompat: unexpected status %d", e.Status) }

// Lister lists the models an OpenAI-compatible server offers.
type Lister struct {
	http *httpclient.Adapter
}

var _ workload.ModelLister = (*Lister)(nil)

// New builds a lister from a validated copy of cfg.
func New(cfg Config) (*Lister, error) {
	c, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	hc, err := httpclient.New(httpclient.Config{
		Name:                 "openaicompat",
		BaseURL:              c.BaseURL,
		Timeout:              c.Timeout,
		Auth:                 c.Auth,
		EndpointPolicy:       c.EndpointPolicy,
		MaxResponseBodyBytes: maxBody,
		DefaultHeaders:       map[string]string{"Accept": "application/json"},
	})
	if err != nil {
		return nil, fmt.Errorf("openaicompat: build http client: %w", err)
	}
	return &Lister{http: hc}, nil
}

type card struct {
	ID          string `json:"id"`
	Created     int64  `json:"created"`
	MaxModelLen int    `json:"max_model_len"`
}

// ListModels reads GET {base}/models.
func (l *Lister) ListModels(ctx context.Context) ([]workload.ModelInfo, error) {
	resp, err := l.http.Do(ctx, httpclient.Request{Method: http.MethodGet, Path: "/models"})
	if err != nil {
		if resp != nil {
			return nil, &Error{Status: resp.StatusCode}
		}
		return nil, fmt.Errorf("openaicompat: list models: %w", err)
	}
	var body struct {
		Data *[]card `json:"data"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, fmt.Errorf("openaicompat: decode models: %w", err)
	}
	if body.Data == nil {
		return nil, errors.New("openaicompat: model list has no data array")
	}
	out := make([]workload.ModelInfo, 0, len(*body.Data))
	for _, c := range *body.Data {
		if c.ID == "" {
			continue
		}
		d := ai.Model{Name: c.ID, Provider: ai.ProviderOpenAICompat}
		if c.MaxModelLen > 0 {
			d.Capabilities.ContextLength = ai.Stated(c.MaxModelLen, ai.SourceRuntime)
		}
		info := workload.ModelInfo{ID: c.ID, Ref: c.ID, Model: d}
		if c.Created > 0 {
			info.Created = time.Unix(c.Created, 0)
		}
		out = append(out, info)
	}
	return out, nil
}
