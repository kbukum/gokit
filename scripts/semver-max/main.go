// Command semver-max reads whitespace-separated semantic versions from stdin and prints the
// single SemVer-highest one, exiting non-zero if none are valid. It exists so the release
// procedure can pick the highest version published across every module proxy path with
// correct SemVer precedence (unlike `sort -V`, which orders a prerelease after its release)
// using the repository's pinned, reviewed SemVer dependency rather than an ad-hoc `go get`.
package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"

	"github.com/kbukum/gokit/version"
)

func main() {
	var (
		best    *semver.Version
		bestRaw string
	)
	sc := bufio.NewScanner(os.Stdin)
	sc.Split(bufio.ScanWords)
	for sc.Scan() {
		raw := sc.Text()
		// Go module proxy tags carry a leading "v" (v1.2.3); the strict parser wants the
		// bare semantic version, so trim it for parsing while keeping raw for output.
		v, err := version.ParseVersion(strings.TrimPrefix(raw, "v"))
		if err != nil {
			// Ignore non-version tokens (blank proxy responses, stray lines): only valid
			// semantic versions participate in the maximum.
			continue
		}
		if best == nil || v.GreaterThan(best) {
			best, bestRaw = v, raw
		}
	}
	if err := sc.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "semver-max: read error:", err)
		os.Exit(1)
	}
	if best == nil {
		fmt.Fprintln(os.Stderr, "semver-max: no valid semantic versions on input")
		os.Exit(1)
	}
	fmt.Println(bestRaw)
}
