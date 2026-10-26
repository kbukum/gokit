package s3

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"
	"time"
)

func newHTTPClient() *http.Client {
	dialer := &net.Dialer{Timeout: 2 * time.Second, KeepAlive: 30 * time.Second}
	return &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("storage redirects are not allowed")
		},
		Transport: &http.Transport{
			DialContext: dialer.DialContext, ForceAttemptHTTP2: true,
			TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS13},
			TLSHandshakeTimeout: 2 * time.Second,
			IdleConnTimeout:     30 * time.Second, MaxIdleConns: 8, MaxIdleConnsPerHost: 8, MaxConnsPerHost: 8,
		},
	}
}

// Close releases this backend's idle HTTP connections after callers have drained.
func (s *Storage) Close(context.Context) error {
	s.http.CloseIdleConnections()
	return nil
}
