package lmstudio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/workload"
)

const maxBody = 4 << 20

// Error is a non-2xx answer from LM Studio. It carries the status only: a response body can echo credentials.
type Error struct {
	Status int
}

func (e *Error) Error() string { return fmt.Sprintf("lmstudio: unexpected status %d", e.Status) }

// Lister lists and describes the models an LM Studio server holds.
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
		Name:                 "lmstudio",
		BaseURL:              c.BaseURL,
		Timeout:              c.Timeout,
		Auth:                 c.Auth,
		EndpointPolicy:       c.EndpointPolicy,
		MaxResponseBodyBytes: maxBody,
		DefaultHeaders:       map[string]string{"Accept": "application/json"},
	})
	if err != nil {
		return nil, fmt.Errorf("lmstudio: build http client: %w", err)
	}
	return &Lister{http: hc}, nil
}

type model struct {
	Type         string `json:"type"`
	Key          string `json:"key"`
	Architecture string `json:"architecture"`
	Quantization *struct {
		Name string `json:"name"`
	} `json:"quantization"`
	SizeBytes        int64  `json:"size_bytes"`
	ParamsString     string `json:"params_string"`
	MaxContextLength int    `json:"max_context_length"`
	Format           string `json:"format"`
	Capabilities     *struct {
		Vision            *bool `json:"vision"`
		TrainedForToolUse *bool `json:"trained_for_tool_use"`
		Reasoning         *struct {
			AllowedOptions []string `json:"allowed_options"`
		} `json:"reasoning"`
	} `json:"capabilities"`
}

// ListModels reads GET /api/v1/models.
func (l *Lister) ListModels(ctx context.Context) ([]workload.ModelInfo, error) {
	resp, err := l.http.Do(ctx, httpclient.Request{Method: http.MethodGet, Path: "/api/v1/models"})
	if err != nil {
		if resp != nil {
			return nil, &Error{Status: resp.StatusCode}
		}
		return nil, fmt.Errorf("lmstudio: list models: %w", err)
	}
	var body struct {
		Models *[]model `json:"models"`
	}
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return nil, fmt.Errorf("lmstudio: decode models: %w", err)
	}
	if body.Models == nil {
		return nil, errors.New("lmstudio: model list has no models array")
	}
	out := make([]workload.ModelInfo, 0, len(*body.Models))
	for _, m := range *body.Models {
		if m.Key == "" {
			continue
		}
		d := describe(m)
		out = append(out, workload.ModelInfo{ID: m.Key, Ref: m.Key, SizeBytes: d.Variant.SizeBytes, Model: d})
	}
	return out, nil
}

func describe(m model) ai.Model {
	d := ai.Model{
		Name:     m.Key,
		Provider: ai.ProviderLMStudio,
		Variant:  ai.Variant{Parameters: m.ParamsString, Format: m.Format, SizeBytes: max(m.SizeBytes, 0)},
	}
	if m.Quantization != nil {
		d.Variant.Quantization = m.Quantization.Name
	}
	if m.Architecture != "" {
		d.Family = ai.Stated(m.Architecture, ai.SourceRuntime)
	}
	if m.MaxContextLength > 0 {
		d.Capabilities.ContextLength = ai.Stated(m.MaxContextLength, ai.SourceRuntime)
	}
	text := []ai.Modality{ai.ModalityText}
	switch m.Type {
	case "llm":
		d.Task = ai.Stated(ai.TaskTextGeneration, ai.SourceRuntime)
		d.Outputs = ai.Stated(text, ai.SourceRuntime)
	case "embedding":
		d.Task = ai.Stated(ai.TaskEmbedding, ai.SourceRuntime)
		d.Inputs = ai.Stated(text, ai.SourceRuntime)
		d.Outputs = ai.Stated([]ai.Modality{ai.ModalityEmbedding}, ai.SourceRuntime)
	}
	c := m.Capabilities
	if c == nil {
		return d
	}
	if c.Vision != nil {
		d.Capabilities.Vision = fact(*c.Vision)
		if m.Type == "llm" {
			in := text
			if *c.Vision {
				in = append([]ai.Modality{}, ai.ModalityText, ai.ModalityImage)
			}
			d.Inputs = ai.Stated(in, ai.SourceRuntime)
		}
	}
	if c.TrainedForToolUse != nil {
		d.Capabilities.ToolUse = fact(*c.TrainedForToolUse)
	}
	// A missing reasoning object only means no reasoning setting is exposed, so reasoning stays unknown.
	if c.Reasoning != nil {
		on := false
		for _, opt := range c.Reasoning.AllowedOptions {
			on = on || opt != "off"
		}
		d.Capabilities.Reasoning = fact(on)
	}
	return d
}

func fact(yes bool) ai.Fact {
	if yes {
		return ai.Supported(ai.SourceRuntime)
	}
	return ai.Unsupported(ai.SourceRuntime)
}
