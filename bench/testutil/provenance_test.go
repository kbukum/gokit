package testutil

import "testing"

func TestFixedProvenance(t *testing.T) {
	t.Parallel()
	p := NewFixedProvenanceProbe(WithGitCommit("abc"), WithGitTreeState("dirty"), WithHost("host"), WithOS("os"), WithArch("arch"))
	if p.GitCommit() != "abc" || p.GitTreeState() != "dirty" || p.Host() != "host" || p.OS() != "os" || p.Arch() != "arch" {
		t.Fatalf("probe=%+v", p)
	}
}
