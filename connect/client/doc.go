// Package client provides HTTP clients configured for ConnectRPC services.
//
// ConnectRPC clients are built on standard net/http. Unlike gRPC,
// there is no special dial/connection step — you just need an *http.Client configured for HTTP/2
// and a base URL.
//
// When TLS is not configured (the default), the client uses h2c (cleartext HTTP/2).
// When TLS is configured via security.TLSConfig, standard HTTPS is used.
//
// # Basic Usage (static address)
//
// Create an HTTP client and use it with any generated Connect client:
//
//	cfg := client.Config{BaseURL: "http://localhost:8080"}
//	httpClient, err := client.NewHTTPClient(cfg)
//	svcClient := myv1connect.NewMyServiceClient(httpClient, cfg.BaseURL)
//
// # With Service Discovery (via provider.Connector)
//
// Use provider.Connector for deferred initialization with resilience:
//
//	c := provider.NewConnector(provider.ConnectorConfig[myv1connect.MyServiceClient]{
//	    ServiceName: "my-service",
//	    Create: func() (myv1connect.MyServiceClient, error) {
//	        url, err := discovery.Resolve("my-service")
//	        if err != nil { return nil, err }
//	        return myv1connect.NewMyServiceClient(httpClient, url), nil
//	    },
//	    Resilience: &provider.ResilienceConfig{...},
//	})
//	resp, err := provider.Call(ctx, c, func(svc myv1connect.MyServiceClient) (*Resp, error) {
//	    return svc.DoThing(ctx, connect.NewRequest(req))
//	})
//
// # With gRPC Protocol (required for bidi streaming)
//
//	cfg := client.Config{BaseURL: "http://localhost:8080", Protocol: client.ProtocolGRPC}
//	httpClient, err := client.NewHTTPClient(cfg)
//	opts := client.ClientOptions(cfg)
//	svcClient := myv1connect.NewMyServiceClient(httpClient, cfg.BaseURL, opts...)
//
// # Long-lived streams
//
// A stream client sets NoTimeout and bounds the wait for the first message instead. FirstMessageTimeout goes after Availability, so a peer that never answers is recorded as unavailable, and Unanswered maps the failure for the caller:
//
//	cfg := client.Config{BaseURL: "http://localhost:8080", NoTimeout: true}
//	availability := client.NewAvailability("feed")
//	first, err := client.FirstMessageTimeout(time.Second, util.SystemClock{})
//	opts := append(client.ClientOptions(cfg), connect.WithInterceptors(availability, first))
//	...
//	if appErr, ok := client.Unanswered(ctx, "feed", stream.Err()); ok {
//	    return appErr
//	}
//
// # With TLS
//
//	cfg := client.Config{
//	    BaseURL: "https://api.example.com:443",
//	    TLS:     &security.TLSConfig{CAFile: "/path/to/ca.pem"},
//	}
//	httpClient, err := client.NewHTTPClient(cfg)
package client
