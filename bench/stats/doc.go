// Package stats summarizes small evaluation samples: a seeded percentile bootstrap interval for a mean ([BootstrapMean]) and a Bradley–Terry ranking from pairwise outcomes ([BradleyTerry]) with seeded bootstrap intervals.
//
// Every random draw comes from a ChaCha8 generator seeded by the caller, so the same input and seed always give the same interval. Inputs are bounded and validated; a non-finite value is an error, never a silently dropped sample.
package stats
