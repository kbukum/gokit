package skill

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/kbukum/gokit/fs"
)

// MaxManifestBytes bounds the manifest file size accepted at load time.
const MaxManifestBytes = 1 << 20 // 1 MiB

const ManifestFileName = "kit.skill.yaml"

var semverPattern = regexp.MustCompile(`^v?\d+\.\d+\.\d+(?:[-+][0-9A-Za-z.-]+)?$`)

type Safety string

const (
	SafetyReadOnly    Safety = "read-only"
	SafetyMutating    Safety = "mutating"
	SafetyDestructive Safety = "destructive"
)

type Manifest struct {
	SchemaVersion         string                `yaml:"schema_version" json:"schema_version"`
	Name                  string                `yaml:"name" json:"name"`
	Version               string                `yaml:"version" json:"version"`
	Description           string                `yaml:"description" json:"description"`
	License               string                `yaml:"license,omitempty" json:"license,omitempty"`
	Authors               []string              `yaml:"authors,omitempty" json:"authors,omitempty"`
	References            References            `yaml:"references" json:"references"`
	Requires              Requires              `yaml:"requires,omitempty" json:"requires,omitempty"`
	HumanApproval         []HumanApproval       `yaml:"human_approval" json:"human_approval"`
	Budgets               Budgets               `yaml:"budgets,omitempty" json:"budgets,omitempty"`
	ModelHints            ModelHints            `yaml:"model_hints,omitempty" json:"model_hints,omitempty"`
	ProgressiveDisclosure ProgressiveDisclosure `yaml:"progressive_disclosure,omitempty" json:"progressive_disclosure,omitempty"`
	Scripts               []Script              `yaml:"scripts,omitempty" json:"scripts,omitempty"`
	Signature             *Signature            `yaml:"signature,omitempty" json:"signature,omitempty"`
	Safety                Safety                `yaml:"safety,omitempty" json:"safety,omitempty"`
}

type References struct {
	Tools      []string          `yaml:"tools" json:"tools"`
	Prompts    []PromptReference `yaml:"prompts" json:"prompts"`
	Resources  []string          `yaml:"resources" json:"resources"`
	MCPServers []string          `yaml:"mcp_servers" json:"mcp_servers"`
}

type PromptReference struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}

type Requires struct {
	Scopes       []string `yaml:"scopes,omitempty" json:"scopes,omitempty"`
	Capabilities []string `yaml:"capabilities,omitempty" json:"capabilities,omitempty"`
}

type HumanApproval struct {
	Step      string `yaml:"step" json:"step"`
	When      string `yaml:"when" json:"when"`
	Rationale string `yaml:"rationale" json:"rationale"`
}

type Budgets struct {
	MaxTokens int     `yaml:"max_tokens,omitempty" json:"max_tokens,omitempty"`
	MaxCalls  int     `yaml:"max_calls,omitempty" json:"max_calls,omitempty"`
	MaxCost   MaxCost `yaml:"max_cost,omitempty" json:"max_cost,omitempty"`
	WallClock string  `yaml:"wall_clock,omitempty" json:"wall_clock,omitempty"`
}

type MaxCost struct {
	Amount   float64 `yaml:"amount" json:"amount"`
	Currency string  `yaml:"currency" json:"currency"`
}

type ModelHints struct {
	Preferred []string `yaml:"preferred,omitempty" json:"preferred,omitempty"`
	Reject    []string `yaml:"reject,omitempty" json:"reject,omitempty"`
}

type ProgressiveDisclosure struct {
	Summary string `yaml:"summary,omitempty" json:"summary,omitempty"`
	Detail  string `yaml:"detail,omitempty" json:"detail,omitempty"`
}

type Script struct {
	Path        string `yaml:"path" json:"path"`
	Description string `yaml:"description" json:"description"`
}

type Signature struct {
	Algorithm string `yaml:"algorithm" json:"algorithm"`
	Value     string `yaml:"value" json:"value"`
	KeyID     string `yaml:"key_id" json:"key_id"`
}

