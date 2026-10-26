package dmr

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/workload"
)

// maxErrBody bounds how much of a pull stream's plain-text failure tail is read.
const maxErrBody = 4 << 10

// maxModelBody bounds how much of a model list/detail response is read. BaseURL
// is caller-configurable (a trust boundary), so a misbehaving or compromised
// endpoint cannot force unbounded allocation.
const maxModelBody = 8 << 20

// maxPullStream bounds how much of the create progress stream is consumed, so a
// misbehaving daemon cannot stream unboundedly.
const maxPullStream = 64 << 20

// Error is a typed Docker Model Runner API error. Callers can inspect Status via errors.As to distinguish not-found,
// unauthorized, or server failures. It carries the status only: a response body can echo credentials or prompts.
type Error struct {
	Status int // HTTP status code
}

func (e *Error) Error() string {
	return fmt.Sprintf("dmr: unexpected status %d", e.Status)
}

// client is a thin typed wrapper over the Docker Model Runner REST API, built on
// the canonical httpclient adapter so every call is timeout-bounded and can run
// through a resilience policy.
type client struct {
	baseURL     string // normalized, no trailing slash
	http        *httpclient.Adapter
	pullTimeout time.Duration
}

func newClient(cfg *Config) (*client, error) {
	base := cfg.normalizedBaseURL()
	hc, err := httpclient.New(httpclient.Config{
		Name:    "dmr",
		BaseURL: base,
		Timeout: cfg.Timeout,
		Stream: httpclient.StreamConfig{
			TotalTimeout:         cfg.PullTimeout,
			FirstProgressTimeout: cfg.PullTimeout,
			IdleTimeout:          cfg.PullTimeout,
		},
		ResiliencePolicy:     cfg.ResiliencePolicy,
		EndpointPolicy:       cfg.EndpointPolicy,
		MaxResponseBodyBytes: maxModelBody,
		DefaultHeaders:       map[string]string{"Accept": "application/json"},
	})
	if err != nil {
		return nil, fmt.Errorf("dmr: build http client: %w", err)
	}
	return &client{baseURL: base, http: hc, pullTimeout: cfg.PullTimeout}, nil
}

// enginesBaseURL is the OpenAI-compatible inference base URL.
func (c *client) enginesBaseURL() string {
	return c.baseURL + "/engines/v1"
}

// endpoint returns the inference endpoint for a model.
func (c *client) endpoint(model string) workload.Endpoint {
	return workload.Endpoint{BaseURL: c.enginesBaseURL(), Model: model, API: workload.APIOpenAI}
}

// modelPath builds the /models/{ref} path with ref percent-escaped as path
// segments. It preserves "/" so multi-segment refs (for example "ai/smollm2")
// keep matching DMR's {name...} wildcard, while corrupting characters such as #,
// ?, %, or spaces are escaped.
//
// The ref is an untrusted input (it may originate from a caller or a registry
// response), so each segment is validated: empty, "." and ".." segments are
// rejected before the path is built, preventing a ref such as "../engines/ps"
// from escaping the /models/ route after HTTP path cleaning or redirects.
func modelPath(ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("dmr: model is required")
	}
	for _, seg := range strings.Split(ref, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return "", fmt.Errorf("dmr: invalid model reference %q", ref)
		}
	}
	return "/models/" + (&url.URL{Path: ref}).EscapedPath(), nil
}

// modelObject is the subset of DMR's native model record this backend consumes.
// It matches the wire shape of docker/model-runner's `Model` type: the tag lives
// in Tags (ID is a content digest) and on-disk size is a byte-count string nested
// under Config.
type modelObject struct {
	ID      string   `json:"id"`
	Tags    []string `json:"tags"`
	Created int64    `json:"created"`
	Config  struct {
		Format       string `json:"format"`
		Size         string `json:"size"`
		Architecture string `json:"architecture"`
		Parameters   string `json:"parameters"`
		Quantization string `json:"quantization"`
		ContextSize  *int32 `json:"context_size"`
	} `json:"config"`
}

