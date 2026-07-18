package skill

// Limits bounds the sizes of untrusted skill-pack files read at load time.
// Reads fail closed once a bound is exceeded. The zero value is not usable;
// obtain defaults from DefaultLimits or inject via WithLimits,
// which fills any non-positive field with its default.
type Limits struct {
	// Manifest bounds the manifest file size.
	Manifest int64
	// Body bounds the SKILL.md body size.
	Body int64
	// Asset bounds a single inert asset size.
	Asset int64
	// AssetTotal bounds the aggregate size of all inert assets.
	AssetTotal int64
}

// DefaultLimits returns the production size limits.
func DefaultLimits() Limits {
	return Limits{
		Manifest:   MaxManifestBytes,
		Body:       MaxBodyBytes,
		Asset:      MaxAssetBytes,
		AssetTotal: MaxAssetTotalBytes,
	}
}

// withDefaults returns a copy where any non-positive field is replaced by its default,
// so a partially-specified Limits stays safe.
func (l Limits) withDefaults() Limits {
	d := DefaultLimits()
	if l.Manifest <= 0 {
		l.Manifest = d.Manifest
	}
	if l.Body <= 0 {
		l.Body = d.Body
	}
	if l.Asset <= 0 {
		l.Asset = d.Asset
	}
	if l.AssetTotal <= 0 {
		l.AssetTotal = d.AssetTotal
	}
	return l
}
