package media

import (
	"cmp"
	"reflect"
	"slices"
)

// Registry is an application-owned knowledge base of known [Format]s plus a set
// of injected [Prober] backends. It is the light-kit parallel of rskit's media
// registry: constructed explicitly with functional options, never mutated
// through package globals and never populated by init side effects.
//
// The zero Registry is safe to read but empty: its methods return no formats and
// no probers. Use [NewRegistry] to obtain a Registry seeded with the built-in
// format catalog; add [WithImageProber] (or a custom [WithProber]) to enable
// metadata enrichment.
type Registry struct {
	formats map[Format]FormatInfo
	probers []Prober
}

// Option configures a [Registry] at construction time.
type Option func(*Registry)

// WithFormat adds or overrides a [FormatInfo] catalog entry. It lazily
// initializes the catalog so the option is safe to apply to a zero Registry.
func WithFormat(fi FormatInfo) Option {
	return func(r *Registry) {
		if r.formats == nil {
			r.formats = make(map[Format]FormatInfo)
		}
		r.formats[fi.Format] = fi
	}
}

// WithProber appends a [Prober] backend. Probers are tried in registration
// order; the first that succeeds wins. A nil prober (including a typed-nil
// interface) is ignored so it cannot panic later during probing.
func WithProber(p Prober) Option {
	return func(r *Registry) {
		if !isNilProber(p) {
			r.probers = append(r.probers, p)
		}
	}
}

// isNilProber reports whether p is nil, including a typed-nil interface value
// (e.g. a nil *T stored in the interface) that a plain p == nil check misses.
func isNilProber(p Prober) bool {
	if p == nil {
		return true
	}
	v := reflect.ValueOf(p)
	switch v.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}

// WithImageProber appends the built-in stdlib image prober, which enriches
// detection with pixel dimensions for JPEG, PNG, and GIF content.
func WithImageProber() Option {
	return WithProber(imageProber{})
}

// NewRegistry builds a Registry seeded with the built-in [knownFormats] catalog,
// then applies opts. Pass [WithImageProber] (or a custom [WithProber]) to enable
// metadata enrichment; with no prober options the registry performs
// signature-only detection.
func NewRegistry(opts ...Option) *Registry {
	r := &Registry{formats: make(map[Format]FormatInfo)}
	for _, fi := range knownFormats() {
		r.formats[fi.Format] = fi
	}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Lookup returns the catalog entry for f, if known.
func (r *Registry) Lookup(f Format) (FormatInfo, bool) {
	fi, ok := r.formats[f]
	return fi, ok
}

// Formats returns the catalog entries sorted by format identifier.
func (r *Registry) Formats() []FormatInfo {
	out := make([]FormatInfo, 0, len(r.formats))
	for _, fi := range r.formats {
		out = append(out, fi)
	}
	slices.SortFunc(out, func(a, b FormatInfo) int { return cmp.Compare(a.Format, b.Format) })
	return out
}

// Probe classifies content and enriches it with metadata from the registered
// probers. Signature detection is authoritative for the classification; the
// first prober that recognizes the content contributes its extra fields (e.g.
// image dimensions). Probers that do not recognize the content (or fail to
// decode it) are skipped, so the result degrades gracefully to detection
// [Info] only.
func (r *Registry) Probe(data []byte) Metadata {
	meta := Metadata{Info: Detect(data)}
	for _, p := range r.probers {
		m, err := p.Probe(data)
		if err != nil {
			continue
		}
		meta.Resolution = m.Resolution
		break
	}
	return meta
}

// SupportedFormats reports the format identifiers in the catalog, sorted.
func (r *Registry) SupportedFormats() []Format {
	out := make([]Format, 0, len(r.formats))
	for f := range r.formats {
		out = append(out, f)
	}
	slices.Sort(out)
	return out
}