// describe maps the record to a model descriptor. DMR reports no task or capabilities, so those stay unknown.
func (m *modelObject) describe() ai.Model {
	d := ai.Model{
		Name:     m.ref(),
		Provider: ai.ProviderDMR,
		Digest:   m.ID,
		Variant: ai.Variant{
			Parameters:   m.Config.Parameters,
			Quantization: m.Config.Quantization,
			Format:       m.Config.Format,
			SizeBytes:    m.sizeBytes(),
		},
	}
	if m.Config.Architecture != "" {
		d.Family = ai.Stated(m.Config.Architecture, ai.SourceRuntime)
	}
	if m.Config.ContextSize != nil && *m.Config.ContextSize > 0 {
		d.Capabilities.ContextLength = ai.Stated(int(*m.Config.ContextSize), ai.SourceRuntime)
	}
	return d
}

// ref returns the human-readable model reference (first tag), falling back to the
// digest ID when no tag is present.
func (m *modelObject) ref() string {
	if len(m.Tags) > 0 {
		return m.Tags[0]
	}
	return m.ID
}

// sizeBytes parses the on-disk size, returning 0 when absent or unparseable.
func (m *modelObject) sizeBytes() int64 {
	return parseSize(m.Config.Size)
}

// sizeUnits are the units DMR's human-readable sizes use, such as "256.35MiB" or "1.2 GB".
var sizeUnits = map[string]float64{
	"": 1, "B": 1,
	"kB": 1e3, "KB": 1e3, "MB": 1e6, "GB": 1e9, "TB": 1e12,
	"KiB": 1 << 10, "MiB": 1 << 20, "GiB": 1 << 30, "TiB": 1 << 40,
}

// parseSize reads a byte count with an optional unit. It returns 0 for anything it cannot read, so a size is
// unknown rather than wrong.
func parseSize(s string) int64 {
	s = strings.TrimSpace(s)
	i := strings.IndexFunc(s, func(r rune) bool { return (r < '0' || r > '9') && r != '.' })
	if i < 0 {
		i = len(s)
	}
	n, err := strconv.ParseFloat(s[:i], 64)
	unit, ok := sizeUnits[strings.TrimSpace(s[i:])]
	if err != nil || !ok || n <= 0 || n*unit > math.MaxInt64 {
		return 0
	}
	return int64(n * unit)
}

// pullProgress is one event in DMR's model-create progress stream. DMR emits
// oci.ProgressMessage events: a failure is a {"type":"error","message":...}
// event (Error is a legacy fallback field). Total is the model's download size;
// Layer reports the bytes of one layer so far, and older daemons state only
// Pulled for the whole model.
type pullProgress struct {
	Type    string    `json:"type"`
	Message string    `json:"message"`
	Error   string    `json:"error"`
	Total   int64     `json:"total"`
	Pulled  int64     `json:"pulled"`
	Layer   pullLayer `json:"layer"`
}

type pullLayer struct {
	ID      string `json:"id"`
	Current int64  `json:"current"`
}

// pullTally sums a pull's progress across events.
type pullTally struct {
	layers map[string]int64
	pulled int64
	total  int64
}

// add folds one event in and returns the overall progress. Completed never
// exceeds a stated total, and a successful pull completes it.
func (t *pullTally) add(ev *pullProgress) workload.PullProgress {
	t.total = max(t.total, ev.Total, 0)
	if ev.Layer.ID != "" {
		if t.layers == nil {
			t.layers = map[string]int64{}
		}
		t.layers[ev.Layer.ID] = max(ev.Layer.Current, 0)
		t.pulled = 0
		for _, n := range t.layers {
			t.pulled += n
		}
	} else if ev.Pulled > 0 {
		t.pulled = ev.Pulled
	}
	if ev.Type == "success" && t.total > 0 {
		t.pulled = t.total
	}
	if t.total > 0 {
		t.pulled = min(t.pulled, t.total)
	}
	return workload.PullProgress{Status: strings.TrimSpace(ev.Message), Completed: t.pulled, Total: t.total}
}

