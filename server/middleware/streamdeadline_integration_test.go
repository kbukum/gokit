package middleware

import (
	"crypto/tls"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kbukum/gokit/util"
)

// The server's WriteTimeout is an absolute per-stream deadline under HTTP/2; the middleware lets a stream that keeps
// writing outlive it.
func TestStreamWriteDeadlineOutlivesServerWriteTimeoutOverHTTP2(t *testing.T) {
	const writeTimeout = 150 * time.Millisecond
	stream := func(w http.ResponseWriter, r *http.Request) {
		rc := http.NewResponseController(w)
		for i := range 4 {
			if i > 0 {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(writeTimeout / 2):
				}
			}
			if _, err := w.Write([]byte{'a' + byte(i)}); err != nil {
				return
			}
			if err := rc.Flush(); err != nil {
				return
			}
		}
	}
	read := func(t *testing.T, h http.Handler) (string, error) {
		t.Helper()
		srv := httptest.NewUnstartedServer(h)
		srv.EnableHTTP2 = true
		srv.Config.WriteTimeout = writeTimeout
		srv.StartTLS()
		t.Cleanup(srv.Close)
		client := srv.Client()
		client.Transport.(*http.Transport).TLSClientConfig.MinVersion = tls.VersionTLS13
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, srv.URL, http.NoBody)
		require.NoError(t, err)
		resp, err := client.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		require.Equal(t, 2, resp.ProtoMajor)
		body, err := io.ReadAll(resp.Body)
		return string(body), err
	}

	plain, err := read(t, http.HandlerFunc(stream))
	require.Error(t, err, "without the middleware the stream is reset at the server write timeout")
	assert.NotEqual(t, "abcd", plain)

	body, err := read(t, streamWriteDeadline(t, time.Second, util.SystemClock{})(http.HandlerFunc(stream)))
	require.NoError(t, err)
	assert.Equal(t, "abcd", body)
}
