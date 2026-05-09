package triton

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/authz"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/inference"
	"github.com/kbukum/gokit/observability"
	"github.com/kbukum/gokit/resilience"
)

const servingProtocol = "kserve-v2-http"

// Config configures the Triton KServe v2 HTTP adapter.
type Config struct {
	Name           string            `json:"name,omitempty"`
	Description    string            `json:"description,omitempty"`
	BaseURL        string            `json:"base_url"`
	TimeoutSeconds int               `json:"timeout_seconds,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	BearerToken    string            `json:"bearer_token,omitempty"`
}

// Option injects lower-layer dependencies into Provider.
type Option func(*providerOptions)

type providerOptions struct {
	httpClient *httpclient.Adapter
	retry      *resilience.RetryConfig
	decider    authz.Decider
	subject    authz.Subject
}

// WithHTTPClient injects a configured httpclient.Adapter.
func WithHTTPClient(client *httpclient.Adapter) Option {
	return func(opts *providerOptions) { opts.httpClient = client }
}

// WithRetry injects a resilience retry policy without inventing local retry loops.
func WithRetry(retry resilience.RetryConfig) Option {
	return func(opts *providerOptions) { opts.retry = &retry }
}

// WithDecider injects an optional authz decider. Nil means open.
func WithDecider(decider authz.Decider, subject authz.Subject) Option {
	return func(opts *providerOptions) {
		opts.decider = decider
		opts.subject = subject
	}
}

// Provider implements inference.Inference against Triton / KServe v2 HTTP.
type Provider struct {
	cfg        Config
	client     *httpclient.Adapter
	descriptor inference.Descriptor
	decider    authz.Decider
	subject    authz.Subject
}

// NewProvider creates a Triton KServe v2 HTTP provider.
func NewProvider(cfg Config, options ...Option) (*Provider, error) {
	if strings.TrimSpace(cfg.BaseURL) == "" {
		return nil, errors.New("triton: base_url is required")
	}
	parsed, err := url.Parse(cfg.BaseURL)
	if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
		return nil, fmt.Errorf("triton: invalid base_url %q", cfg.BaseURL)
	}
	if cfg.Name == "" {
		cfg.Name = Kind
	}
	if cfg.Description == "" {
		cfg.Description = "Triton / KServe v2 HTTP model-serving adapter"
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 30
	}

	opts := providerOptions{}
	for _, opt := range options {
		opt(&opts)
	}

	client := opts.httpClient
	if client == nil {
		httpCfg := httpclient.Config{
			Name:    cfg.Name,
			BaseURL: strings.TrimRight(cfg.BaseURL, "/"),
			Timeout: time.Duration(cfg.TimeoutSeconds) * time.Second,
			Headers: cfg.Headers,
			Retry:   opts.retry,
		}
		if strings.TrimSpace(cfg.BearerToken) != "" {
			httpCfg.Auth = &httpclient.AuthConfig{Type: httpclient.AuthBearer, Token: cfg.BearerToken}
		}
		client, err = httpclient.New(httpCfg)
		if err != nil {
			return nil, fmt.Errorf("triton: create http client: %w", err)
		}
	}

	return &Provider{
		cfg:        cfg,
		client:     client,
		descriptor: descriptor(cfg, parsed),
		decider:    opts.decider,
		subject:    opts.subject,
	}, nil
}

// Descriptor documents the adapter and its network egress envelope.
func (p *Provider) Descriptor() inference.Descriptor { return p.descriptor }

// Health probes /v2/health/ready.
func (p *Provider) Health(ctx context.Context) error {
	ctx, span := startSpan(ctx, "health")
	defer span.End()

	_, err := p.do(ctx, http.MethodGet, "/v2/health/ready", nil)
	if err != nil {
		span.RecordError(err)
		span.SetError(err.Error())
	}
	return err
}

// Predict calls /v2/models/{name}/infer (or /versions/{version}/infer).
func (p *Provider) Predict(ctx context.Context, req inference.PredictRequest) (inference.PredictResponse, error) {
	if strings.TrimSpace(req.ModelName) == "" {
		return inference.PredictResponse{}, errors.New("triton: model_name is required")
	}
	if strings.TrimSpace(req.RequestID) == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return inference.PredictResponse{}, fmt.Errorf("triton: generate request id: %w", err)
		}
		req.RequestID = id.String()
	}
	ctx, span := startSpan(ctx, operation(req), modelAttributes(req)...)
	defer span.End()
	span.SetAttributes(observability.StringAttribute(semconv.GenAIRequestID, req.RequestID))

	if err := p.authorize(ctx, req); err != nil {
		span.RecordError(err)
		span.SetError(err.Error())
		return inference.PredictResponse{}, err
	}

	body, err := encodeRequest(req)
	if err != nil {
		span.RecordError(err)
		span.SetError(err.Error())
		return inference.PredictResponse{}, err
	}

	path := "/v2/models/" + url.PathEscape(req.ModelName)
	if req.ModelVersion != "" {
		path += "/versions/" + url.PathEscape(req.ModelVersion)
	}
	path += "/infer"

	resp, err := p.do(ctx, http.MethodPost, path, body)
	if err != nil {
		span.RecordError(err)
		span.SetError(err.Error())
		return inference.PredictResponse{}, err
	}
	decoded, err := decodeResponse(resp)
	if err != nil {
		span.RecordError(err)
		span.SetError(err.Error())
		return inference.PredictResponse{}, err
	}
	span.SetAttributes(usageAttributes(decoded.Usage)...)
	if decoded.Model.Name != "" {
		span.SetAttributes(observability.StringAttribute(semconv.GenAIResponseModel, decoded.Model.Name))
	}
	if finishReason := decoded.Metadata["finish_reason"]; finishReason != "" {
		span.SetAttributes(observability.StringAttribute(semconv.GenAIResponseFinishReason, finishReason))
	}
	return decoded, nil
}

func startSpan(ctx context.Context, operationName string, attrs ...observability.SpanAttribute) (context.Context, *observability.Span) {
	baseAttrs := make([]observability.SpanAttribute, 0, 2+len(attrs))
	baseAttrs = append(baseAttrs,
		observability.StringAttribute(semconv.GenAISystem, Kind),
		observability.StringAttribute(semconv.GenAIOperationName, operationName),
	)
	baseAttrs = append(baseAttrs, attrs...)
	return observability.StartNamedSpan(ctx, "github.com/kbukum/gokit/inference/triton", semconv.OpInferenceRequest,
		observability.WithSpanKind(observability.SpanKindClient),
		observability.WithSpanAttributes(baseAttrs...),
	)
}

func (p *Provider) do(ctx context.Context, method, path string, body any) (*httpclient.Response, error) {
	resp, err := p.client.Do(ctx, httpclient.Request{Method: method, Path: path, Body: body})
	if err != nil {
		if resp != nil && len(resp.Body) > 0 {
			return nil, errors.Join(err, fmt.Errorf("triton: response body: %s", strings.TrimSpace(string(resp.Body))))
		}
		return nil, err
	}
	return resp, nil
}

func (p *Provider) authorize(ctx context.Context, req inference.PredictRequest) error {
	if p.decider == nil {
		return nil
	}
	decision, err := p.decider.Decide(ctx, authz.Request{
		Subject: p.subject,
		Resource: authz.Resource{Type: "inference.model", ID: req.ModelName, Attributes: authz.Attributes{
			"adapter": Kind,
			"version": req.ModelVersion,
		}},
		Action: "inference:predict",
	})
	if err != nil {
		return fmt.Errorf("triton: authz decision: %w", err)
	}
	if !decision.Allowed {
		return fmt.Errorf("triton: authz denied: %s", decision.Reason)
	}
	return nil
}

func descriptor(cfg Config, u *url.URL) inference.Descriptor {
	_ = u // hostname/port retained for future capability hints; not part of the lean Descriptor.
	return inference.Descriptor{
		Name:            cfg.Name,
		Description:     cfg.Description,
		ServingProtocol: servingProtocol,
		Capabilities:    inference.CapabilityHints{SupportsStreaming: true, SupportsBatching: true},
		Available:       true,
	}
}

type kserveRequest struct {
	ID         string         `json:"id,omitempty"`
	Inputs     []kserveTensor `json:"inputs"`
	Parameters map[string]any `json:"parameters,omitempty"`
}

type kserveResponse struct {
	ModelName    string         `json:"model_name,omitempty"`
	ModelVersion string         `json:"model_version,omitempty"`
	Outputs      []kserveTensor `json:"outputs"`
	Parameters   map[string]any `json:"parameters,omitempty"`
}

type kserveTensor struct {
	Name     string          `json:"name"`
	Shape    []int64         `json:"shape"`
	Datatype string          `json:"datatype"`
	Data     json.RawMessage `json:"data,omitempty"`
}

func encodeRequest(req inference.PredictRequest) (kserveRequest, error) {
	inputs := make([]kserveTensor, 0, len(req.Inputs))
	var joined error
	for name, value := range req.Inputs {
		input, err := encodeInput(name, value)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		inputs = append(inputs, input)
	}
	if joined != nil {
		return kserveRequest{}, joined
	}
	parameters := make(map[string]any, len(req.Parameters)+len(req.Options))
	for key, value := range req.Parameters {
		parameters[key] = value
	}
	for key, value := range req.Options {
		parameters[key] = value
	}
	if len(parameters) == 0 {
		parameters = nil
	}
	return kserveRequest{ID: req.RequestID, Inputs: inputs, Parameters: parameters}, nil
}

func encodeInput(name string, value inference.Value) (kserveTensor, error) {
	switch value.Kind {
	case inference.KindText:
		return tensor(name, "BYTES", []int64{1}, []string{value.Text})
	case inference.KindBytes:
		return tensor(name, "BYTES", []int64{1}, []string{base64.StdEncoding.EncodeToString(value.Bytes)})
	case inference.KindTensor:
		if value.Tensor == nil {
			return kserveTensor{}, fmt.Errorf("triton: input %q tensor is nil", name)
		}
		data, err := normalizeTensorData(value.Tensor.DType, value.Tensor.Data)
		if err != nil {
			return kserveTensor{}, fmt.Errorf("triton: input %q: %w", name, err)
		}
		return tensor(name, value.Tensor.DType, value.Tensor.Shape, data)
	case inference.KindJSON:
		return kserveTensor{Name: name, Shape: []int64{1}, Datatype: "JSON", Data: append(json.RawMessage(nil), value.JSON...)}, nil
	default:
		return kserveTensor{}, fmt.Errorf("triton: input %q has unsupported kind %q", name, value.Kind)
	}
}

func tensor(name, dtype string, shape []int64, data any) (kserveTensor, error) {
	encoded, err := json.Marshal(data)
	if err != nil {
		return kserveTensor{}, fmt.Errorf("marshal tensor %q data: %w", name, err)
	}
	return kserveTensor{Name: name, Shape: append([]int64(nil), shape...), Datatype: dtype, Data: encoded}, nil
}

func normalizeTensorData(dtype string, data any) (any, error) {
	switch strings.ToUpper(dtype) {
	case "FP32":
		switch v := data.(type) {
		case []float32, []float64:
			return v, nil
		default:
			return nil, fmt.Errorf("FP32 data must be []float32 or []float64")
		}
	case "INT64":
		switch v := data.(type) {
		case []int64:
			return v, nil
		case []int:
			out := make([]int64, len(v))
			for i, n := range v {
				out[i] = int64(n)
			}
			return out, nil
		default:
			return nil, fmt.Errorf("INT64 data must be []int64 or []int")
		}
	case "BYTES":
		switch v := data.(type) {
		case []string, [][]byte:
			return v, nil
		case []byte:
			return []string{base64.StdEncoding.EncodeToString(v)}, nil
		default:
			return nil, fmt.Errorf("BYTES data must be []string, [][]byte, or []byte")
		}
	default:
		return data, nil
	}
}

func decodeResponse(resp *httpclient.Response) (inference.PredictResponse, error) {
	var payload kserveResponse
	if err := json.Unmarshal(resp.Body, &payload); err != nil {
		return inference.PredictResponse{}, fmt.Errorf("triton: decode KServe v2 response: %w", err)
	}
	outputs := make(map[string]inference.Value, len(payload.Outputs))
	var joined error
	for _, output := range payload.Outputs {
		value, err := decodeOutput(output)
		if err != nil {
			joined = errors.Join(joined, err)
			continue
		}
		outputs[output.Name] = value
	}
	if joined != nil {
		return inference.PredictResponse{}, joined
	}
	modelName := firstNonEmpty(resp.Headers["model_name"], resp.Headers["model-name"], payload.ModelName)
	modelVersion := firstNonEmpty(resp.Headers["model_version"], resp.Headers["model-version"], payload.ModelVersion)
	metadata := map[string]string{}
	if modelName != "" {
		metadata["model_name"] = modelName
	}
	if modelVersion != "" {
		metadata["model_version"] = modelVersion
	}
	return inference.PredictResponse{
		Outputs:  outputs,
		Model:    ai.Model{Name: modelName, Version: modelVersion, Provider: ai.ProviderTriton},
		Status:   inference.StatusSuccess,
		Metadata: metadata,
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func decodeOutput(output kserveTensor) (inference.Value, error) {
	switch strings.ToUpper(output.Datatype) {
	case "FP32":
		var data []float32
		if err := json.Unmarshal(output.Data, &data); err != nil {
			return inference.Value{}, fmt.Errorf("triton: output %q FP32 decode: %w", output.Name, err)
		}
		return inference.TensorValue(inference.Tensor{DType: "FP32", Shape: output.Shape, Data: data}), nil
	case "INT64":
		var data []int64
		if err := json.Unmarshal(output.Data, &data); err != nil {
			return inference.Value{}, fmt.Errorf("triton: output %q INT64 decode: %w", output.Name, err)
		}
		return inference.TensorValue(inference.Tensor{DType: "INT64", Shape: output.Shape, Data: data}), nil
	case "BYTES":
		var data []string
		if err := json.Unmarshal(output.Data, &data); err != nil {
			return inference.Value{}, fmt.Errorf("triton: output %q BYTES decode: %w", output.Name, err)
		}
		return inference.TensorValue(inference.Tensor{DType: "BYTES", Shape: output.Shape, Data: data}), nil
	case "JSON":
		return inference.JSONValue(output.Data), nil
	default:
		var data any
		if len(output.Data) > 0 {
			if err := json.Unmarshal(output.Data, &data); err != nil {
				return inference.Value{}, fmt.Errorf("triton: output %q decode: %w", output.Name, err)
			}
		}
		return inference.TensorValue(inference.Tensor{DType: output.Datatype, Shape: output.Shape, Data: data}), nil
	}
}

func operation(inference.PredictRequest) string {
	return semconv.OpInferenceRequest
}

func modelAttributes(req inference.PredictRequest) []observability.SpanAttribute {
	attrs := []observability.SpanAttribute{
		observability.StringAttribute(semconv.GenAIRequestModel, req.ModelName),
	}
	if req.ModelVersion != "" {
		attrs = append(attrs, observability.StringAttribute(semconv.GenAIRequestModelVersion, req.ModelVersion))
	}
	return attrs
}

func usageAttributes(usage inference.Usage) []observability.SpanAttribute {
	return []observability.SpanAttribute{
		observability.IntAttribute(semconv.GenAIUsageInputTokens, usage.InputTokens),
		observability.IntAttribute(semconv.GenAIUsageOutputTokens, usage.OutputTokens),
		observability.IntAttribute(semconv.GenAIUsageCachedTokens, usage.CachedTokens),
		observability.IntAttribute(semconv.GenAIUsageReasoningTokens, usage.ReasoningTokens),
	}
}
