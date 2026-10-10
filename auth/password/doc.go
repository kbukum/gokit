// Package password hashes and verifies user passwords.
//
// [NewHasher] validates a [Config] and returns a [Hasher] for argon2id (the default) or bcrypt. Hash enforces the
// configured length policy in characters; Verify distinguishes a mismatch (UNAUTHORIZED, [ReasonMismatch]) from a
// corrupt stored hash (INTERNAL, [ReasonCorruptHash]) and bounds the cost a stored hash may demand. [Pool] admits
// hashing work through a bounded worker pool and queue and refuses with RATE_LIMITED ([ReasonBusy]) before any
// hashing starts.
//
//	hasher, err := password.NewHasher(password.Config{MinLength: 15})
//	pool, err := password.NewPool(hasher, password.PoolConfig{Workers: 4, Queue: 64, MaxWait: time.Second})
//	err = pool.Verify(ctx, presented, stored)
package password
