package triton

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/google/uuid"

	"github.com/kbukum/gokit/ai/semconv"
	"github.com/kbukum/gokit/authz"
	"github.com/kbukum/gokit/httpclient"
	"github.com/kbukum/gokit/inference"
	"github.com/kbukum/gokit/observability"
)

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

	resp, err := p.do(ctx, httpclient.Request{Method: http.MethodPost, Path: path, Body: body})
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
	p.lifecycle.Touch()
	return decoded, nil
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
