package wire

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	codepb "google.golang.org/genproto/googleapis/rpc/code"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	apperrors "github.com/kbukum/gokit/errors"
	errorrpc "github.com/kbukum/gokit/errors/rpc"
)

// wireFixtures holds the published golden wire fixtures. They are committed JSON
// so consumers in other repositories (plainworks over Connect and HTTP, the rskit
// mirror) can decode exactly the bytes a gokit service emits, and so gokit's own
// tests can prove it encodes and decodes them without loss.
//
//go:embed testdata/wire/*.json
var wireFixtures embed.FS

// WireCase is one canonical failure, the single source of truth a Fixture is
// rendered from. Transport tests round-trip Err through their own encoder and
// decoder and assert the vocabulary survives.
type WireCase struct {
	Name string
	Err  *apperrors.AppError
}

// Vocabulary is the transport-agnostic typed failure: exactly the fields section
// 1 of contracts.md defines, named as they appear on the wire.
type Vocabulary struct {
	Code         string                `json:"code"`
	Message      string                `json:"message"`
	Reason       string                `json:"reason,omitempty"`
	Violations   []apperrors.Violation `json:"violations,omitempty"`
	Retryable    bool                  `json:"retryable"`
	RetryAfterMs int64                 `json:"retryAfterMs,omitempty"`
	TraceID      string                `json:"traceId,omitempty"`
}

// Fixture is the published shape: the canonical vocabulary, the HTTP status and
// numeric RPC code the one code table assigns, and the exact problem+json body.
type Fixture struct {
	Name        string          `json:"name"`
	HTTPStatus  int             `json:"httpStatus"`
	RPCCode     uint32          `json:"rpcCode"`
	Vocabulary  Vocabulary      `json:"vocabulary"`
	ProblemJSON json.RawMessage `json:"problemJson"`
	// GRPCStatus is a base64-encoded binary google.rpc.Status in JSON fixtures.
	GRPCStatus  []byte          `json:"grpcStatus"`
	ConnectJSON json.RawMessage `json:"connectJson"`
}

// WireCases returns the canonical failures, built with the ordinary constructors
// so the fixtures track how a real service produces errors. They deliberately
// cover code and reason together, field violations, retry with and without a
// delay, several codes that share one RPC status, a retryable error that shares
// the Internal RPC code, and a safe internal message.
func WireCases() []WireCase {
	return []WireCase{
		{"not_found_with_reason_and_trace", apperrors.NotFound("user", "123").
			WithReason("USER_GONE").WithTraceID("trace-abc")},
		{"validation_with_violations", apperrors.Validation("request failed validation").
			WithViolations(
				apperrors.Violation{Field: "email", Reason: "REQUIRED", Message: "is required"},
				apperrors.Violation{Field: "age", Reason: "OUT_OF_RANGE", Message: "must be >= 18"},
			).WithReason("FORM_INVALID")},
		{"rate_limited_retry_with_delay", apperrors.RateLimited().
			WithRetryAfter(1500 * time.Millisecond)},
		{"service_unavailable_retry_no_delay", apperrors.ServiceUnavailable("db")},
		{"missing_field_exact_code", apperrors.MissingField("name")},
		{"invalid_format_exact_code", apperrors.InvalidFormat("date", "RFC3339")},
		{"external_service_retryable_shared_code", apperrors.ExternalServiceError("stripe", nil).WithRetryable(true)},
		{"service_unavailable_retry_disabled", apperrors.ServiceUnavailable("db").WithRetryable(false)},
		{"internal_safe_message", apperrors.Internal(nil)},
		{"connection_failed", apperrors.ConnectionFailed("db")},
		{"timeout", apperrors.Timeout("request")},
		{"already_exists", apperrors.AlreadyExists("user")},
		{"conflict", apperrors.Conflict("state changed")},
		{"unauthorized", apperrors.Unauthorized("")},
		{"token_expired", apperrors.TokenExpired()},
		{"invalid_token", apperrors.InvalidToken()},
		{"forbidden", apperrors.Forbidden("")},
		{"database_error", apperrors.DatabaseError(nil)},
		{"canceled", apperrors.Canceled("request")},
		{"external_service_permanent", apperrors.ExternalServiceError("provider", nil)},
	}
}

