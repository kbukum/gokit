package util

import "reflect"

// IsNil reports whether v is nil or a typed-nil wrapped in an interface.
//
// A plain nil interface compares equal to nil, but an interface holding a nil
// pointer, map, slice, channel, or function value does not — so a guard like
// `dep == nil` silently accepts an absent dependency and panics on first use.
// IsNil closes that gap for injected interface seams. The parameter is a
// documented opaque-value exception: the nil-ness of an arbitrary dynamic type
// cannot be expressed through generics.
func IsNil(v any) bool {
	if v == nil {
		return true
	}
	switch rv := reflect.ValueOf(v); rv.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice, reflect.Chan, reflect.Func:
		return rv.IsNil()
	default:
		return false
	}
}
