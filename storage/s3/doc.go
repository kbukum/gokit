// Package s3 implements the opt-in [storage.Storage] adapter backed by
// Amazon S3 (and S3-compatible providers like MinIO and DigitalOcean Spaces).
//
// Importing this package has no side effects. Call Register with an explicit
// storage.FactoryRegistry before selecting the s3 provider.
package s3