// RenderFixture renders the published Fixture for a canonical case. The generator
// and the drift test share this one renderer, so the committed files are exactly
// what gokit produces.
func RenderFixture(c WireCase) (Fixture, error) {
	pd := c.Err.ToProblemDetail()
	body, err := json.Marshal(pd)
	if err != nil {
		return Fixture{}, fmt.Errorf("marshal problem+json for %q: %w", c.Name, err)
	}
	status, err := errorrpc.Encode(c.Err, "gokit.test.v1.TestService")
	if err != nil {
		return Fixture{}, err
	}
	statusBytes, err := proto.MarshalOptions{Deterministic: true}.Marshal(status)
	if err != nil {
		return Fixture{}, err
	}
	type connectDetail struct {
		Type  string          `json:"type"`
		Value string          `json:"value"`
		Debug json.RawMessage `json:"debug"`
	}
	bodyRPC := struct {
		Code    string          `json:"code"`
		Message string          `json:"message"`
		Details []connectDetail `json:"details"`
	}{Code: strings.ToLower(codepb.Code(status.Code).String()), Message: status.Message}
	if status.Code == int32(apperrors.RPCCodeCanceled) {
		bodyRPC.Code = "canceled"
	}
	for _, detail := range status.Details {
		message, detailErr := detail.UnmarshalNew()
		if detailErr != nil {
			return Fixture{}, detailErr
		}
		debug, detailErr := protojson.Marshal(message)
		if detailErr != nil {
			return Fixture{}, detailErr
		}
		bodyRPC.Details = append(bodyRPC.Details, connectDetail{
			Type:  string(detail.MessageName()),
			Value: base64.RawStdEncoding.EncodeToString(detail.Value),
			Debug: debug,
		})
	}
	connectJSON, err := json.Marshal(bodyRPC)
	if err != nil {
		return Fixture{}, err
	}
	return Fixture{
		Name:       c.Name,
		HTTPStatus: apperrors.HTTPStatusFor(c.Err.Code),
		RPCCode:    uint32(apperrors.RPCCodeFor(c.Err.Code)),
		Vocabulary: Vocabulary{
			Code:         string(c.Err.Code),
			Message:      c.Err.Message,
			Reason:       c.Err.Reason,
			Violations:   c.Err.Violations,
			Retryable:    c.Err.Retryable,
			RetryAfterMs: c.Err.RetryAfter.Milliseconds(),
			TraceID:      c.Err.TraceID,
		},
		ProblemJSON: body,
		GRPCStatus:  statusBytes,
		ConnectJSON: connectJSON,
	}, nil
}

// LoadFixtures reads the committed golden fixtures keyed by name.
func LoadFixtures() (map[string]Fixture, error) {
	entries, err := wireFixtures.ReadDir("testdata/wire")
	if err != nil {
		return nil, err
	}
	out := make(map[string]Fixture, len(entries))
	for _, e := range entries {
		data, readErr := wireFixtures.ReadFile("testdata/wire/" + e.Name())
		if readErr != nil {
			return nil, readErr
		}
		var f Fixture
		if unmarshalErr := json.Unmarshal(data, &f); unmarshalErr != nil {
			return nil, fmt.Errorf("decode fixture %q: %w", e.Name(), unmarshalErr)
		}
		out[f.Name] = f
	}
	return out, nil
}

// FixtureNames returns the canonical case names in a stable order.
func FixtureNames() []string {
	cases := WireCases()
	names := make([]string, 0, len(cases))
	for _, c := range cases {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	return names
}
