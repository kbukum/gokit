package model

import "testing"

func TestWithExtraArgsAppends(t *testing.T) {
	t.Parallel()

	opts := ApplyOptions(
		WithExtraArgs("--first"),
		WithExtraArgs("--second", "--third"),
	)

	want := []string{"--first", "--second", "--third"}
	if len(opts.ExtraArgs) != len(want) {
		t.Fatalf("ExtraArgs len = %d, want %d", len(opts.ExtraArgs), len(want))
	}
	for i := range want {
		if opts.ExtraArgs[i] != want[i] {
			t.Fatalf("ExtraArgs[%d] = %q, want %q", i, opts.ExtraArgs[i], want[i])
		}
	}
}
