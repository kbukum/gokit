package di

// Mode identifies how a registered dependency is resolved.
type Mode int

const (
	// Eager is a pre-built value returned as-is on every resolve.
	Eager Mode = iota
	// Singleton is a factory invoked once; its result is cached.
	Singleton
	// Transient is a factory invoked fresh on every resolve.
	Transient
)

// String returns the mode name.
func (m Mode) String() string {
	switch m {
	case Eager:
		return "eager"
	case Singleton:
		return "singleton"
	case Transient:
		return "transient"
	default:
		return "unknown"
	}
}

// RegistrationInfo describes a single registration for introspection.
type RegistrationInfo struct {
	// Key is the display key: the registration name if one was given via [WithName], otherwise the concrete type name.
	Key string
	// Type is the concrete Go type name of the registration.
	Type string
	// Mode is the registration mode.
	Mode Mode
	// Initialized reports whether a resolved value is currently cached. It is always true for [Eager] registrations and always false for [Transient].
	Initialized bool
}

// Registrations returns info for every registration, for diagnostics and
// startup summaries. The order is unspecified; sort by [RegistrationInfo.Key]
// for deterministic output.
func (c *Container) Registrations() []RegistrationInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()

	out := make([]RegistrationInfo, 0, len(c.entries))
	for _, e := range c.entries {
		e.mu.Lock()
		initialized := e.mode == modeEager || e.initialized
		e.mu.Unlock()
		out = append(out, RegistrationInfo{
			Key:         e.displayKey(),
			Type:        e.typeName,
			Mode:        toMode(e.mode),
			Initialized: initialized,
		})
	}
	return out
}

func toMode(m mode) Mode {
	switch m {
	case modeEager:
		return Eager
	case modeSingleton:
		return Singleton
	default:
		return Transient
	}
}
