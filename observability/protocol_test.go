package observability

import "testing"

func TestParseOTLPProtocol(t *testing.T) {
	t.Parallel()
	cases := map[string]OTLPProtocol{
		"grpc":          OTLPGRPC,
		"otlp/grpc":     OTLPGRPC,
		"GRPC":          OTLPGRPC,
		"http":          OTLPHTTP,
		"http/protobuf": OTLPHTTP,
		"":              OTLPHTTP,
		"nonsense":      OTLPHTTP,
	}
	for in, want := range cases {
		if got := ParseOTLPProtocol(in); got != want {
			t.Errorf("ParseOTLPProtocol(%q) = %v, want %v", in, got, want)
		}
	}
	if OTLPGRPC.String() != "grpc" || OTLPHTTP.String() != "http" {
		t.Fatalf("unexpected String(): %q %q", OTLPGRPC.String(), OTLPHTTP.String())
	}
}
