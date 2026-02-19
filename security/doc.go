// Package security provides shared security primitives for gokit modules.
//
// It includes TLS configuration and certificate handling that can be reused
// across HTTP, gRPC, Kafka, and other transport modules.
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
