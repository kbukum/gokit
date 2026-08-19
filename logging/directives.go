package logging

import (
	"sort"
	"strings"
)

// BuildDirectives renders a base level and a set of per-module overrides into a single
// comma-separated directive string, e.g. "info,sqlx=warn,hyper=error". The base level
// comes first and module directives follow in sorted order, so the output is
// deterministic and stable across runs. This mirrors the RUST_LOG / env-filter grammar
// used by the sibling kits, giving gokit a portable interchange form for per-module log
// configuration. When there are no module overrides the base level is returned alone.
func BuildDirectives(baseLevel string, moduleLevels map[string]string) string {
	baseLevel = strings.TrimSpace(baseLevel)
	if len(moduleLevels) == 0 {
		return baseLevel
	}
	modules := make([]string, 0, len(moduleLevels))
	for module := range moduleLevels {
		modules = append(modules, module)
	}
	sort.Strings(modules)

	parts := make([]string, 0, len(moduleLevels)+1)
	if baseLevel != "" {
		parts = append(parts, baseLevel)
	}
	for _, module := range modules {
		level := strings.TrimSpace(moduleLevels[module])
		if module == "" || level == "" {
			continue
		}
		parts = append(parts, module+"="+level)
	}
	return strings.Join(parts, ",")
}

// ParseDirectives is the inverse of BuildDirectives. It splits a directive string into
// the base level and the per-module overrides, so a value carried in an environment
// variable such as LOG_LEVEL="info,sqlx=warn" can be turned back into structured
// configuration. A bare "module=level" token with no leading base level yields an empty
// base level and that override; empty tokens are ignored.
func ParseDirectives(directives string) (baseLevel string, moduleLevels map[string]string) {
	moduleLevels = make(map[string]string)
	for _, token := range strings.Split(directives, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		module, level, isOverride := strings.Cut(token, "=")
		module = strings.TrimSpace(module)
		if !isOverride {
			if baseLevel == "" {
				baseLevel = module
			}
			continue
		}
		level = strings.TrimSpace(level)
		if module == "" || level == "" {
			continue
		}
		moduleLevels[module] = level
	}
	return baseLevel, moduleLevels
}
