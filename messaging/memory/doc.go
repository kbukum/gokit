// Package memory provides an in-memory message broker for tests and local development.
//
// [NewBroker] returns an [InMemoryBroker] implementing the messaging contract without external infrastructure, and the Assert* / WaitFor* helpers make publish/consume behavior straightforward to assert in tests.
package memory
