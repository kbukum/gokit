// Package storage provides object storage abstractions with pluggable backends
// for gokit applications.
//
// It defines interfaces for common storage operations (upload, download, delete, list)
// and follows gokit's component pattern with lifecycle management.
//
// # Optional capabilities
//
// Single-object metadata, copy, move, and signed URLs are opt-in capability
// interfaces ([HeadProvider], [CopyProvider], [RenameProvider], and
// [SignedURLProvider]) rather than required methods. Call the package-level
// [Head], [Copy], and [Rename] helpers to use a backend's native operation when
// it has one and a portable fallback when it does not.
//
// # Backends
//
//   - storage/s3: Amazon S3 and S3-compatible storage
//   - storage/local: Local filesystem storage for development/testing
//   - storage/supabase: Supabase Storage integration
//
// # Configuration
//
// Backend selection and settings are provided via Config:
//
//	storage:
//	  provider: "s3"
//	  s3:
//	    bucket: "my-bucket"
//	    region: "us-east-1"
package storage
