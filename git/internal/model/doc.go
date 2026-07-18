// Package model defines the internal value types shared across the git packages.
//
// Types such as [Oid], [Reference], [Signature], and [Commit] model core git objects independently of any backend so the higher-level git API and its adapters exchange a single representation.
package model