// failure reports the failure message if this event marks a failed pull.
func (p *pullProgress) failure() (string, bool) {
	if p.Type == "error" {
		msg := p.Message
		if msg == "" {
			msg = p.Error
		}
		return strings.TrimSpace(msg), true
	}
	if p.Error != "" {
		return strings.TrimSpace(p.Error), true
	}
	return "", false
}

// backendStatus is the subset of DMR's /engines/ps record this backend consumes.
type backendStatus struct {
	ModelName string `json:"model_name"`
	Loading   bool   `json:"loading"`
}

// pull creates (pulls) a model via POST /models/create {"from": ref}. DMR streams
// chunked progress and only completes the response when the pull finishes, so the
// body must be drained to completion; failures after the initial 200 are reported
// in the stream rather than the status code.
//
// Pulls use the configured pull budget rather than the HTTP adapter's shorter inference defaults. An earlier caller deadline is preserved.
func (c *client) pull(ctx context.Context, ref string, progress func(workload.PullProgress) error) error {
	if _, ok := ctx.Deadline(); !ok && c.pullTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.pullTimeout)
		defer cancel()
	}
	stream, err := c.http.DoStream(ctx, httpclient.Request{
		Method: http.MethodPost,
		Path:   "/models/create",
		Body:   map[string]string{"from": ref},
	})
	if err != nil {
		return fmt.Errorf("dmr: pull %q: %w", ref, err)
	}
	defer func() { _ = stream.Close() }()
	return drainPullStream(stream.Body, maxPullStream, progress)
}

// configure applies per-model runtime settings via POST /engines/_configure. DMR
// answers 202 Accepted and preloads the model in the background.
func (c *client) configure(ctx context.Context, ref string, contextSize int) error {
	req := map[string]any{"model": ref}
	if contextSize > 0 {
		req["context-size"] = contextSize
	}
	_, err := c.do(ctx, http.MethodPost, "/engines/_configure", req)
	return err
}

// unload releases a loaded model via POST /engines/unload {"models":[ref]}. DMR
// reports the number of unloaded runners; unloading an unknown or already-idle
// model is a no-op (zero runners), so this is idempotent.
func (c *client) unload(ctx context.Context, ref string) error {
	_, err := c.do(ctx, http.MethodPost, "/engines/unload", map[string]any{"models": []string{ref}})
	return err
}

// health verifies the runtime is reachable via GET /models.
func (c *client) health(ctx context.Context) error {
	_, err := c.do(ctx, http.MethodGet, "/models", nil)
	return err
}

// list returns the local models via GET /models.
func (c *client) list(ctx context.Context) ([]modelObject, error) {
	resp, err := c.do(ctx, http.MethodGet, "/models", nil)
	if err != nil {
		return nil, err
	}
	return decodeModelList(bytes.NewReader(resp.Body), maxModelBody)
}

// loaded reports whether the named model has a running (non-loading) runner,
// queried via GET /engines/ps.
func (c *client) loaded(ctx context.Context, model string) (bool, error) {
	resp, err := c.do(ctx, http.MethodGet, "/engines/ps", nil)
	if err != nil {
		return false, err
	}
	var running []backendStatus
	if err := json.Unmarshal(resp.Body, &running); err != nil {
		return false, fmt.Errorf("dmr: decode engines/ps: %w", err)
	}
	for i := range running {
		if running[i].ModelName == model && !running[i].Loading {
			return true, nil
		}
	}
	return false, nil
}

// get returns a single model's details via GET /models/{ref}.
func (c *client) get(ctx context.Context, ref string) (*modelObject, error) {
	path, err := modelPath(ref)
	if err != nil {
		return nil, err
	}
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	var m modelObject
	if err := json.Unmarshal(resp.Body, &m); err != nil {
		return nil, fmt.Errorf("dmr: decode model: %w", err)
	}
	return &m, nil
}

// remove deletes a local model via DELETE /models/{ref}.
func (c *client) remove(ctx context.Context, ref string) error {
	path, err := modelPath(ref)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, http.MethodDelete, path, nil)
	return err
}

