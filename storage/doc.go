// Package storage provides object storage abstractions with pluggable backends
// for gokit applications.
//
// It defines interfaces for common storage operations (upload, download, delete,
// list) and follows gokit's component pattern with lifecycle management.
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
