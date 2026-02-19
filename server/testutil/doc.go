// Package testutil provides testing utilities for the server module.
//
// It includes a test server component backed by httptest.Server that
// implements both component.Component and testutil.TestComponent interfaces.
//
// # Quick Start
//
//	srv := testutil.NewComponent()
//	testutil.T(t).Setup(srv)
//
//	// Register routes on the Gin engine
//	srv.GinEngine().GET("/hello", func(c *gin.Context) {
//	    c.String(200, "world")
//	})
//
//	// Make requests using the base URL
//	resp, _ := http.Get(srv.BaseURL() + "/hello")
package testutil
