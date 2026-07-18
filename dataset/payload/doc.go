// Package payload owns bounded materialization for the dataset kit:
// the [Limits] that cap in-memory use and the [Payload] that either holds bytes in memory
// or spills to a file when they exceed the cap.
//
// It is a leaf of the dataset kit — every other sub-package that needs a size bound
// or a byte payload depends on this package rather than re-deriving the caps.
package payload
