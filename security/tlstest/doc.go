// Package tlstest generates throwaway TLS material for tests.
//
// [GenerateTLSCerts] produces an in-memory certificate and key pair,
// and [WriteInvalidPEM] emits deliberately malformed PEM, so TLS success
// and failure paths can be exercised without external fixtures.
package tlstest
