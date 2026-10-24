package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/workload"
)

// maxBody bounds each response. A model's show response carries its template and tensor summary, so it is larger
// than a tag list entry but stays well under this.
const maxBody = 4 << 20

// Error is a non-2xx answer from Ollama. It carries the status only: a response body can echo credentials.
type Error struct {
	Status int
}

func (e *Error) Error() string { return fmt.Sprintf("ollama: unexpected status %d", e.Status) }

// Lister lists and describes the models an Ollama server holds.
type Lister struct {
	http         *httpclient.Adapter
	maxDescribed int
	concurrency  int
}

var _ workload.ModelLister = (*Lister)(nil)

// New builds a lister from a validated copy of cfg.
func New(cfg Config) (*Lister, error) {
	c, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	hc, err := httpclient.New(httpclient.Config{
		Name:                 "ollama",
		BaseURL:              c.BaseURL,
		Timeout:              c.Timeout,
		Auth:                 c.Auth,
		EndpointPolicy:       c.EndpointPolicy,
		MaxResponseBodyBytes: maxBody,
		DefaultHeaders:       map[string]string{"Accept": "application/json"},
	})
	if err != nil {
		return nil, fmt.Errorf("ollama: build http client: %w", err)
	}
	return &Lister{http: hc, maxDescribed: c.MaxDescribed, concurrency: c.Concurrency}, nil
}

type details struct {
	Format            string `json:"format"`
	Family            string `json:"family"`
	ParameterSize     string `json:"parameter_size"`
	QuantizationLevel string `json:"quantization_level"`
}

type tagModel struct {
	Name       string    `json:"name"`
	ModifiedAt time.Time `json:"modified_at"`
	Size       int64     `json:"size"`
	Digest     string    `json:"digest"`
	Details    details   `json:"details"`
}

type showResponse struct {
	ModelInfo    map[string]any `json:"model_info"`
	Capabilities []string       `json:"capabilities"`
}

// ListModels reads GET /api/tags and describes up to MaxDescribed models with POST /api/show, at most Concurrency at
// once, keeping the tag list's order. A model deleted between the two calls is skipped. Any other show failure, such
// as an error status or a request timeout, leaves that model with what /api/tags states, so one slow or broken model
// never hides the rest. ctx's deadline bounds describing the same way: models not described by then keep their tag
// facts. Canceling ctx, or a deadline before the tag list arrives, fails the listing.
func (l *Lister) ListModels(ctx context.Context) ([]workload.ModelInfo, error) {
	var list struct {
		Models *[]tagModel `json:"models"`
	}
	if err := l.call(ctx, http.MethodGet, "/api/tags", nil, &list); err != nil {
		return nil, err
	}
	if list.Models == nil {
		return nil, errors.New("ollama: tag list has no models array")
	}
	tagged := *list.Models
	infos := make([]workload.ModelInfo, len(tagged))
	gone := make([]bool, len(tagged))
	for i := range tagged {
		infos[i] = describeTag(&tagged[i])
	}
	l.describe(ctx, tagged[:min(len(tagged), l.maxDescribed)], infos, gone)
	if err := ctx.Err(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		return nil, fmt.Errorf("ollama: list models: %w", err)
	}
	out := make([]workload.ModelInfo, 0, len(infos))
	for i := range infos {
		if !gone[i] {
			out = append(out, infos[i])
		}
	}
	return out, nil
}

// describe applies each model's show response to infos[i], or marks gone[i] when the model no longer exists. It
// returns once every call it started has finished.
func (l *Lister) describe(ctx context.Context, tagged []tagModel, infos []workload.ModelInfo, gone []bool) {
	slots := make(chan struct{}, l.concurrency)
	var wg sync.WaitGroup
	for i := range tagged {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
			wg.Wait()
			return
		}
		wg.Go(func() {
			defer func() { <-slots }()
			var show showResponse
			err := l.call(ctx, http.MethodPost, "/api/show", map[string]string{"model": tagged[i].Name}, &show)
			var apiErr *Error
			switch {
			case errors.As(err, &apiErr) && apiErr.Status == http.StatusNotFound:
				gone[i] = true
			case err == nil:
				applyShow(&infos[i].Model, show)
			}
		})
	}
	wg.Wait()
}

func (l *Lister) call(ctx context.Context, method, path string, body, into any) error {
	resp, err := l.http.Do(ctx, httpclient.Request{Method: method, Path: path, Body: body})
	if err != nil {
		if resp != nil {
			return &Error{Status: resp.StatusCode}
		}
		return fmt.Errorf("ollama: %s %s: %w", method, path, err)
	}
	if err := json.Unmarshal(resp.Body, into); err != nil {
		return fmt.Errorf("ollama: decode %s: %w", path, err)
	}
	return nil
}

func describeTag(m *tagModel) workload.ModelInfo {
	digest := m.Digest
	if digest != "" && !strings.Contains(digest, ":") {
		digest = "sha256:" + digest
	}
	d := ai.Model{
		Name:     m.Name,
		Provider: ai.ProviderOllama,
		Digest:   digest,
		Variant: ai.Variant{
			Parameters:   m.Details.ParameterSize,
			Quantization: m.Details.QuantizationLevel,
			Format:       m.Details.Format,
			SizeBytes:    max(m.Size, 0),
		},
	}
	if m.Details.Family != "" {
		d.Family = ai.Stated(m.Details.Family, ai.SourceRuntime)
	}
	return workload.ModelInfo{ID: digest, Ref: m.Name, SizeBytes: d.Variant.SizeBytes, Created: m.ModifiedAt, Model: d}
}

// applyShow adds what /api/show states. Ollama derives its capability list from the model itself, so a capability
// it leaves out is unsupported rather than unknown.
func applyShow(d *ai.Model, s showResponse) {
	if arch, ok := s.ModelInfo["general.architecture"].(string); ok && arch != "" {
		if n, ok := s.ModelInfo[arch+".context_length"].(float64); ok && n > 0 && n <= 1<<31 {
			d.Capabilities.ContextLength = ai.Stated(int(n), ai.SourceRuntime)
		}
	}
	if s.Capabilities == nil {
		return
	}
	has := map[string]bool{}
	for _, c := range s.Capabilities {
		has[c] = true
	}
	fact := func(name string) ai.Fact {
		if has[name] {
			return ai.Supported(ai.SourceRuntime)
		}
		return ai.Unsupported(ai.SourceRuntime)
	}
	caps := &d.Capabilities
	caps.Vision, caps.ToolUse, caps.Reasoning, caps.Audio = fact("vision"), fact("tools"), fact("thinking"), fact("audio")
	switch {
	case has["completion"]:
		in := []ai.Modality{ai.ModalityText}
		if has["vision"] {
			in = append(in, ai.ModalityImage)
		}
		if has["audio"] {
			in = append(in, ai.ModalityAudio)
		}
		d.Task = ai.Stated(ai.TaskTextGeneration, ai.SourceRuntime)
		d.Inputs = ai.Stated(in, ai.SourceRuntime)
		d.Outputs = ai.Stated([]ai.Modality{ai.ModalityText}, ai.SourceRuntime)
	case has["embedding"]:
		d.Task = ai.Stated(ai.TaskEmbedding, ai.SourceRuntime)
		d.Inputs = ai.Stated([]ai.Modality{ai.ModalityText}, ai.SourceRuntime)
		d.Outputs = ai.Stated([]ai.Modality{ai.ModalityEmbedding}, ai.SourceRuntime)
	}
}
