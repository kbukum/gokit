package prompt

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/kbukum/gokit/schema"
)

// PromptIdentity identifies a registered prompt template.
type PromptIdentity struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Registry stores prompt templates by name and semver version.
type Registry struct {
	templates map[string]map[string]PromptTemplate
}

// NewRegistry creates an empty prompt registry.
func NewRegistry() *Registry { return &Registry{templates: map[string]map[string]PromptTemplate{}} }

// Register stores a template by name and version.
func (r *Registry) Register(name, version, tmpl string, outputSchema ...schema.JSON) error {
	if r == nil {
		return fmt.Errorf("prompt: registry is nil")
	}
	if strings.TrimSpace(name) == "" || strings.TrimSpace(version) == "" {
		return fmt.Errorf("prompt: name and version are required")
	}
	if err := validateSyntax(tmpl); err != nil {
		return fmt.Errorf("prompt: parse template %q@%s: %w", name, version, err)
	}
	if r.templates[name] == nil {
		r.templates[name] = map[string]PromptTemplate{}
	}
	if _, exists := r.templates[name][version]; exists {
		return fmt.Errorf("prompt: template %q version %q already registered", name, version)
	}
	pt := PromptTemplate{Name: name, Version: version, Template: tmpl}
	if len(outputSchema) > 0 {
		pt.OutputSchema = outputSchema[0]
	}
	r.templates[name][version] = pt
	return nil
}

// Lookup returns a registered prompt template by name and version.
func (r *Registry) Lookup(name, version string) (PromptTemplate, bool) {
	if r == nil || r.templates[name] == nil {
		return PromptTemplate{}, false
	}
	pt, ok := r.templates[name][version]
	return pt, ok
}

// LookupLatest returns the highest semver version for name.
func (r *Registry) LookupLatest(name string) (PromptTemplate, bool) {
	versions := r.Versions(name)
	if len(versions) == 0 {
		return PromptTemplate{}, false
	}
	return r.Lookup(name, versions[len(versions)-1])
}

// List returns all registered prompt identities in stable order.
func (r *Registry) List() []PromptIdentity {
	if r == nil {
		return nil
	}
	var out []PromptIdentity
	for name, versions := range r.templates {
		for version := range versions {
			out = append(out, PromptIdentity{Name: name, Version: version})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name == out[j].Name {
			return compareSemver(out[i].Version, out[j].Version) < 0
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// Versions returns registered versions for name in ascending semver order.
func (r *Registry) Versions(name string) []string {
	if r == nil || r.templates[name] == nil {
		return nil
	}
	versions := make([]string, 0, len(r.templates[name]))
	for version := range r.templates[name] {
		versions = append(versions, version)
	}
	sort.Slice(versions, func(i, j int) bool { return compareSemver(versions[i], versions[j]) < 0 })
	return versions
}

func compareSemver(a, b string) int {
	ap := parseSemver(a)
	bp := parseSemver(b)
	for i := range ap.core {
		if ap.core[i] < bp.core[i] {
			return -1
		}
		if ap.core[i] > bp.core[i] {
			return 1
		}
	}
	return comparePrerelease(ap.prerelease, bp.prerelease)
}

type semver struct {
	core       [3]int
	prerelease []string
}

func parseSemver(version string) semver {
	version = strings.TrimPrefix(version, "v")
	buildParts := strings.SplitN(version, "+", 2)
	version = buildParts[0]
	prereleaseParts := strings.SplitN(version, "-", 2)
	coreParts := strings.Split(prereleaseParts[0], ".")
	var out semver
	for i := 0; i < len(coreParts) && i < len(out.core); i++ {
		out.core[i], _ = strconv.Atoi(coreParts[i])
	}
	if len(prereleaseParts) == 2 && prereleaseParts[1] != "" {
		out.prerelease = strings.Split(prereleaseParts[1], ".")
	}
	return out
}

func comparePrerelease(a, b []string) int {
	switch {
	case len(a) == 0 && len(b) == 0:
		return 0
	case len(a) == 0:
		return 1
	case len(b) == 0:
		return -1
	}
	for i := 0; i < len(a) && i < len(b); i++ {
		if cmp := comparePrereleaseIdentifier(a[i], b[i]); cmp != 0 {
			return cmp
		}
	}
	switch {
	case len(a) < len(b):
		return -1
	case len(a) > len(b):
		return 1
	default:
		return 0
	}
}

func comparePrereleaseIdentifier(a, b string) int {
	ai, aerr := strconv.Atoi(a)
	bi, berr := strconv.Atoi(b)
	switch {
	case aerr == nil && berr == nil:
		switch {
		case ai < bi:
			return -1
		case ai > bi:
			return 1
		default:
			return 0
		}
	case aerr == nil:
		return -1
	case berr == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}
