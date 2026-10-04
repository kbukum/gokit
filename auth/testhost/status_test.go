package testhost

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/kbukum/gokit/codec"
)

func TestLateStatusResponseCannotRenewLoggedOutCookie(t *testing.T) {
	t.Parallel()
	host, client, fixture := liveHost(t)
	document := login(t, host, client, fixture)
	hold := control(t, host, client, fixture, "/_test/scenario", `{"name":"hold-status"}`)
	if hold.StatusCode != http.StatusNoContent {
		t.Fatal("status hold failed")
	}
	type result struct {
		response *http.Response
		err      error
	}
	results := make(chan result, 1)
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("status request did not stop after cancellation")
		}
		select {
		case pending := <-results:
			if pending.response != nil {
				_ = pending.response.Body.Close()
			}
		default:
		}
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, host.Origin()+"/auth/session", http.NoBody)
	if err != nil {
		t.Fatal(err)
	}
	go func() {
		defer close(done)
		response, err := client.Do(req)
		if response != nil {
			err = errors.Join(err, response.Body.Close())
		}
		results <- result{response, err}
	}()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	waiting := true
	for waiting {
		select {
		case <-deadline.C:
			t.Fatal("authoritative status response was not held within two seconds")
		case <-tick.C:
			response := request(t, client, http.MethodGet, host.Origin()+"/_test/state", "", "")
			data, err := io.ReadAll(io.LimitReader(response.Body, 1024))
			closeErr := response.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			if closeErr != nil {
				t.Fatal(closeErr)
			}
			state, err := codec.Decode[struct {
				StatusPending bool `json:"statusPending"`
			}](codec.CompactJSON(), string(data))
			if err != nil {
				t.Fatal(err)
			}
			waiting = !state.StatusPending
		}
	}
	logout := request(t, client, http.MethodPost, host.Origin()+"/auth/logout", "{}", document.CSRFToken)
	if logout.StatusCode != http.StatusNoContent {
		t.Fatal("concurrent logout failed")
	}
	release := control(t, host, client, fixture, "/_test/scenario", `{"name":"release-status"}`)
	if release.StatusCode != http.StatusNoContent {
		t.Fatal("status release failed")
	}
	select {
	case result := <-results:
		if result.err != nil {
			t.Fatal(result.err)
		}
		defer result.response.Body.Close()
		if result.response.StatusCode != http.StatusOK || len(result.response.Cookies()) != 0 {
			t.Fatal("late pre-logout status renewed credentials or lost fixture ordering")
		}
	case <-time.After(time.Second):
		t.Fatal("released status response remained blocked")
	}
	status := request(t, client, http.MethodGet, host.Origin()+"/auth/session", "", "")
	if status.StatusCode != http.StatusUnauthorized {
		t.Fatal("late status response resurrected database session")
	}
}
