// Package security provides shared security primitives for gokit modules.
//
// It includes TLS configuration, certificate handling, and secure-by-default
// HTTP response header policies that can be reused across transports.
//
// # TLS Configuration
//
//	cfg := security.TLSConfig{
//	    CAFile:   "/path/to/ca.pem",
//	    CertFile: "/path/to/cert.pem",
//	    KeyFile:  "/path/to/key.pem",
//	}
//
//	tlsConfig, err := cfg.Build()
package security
