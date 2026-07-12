// Package value merges codec value trees with configurable array semantics.
//
// [Merge] overlays one decoded value tree onto another: objects merge
// recursively and, by default, later scalars win. [MergeWith] takes an
// [ArrayStrategy] to choose how arrays combine (replace, concatenate, …) per
// use case such as layered configuration.
package value
