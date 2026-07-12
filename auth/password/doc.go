// Package password hashes and verifies user passwords using a configurable,
// current-generation algorithm.
//
// [NewHasher] selects an implementation by [Algorithm] (bcrypt by default) so
// call sites depend on the [Hasher] interface rather than a specific primitive,
// allowing the default cost and algorithm to evolve without API changes.
package password
