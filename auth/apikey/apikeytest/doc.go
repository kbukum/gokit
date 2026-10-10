// Package apikeytest is the behavioral contract every [apikey.Store] must satisfy.
//
// Call [Run] from a store's own tests with a constructor for empty stores. The suite covers digest and id lookup,
// metadata isolation, one-way revocation, and rotation as a single compare-and-swap that cannot race revocation or
// another rotation.
//
//	func TestStoreContract(t *testing.T) {
//		apikeytest.Run(t, apikeytest.Harness{NewStore: func(t *testing.T) apikey.Store { return newStore(t) }})
//	}
package apikeytest
