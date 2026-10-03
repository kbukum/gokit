// Package memory provides a live, bounded in-memory broker for local development without publication history. Consumers own their subscriptions; closing a consumer or broker interrupts blocked reads. Use messaging/testutil.MockProducer and its assertion helpers for full recording in tests.
package memory
