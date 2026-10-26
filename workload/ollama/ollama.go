package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// maxPullLine bounds one line of a pull's progress stream, and maxPullStream the whole stream. A pull reports a
// short status per layer update, so a real stream stays far below both.
const (
	maxPullLine   = 64 << 10
	maxPullStream = 64 << 20
)

// Error is a non-2xx answer from Ollama. It carries the status only: a response body can echo credentials.
type Error struct {
	Status int
}

func (e *Error) Error() string { return fmt.Sprintf("ollama: unexpected status %d", e.Status) }

// Client lists, describes and pulls the models an Ollama server holds.
type Client struct {
	http         *httpclient.Adapter
	maxDescribed int
	concurrency  int
}

var (
	_ workload.ModelLister = (*Client)(nil)
	_ workload.ModelPuller = (*Client)(nil)
)

// New builds a client from a validated copy of cfg.
func New(cfg Config) (*Client, error) {
	c, err := cfg.normalized()
	if err != nil {
		return nil, err
	}
	hc, err := httpclient.New(httpclient.Config{
		Name:           "ollama",
		BaseURL:        c.BaseURL,
		Timeout:        c.Timeout,
		Auth:           c.Auth,
		EndpointPolicy: c.EndpointPolicy,
		Stream: httpclient.StreamConfig{
			TotalTimeout:         c.PullTimeout,
			FirstProgressTimeout: c.PullTimeout,
			IdleTimeout:          c.PullTimeout,
		},
		MaxResponseBodyBytes: maxBody,
		DefaultHeaders:       map[string]string{"Accept": "application/json"},
	})
	if err != nil {
		return nil, fmt.Errorf("ollama: build http client: %w", err)
	}
	return &Client{http: hc, maxDescribed: c.MaxDescribed, concurrency: c.Concurrency}, nil
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
func (c *Client) ListModels(ctx context.Context) ([]workload.ModelInfo, error) {
	var list struct {
		Models *[]tagModel `json:"models"`
	}
	if err := c.call(ctx, http.MethodGet, "/api/tags", nil, &list); err != nil {
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
	c.describe(ctx, tagged[:min(len(tagged), c.maxDescribed)], infos, gone)
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
func (c *Client) describe(ctx context.Context, tagged []tagModel, infos []workload.ModelInfo, gone []bool) {
	slots := make(chan struct{}, c.concurrency)
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
			err := c.call(ctx, http.MethodPost, "/api/show", map[string]string{"model": tagged[i].Name}, &show)
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

// pullLine is one line of POST /api/pull's stream. A layer download states its digest, its total and the bytes so
// far; a failure is an error line.
type pullLine struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
	Error     string `json:"error"`
}

// PullModel downloads ref with POST /api/pull, reporting progress summed over its layers. The pull succeeds only on
// Ollama's final success line: an error line, a truncated stream or an oversized one fails it. An error from progress,
// or canceling ctx, stops the pull. Ollama lists a model only once its pull completes.
func (c *Client) PullModel(ctx context.Context, ref string, progress func(workload.PullProgress) error) error {
	if ref == "" {
		return errors.New("ollama: model ref is required")
	}
	stream, err := c.http.DoStream(ctx, httpclient.Request{
		Method: http.MethodPost,
		Path:   "/api/pull",
		Body:   map[string]any{"model": ref, "stream": true},
	})
	if err != nil {
		var hErr *httpclient.Error
		if errors.As(err, &hErr) && hErr.StatusCode > 0 {
			return &Error{Status: hErr.StatusCode}
		}
		return fmt.Errorf("ollama: pull %q: %w", ref, err)
	}
	defer func() { _ = stream.Close() }()
	if err := drainPull(stream.Body, progress); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("ollama: pull %q: %w", ref, ctxErr)
		}
		return fmt.Errorf("ollama: pull %q: %w", ref, err)
	}
	return nil
}

func drainPull(body io.Reader, progress func(workload.PullProgress) error) error {
	scanner := bufio.NewScanner(io.LimitReader(body, maxPullStream))
	scanner.Buffer(make([]byte, 0, 4<<10), maxPullLine)
	layers := map[string][2]int64{}
	for scanner.Scan() {
		raw := bytes.TrimSpace(scanner.Bytes())
		if len(raw) == 0 {
			continue
		}
		var line pullLine
		if err := json.Unmarshal(raw, &line); err != nil {
			return fmt.Errorf("decode progress: %w", err)
		}
		if line.Error != "" {
			return fmt.Errorf("pull failed: %s", strings.TrimSpace(line.Error))
		}
		if line.Digest != "" {
			layers[line.Digest] = [2]int64{max(line.Completed, 0), max(line.Total, 0)}
		}
		report := workload.PullProgress{Status: strings.TrimSpace(line.Status)}
		for _, l := range layers {
			report.Completed += min(l[0], l[1])
			report.Total += l[1]
		}
		if progress != nil {
			if err := progress(report); err != nil {
				return fmt.Errorf("pull stopped: %w", err)
			}
		}
		if line.Status == "success" {
			return nil
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read progress: %w", err)
	}
	return errors.New("progress stream ended before success")
}

func (c *Client) call(ctx context.Context, method, path string, body, into any) error {
	resp, err := c.http.Do(ctx, httpclient.Request{Method: method, Path: path, Body: body})
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
