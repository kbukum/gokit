package encryption

import (
	"os"
	"testing"
)

// UseFastIterations reduces PBKDF2 iterations for test speed.
// Call in TestMain or at the top of tests that don't need production-strength KDF.
func UseFastIterations(tb testing.TB) {
	tb.Helper()
	old := pbkdf2Iterations
	pbkdf2Iterations = 1000
	tb.Cleanup(func() { pbkdf2Iterations = old })
}

func TestMain(m *testing.M) {
	pbkdf2Iterations = 1000
	os.Exit(m.Run())
}
