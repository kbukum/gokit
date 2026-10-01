package client

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/kbukum/gokit/security"
)

func TestTLSTransportRejectsCleartext(t *testing.T) {
	t.Parallel()
	for _, redirect := range []bool{false, true} {
		name := "direct"
		if redirect {
			name = "redirect"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int64
			cleartext := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(cleartext.Close)
			target := cleartext.URL
			if redirect {
				server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, cleartext.URL, http.StatusTemporaryRedirect)
				}))
				server.EnableHTTP2 = true
				server.StartTLS()
				t.Cleanup(server.Close)
				target = server.URL
			}
			client, err := NewHTTPClient(Config{TLS: &security.TLSConfig{SkipVerify: true}})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.CloseIdleConnections)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, target, http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("Authorization", "Bearer test-token")
			resp, err := client.Do(req)
			if resp != nil {
				resp.Body.Close()
			}
			if err == nil {
				t.Error("TLS client accepted a cleartext request")
			}
			if requests.Load() != 0 {
				t.Errorf("cleartext requests = %d; want none", requests.Load())
			}
		})
	}
}

func TestH2CTransportRejectsHTTPS(t *testing.T) {
	t.Parallel()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("h2c client sent an HTTPS request")
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(server.Close)
	client, err := NewHTTPClient(Config{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.CloseIdleConnections)
	transport, ok := client.Transport.(*schemeTransport)
	if !ok {
		t.Fatal("expected native HTTP transport")
	}
	serverTransport, ok := server.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatal("expected native test server transport")
	}
	transport.TLSClientConfig = serverTransport.TLSClientConfig
	req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Do(req)
	if resp != nil {
		resp.Body.Close()
	}
	if err == nil {
		t.Error("h2c client accepted an HTTPS request")
	}
}

func TestHTTP2TransportRoundTrip(t *testing.T) {
	t.Parallel()
	for _, encrypted := range []bool{false, true} {
		name := "h2c"
		if encrypted {
			name = "tls"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.ProtoMajor != 2 {
					t.Errorf("protocol = %s; want HTTP/2", r.Proto)
				}
				_, _ = io.WriteString(w, "ok")
			}))
			cfg := Config{}
			if encrypted {
				server.EnableHTTP2 = true
				server.StartTLS()
				cfg.TLS = &security.TLSConfig{SkipVerify: true}
			} else {
				server.Config.Protocols = new(http.Protocols)
				server.Config.Protocols.SetUnencryptedHTTP2(true)
				server.Start()
			}
			t.Cleanup(server.Close)
			client, err := NewHTTPClient(cfg)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(client.CloseIdleConnections)
			req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, server.URL, http.NoBody)
			if err != nil {
				t.Fatal(err)
			}
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || string(body) != "ok" || resp.ProtoMajor != 2 {
				t.Fatalf("round trip: protocol=%s body=%q error=%v", resp.Proto, body, err)
			}
		})
	}
}
