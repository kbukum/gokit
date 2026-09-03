// Package testutil provides a reusable harness for testing SSE endpoints wired
// through gokit's sse package — especially the injected authentication seam.
//
// It offers three building blocks so consumers never hand-roll SSE fakes:
//
//   - [FakeAuthenticator]: a call-counting [sse.Authenticator] that allows a
//     fixed identity or rejects with a chosen 401/403 error.
//   - [Harness]: a running [sse.Hub] behind an httptest.Server that serves an SSE
//     endpoint with caller-supplied [sse.ServeOption]s, with automatic cleanup.
//   - [StreamClient]: an SSE stream reader that decodes framed events and lets
//     tests assert on event names, JSON payloads, and the reconnect id.
//
// # Quick Start
//
//	auth := testutil.AllowAuthenticator("user-1")
//	h := testutil.New(t, "base",
//	    sse.WithAuthenticator(auth),
//	    sse.WithClientIdentity(func(_ *http.Request, id any) (string, []sse.ClientOption, error) {
//	        return "user:" + id.(string), nil, nil
//	    }),
//	)
//
//	stream := h.MustConnect(t, ctx, "valid-token")
//	defer stream.Close()
//	testutil.RequireStatus(t, stream, http.StatusOK)
//
//	h.Hub.BroadcastFrame("user:user-1", sse.Frame{Event: "ping", Data: []byte(`{"n":1}`)})
//	evt := stream.Require(t, "ping")
//	// evt.Data == {"n":1}
package testutil