// IsPresent reports whether the signature carries the fields required to identify a signature scheme
// and payload. An empty
// or placeholder object (e.g. `signature: {}` or a blank value) is not present.
func (s *Signature) IsPresent() bool {
	return s != nil && s.Algorithm != "" && s.Value != ""
}

// LoadManifest reads and parses a manifest file, enforcing MaxManifestBytes
// and rejecting symlinks before reading.
func LoadManifest(path string) (*Manifest, error) {
	return loadManifest(path, MaxManifestBytes)
}

func loadManifest(path string, maxBytes int64) (*Manifest, error) {
	data, err := readBounded(path, maxBytes)
	if err != nil {
		return nil, err
	}
	m, err := ParseManifest(data)
	if err != nil {
		return nil, fmt.Errorf("skill: %s: %w", path, err)
	}
	return m, nil
}

// ParseManifest decodes and validates a manifest from raw YAML bytes. Unknown fields are rejected
// and the result is validated, so it fails closed on malformed or untrusted input.
func ParseManifest(data []byte) (*Manifest, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("%w: %w", ErrParseManifest, err)
	}
	if err := Validate(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// Validate checks a manifest's required fields and structural invariants.
// Every failure wraps ErrManifestInvalid.
func Validate(m *Manifest) error {
	if err := validateManifest(m); err != nil {
		return fmt.Errorf("%w: %w", ErrManifestInvalid, err)
	}
	return nil
}

func validateManifest(m *Manifest) error {
	if m == nil {
		return fmt.Errorf("manifest is nil")
	}
	if strings.TrimSpace(m.SchemaVersion) == "" {
		return fmt.Errorf("schema_version is required")
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(m.Version) == "" || !semverPattern.MatchString(m.Version) {
		return fmt.Errorf("version must be semver")
	}
	if strings.TrimSpace(m.Description) == "" {
		return fmt.Errorf("description is required")
	}
	if err := validateUnique("references.tools", m.References.Tools); err != nil {
		return err
	}
	if err := validateUnique("references.resources", m.References.Resources); err != nil {
		return err
	}
	if err := validateUnique("references.mcp_servers", m.References.MCPServers); err != nil {
		return err
	}
	if err := validateUnique("requires.scopes", m.Requires.Scopes); err != nil {
		return err
	}
	if err := validateUnique("requires.capabilities", m.Requires.Capabilities); err != nil {
		return err
	}
	seenPrompts := map[string]struct{}{}
	for _, prompt := range m.References.Prompts {
		if strings.TrimSpace(prompt.Name) == "" || strings.TrimSpace(prompt.Version) == "" {
			return fmt.Errorf("references.prompts name and version are required")
		}
		key := prompt.Name + "@" + prompt.Version
		if _, ok := seenPrompts[key]; ok {
			return fmt.Errorf("duplicate references.prompts value %q", key)
		}
		seenPrompts[key] = struct{}{}
	}
	for _, approval := range m.HumanApproval {
		if strings.TrimSpace(approval.Step) == "" || strings.TrimSpace(approval.When) == "" || strings.TrimSpace(approval.Rationale) == "" {
			return fmt.Errorf("human_approval step, when, and rationale are required")
		}
	}
	for _, script := range m.Scripts {
		if strings.TrimSpace(script.Path) == "" || strings.TrimSpace(script.Description) == "" {
			return fmt.Errorf("scripts path and description are required")
		}
		if err := fs.ValidateRelativePath(script.Path); err != nil {
			return fmt.Errorf("scripts path %q escapes the skill pack: %w", script.Path, err)
		}
	}
	switch m.Safety {
	case "", SafetyReadOnly, SafetyMutating, SafetyDestructive:
	default:
		return fmt.Errorf("invalid safety %q", m.Safety)
	}
	return nil
}

func validateUnique(field string, values []string) error {
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			return fmt.Errorf("%s contains empty value", field)
		}
		if _, ok := seen[value]; ok {
			return fmt.Errorf("duplicate %s value %q", field, value)
		}
		seen[value] = struct{}{}
	}
	return nil
}
