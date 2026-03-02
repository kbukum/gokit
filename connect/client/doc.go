// Package client provides HTTP clients configured for ConnectRPC services.
//
// ConnectRPC clients are built on standard net/http. Unlike gRPC, there is no
// special dial/connection step — you just need an *http.Client configured for
// HTTP/2 and a base URL.
//
// When TLS is not configured (the default), the client uses h2c (cleartext
// HTTP/2). When TLS is configured via security.TLSConfig, standard HTTPS is used.
//
// # Basic Usage (static address)
//
// Create an HTTP client and use it with any generated Connect client:
//
//	cfg := client.Config{BaseURL: "http://localhost:8080"}
//	httpClient, err := client.NewHTTPClient(cfg)
//	svcClient := myv1connect.NewMyServiceClient(httpClient, cfg.BaseURL)
//
// # With Service Discovery (dynamic address)
//
// When using service discovery, BaseURL is resolved at call time:
//
//	cfg := client.Config{}  // no BaseURL needed for transport
//	httpClient, err := client.NewHTTPClient(cfg)
//	endpoint, _ := disc.DiscoverOne(ctx, query)
//	baseURL := fmt.Sprintf("http://%s", endpoint.HostPort())
//	svcClient := myv1connect.NewMyServiceClient(httpClient, baseURL)
//
// # With gRPC Protocol (required for bidi streaming)
//
//	cfg := client.Config{BaseURL: "http://localhost:8080", Protocol: client.ProtocolGRPC}
//	httpClient, err := client.NewHTTPClient(cfg)
//	opts := client.ClientOptions(cfg)
//	svcClient := myv1connect.NewMyServiceClient(httpClient, cfg.BaseURL, opts...)
//
// # With TLS
//
//	cfg := client.Config{
//	    BaseURL: "https://api.example.com:443",
//	    TLS:     &security.TLSConfig{CAFile: "/path/to/ca.pem"},
//	}
//	httpClient, err := client.NewHTTPClient(cfg)
package client
