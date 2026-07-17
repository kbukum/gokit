// Package stage defines the generic streaming stages of the dataset kit: a
// [Source] that produces a bounded stream, a [Transform] that maps and filters
// it, and a [Target] that publishes it. Stages are generics-first (no
// interface{}/any) and compose over [github.com/kbukum/gokit/stream] pipelines
// rather than a parallel streaming stack.
package stage
