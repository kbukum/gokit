// Package echo provides a trivial inference provider that echoes its input.
//
// [Register] installs the [Echo] provider through a [Factory] so inference
// wiring and tests have a dependency-free backend that exercises the provider
// contract without a real model server.
package echo
