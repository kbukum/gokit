package testutil

// TLSFiles are caller-owned certificate paths that must outlive the fixture. The container owns its mounted copies.
type TLSFiles struct {
	CAFile, CertFile, KeyFile string
}

type fixtureOptions struct{ tls *TLSFiles }

// Option configures an isolated PostgreSQL fixture.
type Option func(*fixtureOptions)

// WithTLS enables a TLS 1.3 server and verified client preparation using the supplied test CA.
func WithTLS(files TLSFiles) Option {
	return func(opts *fixtureOptions) { opts.tls = &files }
}
