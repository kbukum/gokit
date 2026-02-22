// Package testutil provides testing utilities for ConnectRPC services.
//
// It wraps net/http/httptest to create lightweight test servers that host
// ConnectRPC handlers. No gokit/server dependency — just httptest + Connect.
//
// # Quick Start
//
//	// Create a test server and mount your Connect handler
//	srv := testutil.NewServer()
//	path, handler := mypbconnect.NewMyServiceHandler(&myHandler{})
//	srv.Mount(path, handler)
//
//	// Start and auto-cleanup
//	testutil.T(t).Setup(srv)
//
//	// Create a real ConnectRPC client against the test server
//	client := mypbconnect.NewMyServiceClient(http.DefaultClient, srv.BaseURL())
package testutil
