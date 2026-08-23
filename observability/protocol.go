package observability

import "strings"

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

// ParseOTLPProtocol maps a config string ("grpc", "http"/"http/protobuf", or "") to a
// protocol. Unknown values fall back to OTLPHTTP so a typo cannot silently disable export.
func ParseOTLPProtocol(value string) OTLPProtocol {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "grpc", "otlp/grpc":
		return OTLPGRPC
	default:
		return OTLPHTTP
	}
}
