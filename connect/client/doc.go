// Package client provides an h2c-capable HTTP client for ConnectRPC services.
//
// ConnectRPC clients are built on standard net/http. Unlike gRPC, there is no
// special dial/connection step — you just need an *http.Client configured for
// HTTP/2 cleartext (h2c) and a base URL.
//
// This package provides:
//   - Config for client configuration (base URL, timeouts)
//   - NewHTTPClient for creating an h2c-capable *http.Client
//   - Protocol option helpers (WithGRPC, WithGRPCWeb, WithConnect)
//
// # Basic Usage
//
// Create an h2c HTTP client and use it with any generated Connect client:
//
//	cfg := client.Config{BaseURL: "http://localhost:8080"}
//	httpClient := client.NewHTTPClient(cfg)
//	svcClient := myv1connect.NewMyServiceClient(httpClient, cfg.BaseURL)
//
// # With gRPC Protocol (required for bidi streaming)
//
//	svcClient := myv1connect.NewMyServiceClient(
//	    httpClient,
//	    cfg.BaseURL,
//	    client.WithGRPC(),
//	)
package client
