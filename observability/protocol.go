package observability

import (
	"strings"

	apperr "github.com/kbukum/gokit/errors"
)

// OTLPProtocol selects the OTLP exporter wire protocol for traces and metrics.
// It lets callers pick the transport their collector exposes instead of being
// locked to HTTP, matching the protocol selection in the sibling kits.
type OTLPProtocol int

const (
	// OTLPHTTP exports over OTLP/HTTP with protobuf payloads (default; collector port 4318).
	OTLPHTTP OTLPProtocol = iota
	// OTLPGRPC exports over OTLP/gRPC (collector port 4317).
	OTLPGRPC
)

// String returns the canonical lowercase name of the protocol.
func (p OTLPProtocol) String() string {
	if p == OTLPGRPC {
		return "grpc"
	}
	return "http"
}

// Validate rejects a protocol value outside the known set, so a directly-assigned enum
// cannot select an unintended exporter at export time.
func (p OTLPProtocol) Validate() error {
	switch p {
	case OTLPHTTP, OTLPGRPC:
		return nil
	default:
		return apperr.InvalidInput("protocol", "must be one of http, grpc")
	}
}

// ParseOTLPProtocol maps a config string ("grpc"/"otlp/grpc", "http"/"http/protobuf", or
// "") to a protocol. An empty value defaults to OTLPHTTP; any other unrecognized value is
// rejected so a typo such as "grcp" fails configuration instead of silently exporting over
// the wrong transport.
func ParseOTLPProtocol(value string) (OTLPProtocol, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "http", "http/protobuf", "otlp/http":
		return OTLPHTTP, nil
	case "grpc", "otlp/grpc":
		return OTLPGRPC, nil
	default:
		return OTLPHTTP, apperr.InvalidInput("protocol", "unsupported OTLP protocol "+value+" (use http or grpc)")
	}
}
