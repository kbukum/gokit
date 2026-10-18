package connect

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	connectrpc "connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"
)

// The interceptor presents its value as the one header value on unary and streaming client calls, replacing a value
// the caller set.
func TestHeaderInterceptorSetsClientHeader(t *testing.T) {
	t.Parallel()
	var (
		mu  sync.Mutex
		got []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Header.Values("X-Api-Key")...)
		mu.Unlock()
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	header, err := HeaderInterceptor("X-API-Key", "secret")
	if err != nil {
		t.Fatal(err)
	}
	unary := connectrpc.NewClient[emptypb.Empty, emptypb.Empty](srv.Client(), srv.URL+"/svc.v1.S/Unary", connectrpc.WithInterceptors(header))
	req := connectrpc.NewRequest(&emptypb.Empty{})
	req.Header().Set("X-API-Key", "caller-supplied")
	_, _ = unary.CallUnary(t.Context(), req)

	stream := connectrpc.NewClient[emptypb.Empty, emptypb.Empty](srv.Client(), srv.URL+"/svc.v1.S/Stream", connectrpc.WithInterceptors(header))
	if s, err := stream.CallServerStream(context.Background(), connectrpc.NewRequest(&emptypb.Empty{})); err == nil {
		s.Receive()
		_ = s.Close()
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) != 2 || got[0] != "secret" || got[1] != "secret" {
		t.Fatalf("presented = %q", got)
	}
}

// On a handler the interceptor changes nothing.
func TestHeaderInterceptorLeavesHandlers(t *testing.T) {
	t.Parallel()
	header, err := HeaderInterceptor("X-API-Key", "secret")
	if err != nil {
		t.Fatal(err)
	}
	var seen string
	next := func(_ context.Context, req connectrpc.AnyRequest) (connectrpc.AnyResponse, error) {
		seen = req.Header().Get("X-API-Key")
		return nil, nil //nolint:nilnil // the test inspects only the request.
	}
	req := connectrpc.NewRequest(&emptypb.Empty{})
	_, _ = header.WrapUnary(next)(t.Context(), req)
	if seen != "" {
		t.Fatalf("handler request header = %q", seen)
	}
	streaming := func(context.Context, connectrpc.StreamingHandlerConn) error { return nil }
	if err := header.WrapStreamingHandler(streaming)(t.Context(), nil); err != nil {
		t.Fatal(err)
	}
}

func TestHeaderInterceptorRejectsInvalidHeaders(t *testing.T) {
	t.Parallel()
	for name, c := range map[string][2]string{
		"no name":  {"", "v"},
		"bad name": {"X Key", "v"},
		"no value": {"X-Key", ""},
		"newline":  {"X-Key", "a\r\nb"},
	} {
		if _, err := HeaderInterceptor(c[0], c[1]); err == nil {
			t.Fatalf("%s: want error", name)
		}
	}
}
