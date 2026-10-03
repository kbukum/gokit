// Package testutil provides testing utilities for the server module.
//
// It includes a test server component backed by httptest.Server that implements both component.Component
// and testutil.TestComponent interfaces.
//
// Reset preserves mounted routes and the live origin. Use Stop/Start for restart. Stop honors its deadline, closes remaining connections on expiry, and returns the drain failure; handlers must honor request cancellation.
//
// # Quick Start
//
//	srv := testutil.NewComponent()
//
//	// Register routes on the Gin engine
//	srv.GinEngine().GET("/hello", func(c *gin.Context) {
//	    c.String(200, "world")
//	})
//	testutil.T(t).Setup(srv)
//
//	// Make requests using the base URL
//	resp, _ := http.Get(srv.BaseURL() + "/hello")
package testutil