// do issues a non-streaming request through the httpclient adapter. A non-2xx
// response is surfaced as a typed [Error] carrying the status only; a transport failure preserves its cause for errors.Is inspection.
func (c *client) do(ctx context.Context, method, path string, body any) (*httpclient.Response, error) {
	resp, err := c.http.Do(ctx, httpclient.Request{Method: method, Path: path, Body: body})
	if err != nil {
		if resp != nil {
			return nil, &Error{Status: resp.StatusCode}
		}
		return nil, fmt.Errorf("dmr: %s %s: %w", method, path, err)
	}
	return resp, nil
}

// readLimited reads r fully but refuses bodies larger than limit, so a
// misbehaving endpoint cannot exhaust memory.
func readLimited(r io.Reader, limit int64, what string) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", what, err)
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("dmr: %s response exceeds %d bytes", what, limit)
	}
	return data, nil
}

// decodeModelList tolerates both a bare array and the common object wrappers
// ({"data":[...]} / {"models":[...]}) that model-management APIs use. A non-array
// success body — an object carrying neither wrapper, or a JSON null — is rejected
// so a malformed success-shaped response (for example {"error":"failed"} or a
// bare null) cannot masquerade as an empty model list.
func decodeModelList(r io.Reader, limit int64) ([]modelObject, error) {
	raw, err := readLimited(r, limit, "model list")
	if err != nil {
		return nil, err
	}
	var arr []modelObject
	if err := json.Unmarshal(raw, &arr); err == nil && arr != nil {
		return arr, nil
	}
	var wrapped struct {
		Data   []modelObject `json:"data"`
		Models []modelObject `json:"models"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return nil, fmt.Errorf("decode model list: %w", err)
	}
	if wrapped.Data != nil {
		return wrapped.Data, nil
	}
	if wrapped.Models != nil {
		return wrapped.Models, nil
	}
	return nil, fmt.Errorf("dmr: model list response has neither \"data\" nor \"models\" array")
}

// drainPullStream consumes DMR's create progress stream to completion, up to
// limit bytes. Reading to EOF blocks until the pull actually finishes and keeps
// the connection intact. DMR reports a mid-pull failure by appending a JSON error
// event or, once the 200 header is committed, a plain-text error via http.Error;
// a successful pull ends with a terminal {"type":"success"} event. Both failure
// forms surface here, a stream that ends before the success event is treated as a
// failed pull, and consuming the whole limit is an explicit failure so neither a
// truncated nor an oversized stream can masquerade as a completed pull. Each
// progress or success event goes to progress when it is set; an error from it
// stops the drain, and the caller closing the body ends the request.
func drainPullStream(body io.Reader, limit int64, progress func(workload.PullProgress) error) error {
	lr := &io.LimitedReader{R: body, N: limit + 1}
	dec := json.NewDecoder(lr)
	sawSuccess := false
	var tally pullTally
	for {
		var ev pullProgress
		err := dec.Decode(&ev)
		if err == nil {
			if msg, failed := ev.failure(); failed {
				return fmt.Errorf("dmr: pull failed: %s", msg)
			}
			if ev.Type == "success" {
				sawSuccess = true
			}
			report := tally.add(&ev)
			if progress != nil {
				if perr := progress(report); perr != nil {
					return fmt.Errorf("dmr: pull stopped: %w", perr)
				}
			}
			continue
		}
		if lr.N <= 0 {
			return fmt.Errorf("dmr: pull stream exceeds %d bytes", limit)
		}
		if errors.Is(err, io.EOF) {
			if !sawSuccess {
				return fmt.Errorf("dmr: pull stream ended before a success event")
			}
			return nil
		}
		// A JSON syntax error is DMR's plain-text terminal error (an http.Error
		// tail written after the 200 header). Any other error is a transport or
		// decode failure whose cause must be preserved for errors.Is inspection
		// (for example context cancellation).
		var syntaxErr *json.SyntaxError
		if !errors.As(err, &syntaxErr) {
			return fmt.Errorf("dmr: pull stream read: %w", err)
		}
		rest, _ := io.ReadAll(io.MultiReader(dec.Buffered(), io.LimitReader(lr, maxErrBody)))
		msg := strings.TrimSpace(string(rest))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("dmr: pull stream error: %s", msg)
	}
}
