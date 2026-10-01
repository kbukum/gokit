// Package gcs implements the opt-in [storage.Storage] adapter backed by Google Cloud Storage.
//
// Importing this package has no side effects.
// Call Register with an explicit storage.FactoryRegistry before selecting the gcs provider.
// Explicit credential files and JSON must contain service-account credentials; other credential types are rejected. Leave both unset to use application default credentials.
package gcs
