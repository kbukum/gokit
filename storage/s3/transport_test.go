package s3

import (
	"bufio"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"testing/synctest"
	"time"
)

func TestS3HTTPClientRequestBudget(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name         string
		headerDelay  time.Duration
		bodyDelay    time.Duration
		callerBudget time.Duration
		wantElapsed  time.Duration
		wantErr      error
	}{
		{name: "slow headers use the request budget", headerDelay: 3 * time.Second, wantElapsed: 3 * time.Second},
		{name: "headers cannot exceed the request budget", headerDelay: 11 * time.Second, wantElapsed: 10 * time.Second, wantErr: context.DeadlineExceeded},
		{name: "caller deadline wins", headerDelay: 3 * time.Second, callerBudget: time.Second, wantElapsed: time.Second, wantErr: context.DeadlineExceeded},
		{name: "body shares the request budget", headerDelay: 3 * time.Second, bodyDelay: 8 * time.Second, wantElapsed: 10 * time.Second, wantErr: context.DeadlineExceeded},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			synctest.Test(t, func(t *testing.T) {
				client := newHTTPClient()
				defer client.CloseIdleConnections()
				transport, ok := client.Transport.(*http.Transport)
				if !ok {
					t.Fatal("S3 client has no HTTP transport")
				}
				local, remote := net.Pipe()
				t.Cleanup(func() {
					if err := local.Close(); err != nil {
						t.Error(err)
					}
				})
				transport.DialContext = func(context.Context, string, string) (net.Conn, error) {
					return local, nil
				}
				ctx := t.Context()
				if tt.callerBudget != 0 {
					var finish context.CancelFunc
					ctx, finish = context.WithTimeout(ctx, tt.callerBudget)
					defer finish()
				}
				serverCtx, stopServer := context.WithCancel(t.Context())
				defer stopServer()
				request, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://s3.test/object", strings.NewReader("value"))
				if err != nil {
					t.Fatal(err)
				}
				served := make(chan error, 1)
				go func() {
					served <- serveDelayedResponse(serverCtx, remote, tt.headerDelay, tt.bodyDelay)
				}()
				start := time.Now()
				response, err := client.Do(request)
				if err == nil {
					_, readErr := io.Copy(io.Discard, response.Body)
					err = errors.Join(readErr, response.Body.Close())
				}
				elapsed := time.Since(start)
				stopServer()
				if serveErr := <-served; serveErr != nil {
					t.Fatal(serveErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("request error = %v; want %v", err, tt.wantErr)
				}
				if elapsed != tt.wantElapsed {
					t.Fatalf("request took %s; want %s", elapsed, tt.wantElapsed)
				}
			})
		})
	}
}

func serveDelayedResponse(ctx context.Context, conn net.Conn, headerDelay, bodyDelay time.Duration) (err error) {
	defer func() { err = errors.Join(err, conn.Close()) }()
	request, err := http.ReadRequest(bufio.NewReader(conn))
	if err != nil {
		return err
	}
	_, err = io.Copy(io.Discard, request.Body)
	if readErr := errors.Join(err, request.Body.Close()); readErr != nil {
		return readErr
	}
	for _, part := range []struct {
		delay time.Duration
		data  string
	}{
		{headerDelay, "HTTP/1.1 200 OK\r\nContent-Length: 2\r\nConnection: close\r\n\r\n"},
		{bodyDelay, "ok"},
	} {
		timer := time.NewTimer(part.delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
		if _, err := io.WriteString(conn, part.data); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
	return nil
}
