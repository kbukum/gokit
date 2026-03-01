// Package rest provides a JSON-focused REST client built on the HTTP adapter.
//
// It inherits all features from httpclient (auth, TLS, resilience, streaming)
// and adds typed convenience methods for common REST operations:
//
//	client := rest.New(httpclient.Config{
//	    BaseURL: "https://api.example.com",
//	    Auth:    httpclient.BearerAuth("token"),
//	    Retry:   httpclient.DefaultRetryConfig(),
//	})
//
//	// Typed GET
//	user, err := rest.Get[User](ctx, client, "/users/123")
//
//	// Typed POST
//	created, err := rest.Post[User](ctx, client, "/users", CreateUserRequest{Name: "Alice"})
//
// Alternatively, use the generic functions directly on the HTTP adapter:
//
//	adapter, _ := httpclient.New(cfg)
//	user, err := httpclient.Get[User](adapter, ctx, "/users/123")
package rest
