package observability

import "go.opentelemetry.io/otel/attribute"

// attrKind is the discriminant of a typed attribute value.
type attrKind uint8

const (
	kindString attrKind = iota
	kindInt
	kindInt64
	kindFloat64
	kindBool
	kindStringSlice
)

// attrValue is a closed, typed union of the attribute value types this package
// supports. It replaces an untyped `any` so span and metric attributes carry no
// open interface value.
type attrValue struct {
	kind        attrKind
	str         string
	i64         int64
	f64         float64
	b           bool
	stringSlice []string
}

func stringValue(v string) attrValue   { return attrValue{kind: kindString, str: v} }
func intValue(v int) attrValue         { return attrValue{kind: kindInt, i64: int64(v)} }
func int64Value(v int64) attrValue     { return attrValue{kind: kindInt64, i64: v} }
func float64Value(v float64) attrValue { return attrValue{kind: kindFloat64, f64: v} }
func boolValue(v bool) attrValue       { return attrValue{kind: kindBool, b: v} }
func stringSliceValue(v []string) attrValue {
	return attrValue{kind: kindStringSlice, stringSlice: v}
}

// keyValue converts the typed value to an OpenTelemetry attribute.KeyValue.
func (v attrValue) keyValue(key string) attribute.KeyValue {
	switch v.kind {
	case kindInt:
		return attribute.Int(key, int(v.i64))
	case kindInt64:
		return attribute.Int64(key, v.i64)
	case kindFloat64:
		return attribute.Float64(key, v.f64)
	case kindBool:
		return attribute.Bool(key, v.b)
	case kindStringSlice:
		return attribute.StringSlice(key, v.stringSlice)
	default: // kindString
		return attribute.String(key, v.str)
	}
}
