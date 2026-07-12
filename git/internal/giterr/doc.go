// Package giterr defines the internal typed error values shared across the git
// packages.
//
// Sentinel errors such as [RepoNotFound], [RefNotFound], and [Conflict] give
// git operations a common, matchable vocabulary via errors.Is without exposing
// these internals outside the module.
package giterr
