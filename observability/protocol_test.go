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
	}
	for in, want := range cases {
		got, err := ParseOTLPProtocol(in)
		if err != nil {
			t.Errorf("ParseOTLPProtocol(%q) unexpected error: %v", in, err)
		}
		if got != want {
			t.Errorf("ParseOTLPProtocol(%q) = %v, want %v", in, got, want)
		}
	}
	// A typo must fail closed rather than silently selecting HTTP.
	if _, err := ParseOTLPProtocol("grcp"); err == nil {
		t.Fatal("ParseOTLPProtocol(grcp) = nil error, want rejection")
	}
	if OTLPGRPC.String() != "grpc" || OTLPHTTP.String() != "http" {
		t.Fatalf("unexpected String(): %q %q", OTLPGRPC.String(), OTLPHTTP.String())
	}
	if err := OTLPProtocol(99).Validate(); err == nil {
		t.Fatal("Validate(99) = nil, want rejection of unknown enum")
	}
}
