// Package tlstest generates throwaway TLS material for tests.
//
// [GenerateTLSCerts] creates a CA and a leaf certificate with Go's crypto standard library and writes PEM files under
// t.TempDir, so they are removed when the test ends. [WithHosts] overrides the leaf's names for hostname-rejection
// proofs, and [WriteInvalidPEM] emits deliberately malformed PEM for failure paths.
//
// The package lives in the root gokit module so root and sub-module tests can import it without dependency cycles.
package tlstest
