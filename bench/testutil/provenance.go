package testutil

// FixedProvenanceProbe is a deterministic bench.ProvenanceProbe for tests and
// reproducible fixtures: it returns fixed, injected values and performs no
// environment or process access. The zero value reports empty strings and no git
// commit; configure it with the functional options below.
type FixedProvenanceProbe struct {
	gitCommit string
	host      string
	os        string
	arch      string
}

// ProbeOption configures a FixedProvenanceProbe.
type ProbeOption func(*FixedProvenanceProbe)

// NewFixedProvenanceProbe builds a fixed probe from the given options.
func NewFixedProvenanceProbe(opts ...ProbeOption) FixedProvenanceProbe {
	var p FixedProvenanceProbe
	for _, o := range opts {
		o(&p)
	}
	return p
}

// WithGitCommit sets the commit the probe reports.
func WithGitCommit(commit string) ProbeOption {
	return func(p *FixedProvenanceProbe) { p.gitCommit = commit }
}

// WithHost sets the host name the probe reports.
func WithHost(host string) ProbeOption {
	return func(p *FixedProvenanceProbe) { p.host = host }
}

// WithOS sets the operating system identifier the probe reports.
func WithOS(os string) ProbeOption {
	return func(p *FixedProvenanceProbe) { p.os = os }
}

// WithArch sets the CPU architecture identifier the probe reports.
func WithArch(arch string) ProbeOption {
	return func(p *FixedProvenanceProbe) { p.arch = arch }
}

// GitCommit returns the injected commit, or "" when unset.
func (p FixedProvenanceProbe) GitCommit() string { return p.gitCommit }

// Host returns the injected host name.
func (p FixedProvenanceProbe) Host() string { return p.host }

// OS returns the injected operating system identifier.
func (p FixedProvenanceProbe) OS() string { return p.os }

// Arch returns the injected CPU architecture identifier.
func (p FixedProvenanceProbe) Arch() string { return p.arch }
