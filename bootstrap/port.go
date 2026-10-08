package bootstrap

import "reflect"

// Port is a typed key for a capability one module provides and other modules need. T is the Go interface the capability exposes. A port is identified by its value, not its name: declare each port once with [NewPort] as a package variable next to its interface and reference that variable everywhere. Two NewPort calls are different ports even with the same name.
type Port[T any] struct {
	ref PortRef
	// Zero-size and unused; it makes Port types for different T distinct, so a *Port[A] cannot be converted to a *Port[B] and provide a B value under A's key.
	_ [0]T
}

// NewPort declares a port with interface type T. The name is for people: it appears in errors and the startup summary. Startup reports an empty name or a non-interface T as an invalid declaration.
func NewPort[T any](name string) *Port[T] {
	return &Port[T]{ref: PortRef{key: &portKey{name: name, typ: reflect.TypeFor[T]()}}}
}

// Name returns the port name, or "" for a nil port.
func (p *Port[T]) Name() string { return p.Ref().Name() }

// Ref returns the untyped reference used in a [ModuleSpec]. A nil port returns the zero PortRef, which startup reports as invalid.
func (p *Port[T]) Ref() PortRef {
	if p == nil {
		return PortRef{}
	}
	return p.ref
}

// PortRef identifies a port in a [ModuleSpec] without its type parameter. Refs are equal only when they come from the same [Port]. Obtain one with [Port.Ref]; the zero value is invalid.
type PortRef struct{ key *portKey }

type portKey struct {
	name string
	typ  reflect.Type
}

// Name returns the port name, or "" for the zero PortRef.
func (r PortRef) Name() string {
	if r.key == nil {
		return ""
	}
	return r.key.name
}

// String returns the port name and its interface type.
func (r PortRef) String() string {
	if r.key == nil {
		return "<invalid port>"
	}
	return r.key.name + " (" + r.key.typ.String() + ")"
}

func (r PortRef) valid() bool { return r.key != nil }
