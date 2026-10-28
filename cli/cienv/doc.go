// Package cienv reads which CI system runs the program and the commit it builds, from the environment variables each system sets.
//
// [Detect] takes an injected lookup, such as [os.LookupEnv], and returns the provider, repository, commit, branch and pipeline as labels. It reads only these well-known, non-secret variables, so the labels are safe to send and record.
package cienv
