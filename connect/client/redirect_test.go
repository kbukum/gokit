package client

import (
	"encoding/pem"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/kbukum/gokit/security"
)

func TestHTTPClientRejectsRedirects(t *testing.T) {
	t.Parallel()
	for _, tls := range []bool{false, true} {
		for _, relative := range []bool{false, true} {
			for _, status := range []int{301, 302, 303, 307, 308} {
				name := "tls=" + strconv.FormatBool(tls) + "/relative=" + strconv.FormatBool(relative) + "/" + strconv.Itoa(status)
				t.Run(name, func(t *testing.T) {
					t.Parallel()
					var forwarded atomic.Int64
					target := h2cServer(t, func(w http.ResponseWriter, _ *http.Request) {
						forwarded.Add(1)
						w.WriteHeader(http.StatusOK)
					})
					destination := strings.Replace(target.URL, "127.0.0.1", "localhost", 1)
					if relative {
						destination = "/redirected"
					}
					source := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.URL.Path == "/redirected" {
							forwarded.Add(1)
							w.WriteHeader(http.StatusOK)
							return
						}
						http.Redirect(w, r, destination, status)
					}))
					cfg := Config{}
					if tls {
						source.EnableHTTP2 = true
						source.StartTLS()
						ca := filepath.Join(t.TempDir(), "ca.pem")
						if err := os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: source.Certificate().Raw}), 0o600); err != nil {
							t.Fatal(err)
						}
						cfg.TLS = &security.TLSConfig{CAFile: ca}
					} else {
						source.Config.Protocols = new(http.Protocols)
						source.Config.Protocols.SetUnencryptedHTTP2(true)
						source.Start()
					}
					t.Cleanup(source.Close)
					client, err := NewHTTPClient(cfg)
					if err != nil {
						t.Fatal(err)
					}
					t.Cleanup(client.CloseIdleConnections)
					req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, source.URL, strings.NewReader("test payload"))
					if err != nil {
						t.Fatal(err)
					}
					req.Header.Set("X-API-Key", "test-placeholder")
					req.Header.Set("Delegation", "test-placeholder")
					resp, err := client.Do(req)
					if resp != nil {
						resp.Body.Close()
					}
					if !errors.Is(err, ErrRedirect) {
						t.Errorf("redirect error = %v, want ErrRedirect", err)
					}
					if got := forwarded.Load(); got != 0 {
						t.Errorf("redirected requests = %d, want none", got)
					}
				})
			}
		}
	}
}
