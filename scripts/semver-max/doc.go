// Command semver-max reads whitespace-separated semantic versions from stdin and prints the
// single SemVer-highest one, exiting non-zero if none are valid. It exists so the release
// procedure can pick the highest version published across every module proxy path with
// correct SemVer precedence (unlike `sort -V`, which orders a prerelease after its release)
// using the repository's pinned, reviewed SemVer dependency rather than an ad-hoc `go get`.
package main
