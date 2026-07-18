package tool

// Envelope is the executable permission envelope for a tool.
// It is the single source of truth for what a tool may do at runtime;
// skills reference tools by name and never re-declare or override the envelope.
//
// All fields default-deny when absent.
// Intersection at activation (T.declared ∩ principal.grants ∩ operator.ceiling ∩ skill.references) can only narrow the envelope;
// it never widens grants.
type Envelope struct {
	// Scopes consumed via authz (e.g., "database:read"). Matched exactly against authz scope names;
	// no prefix or glob matching.
	// Empty set = no scope required (per-call authz decisions still apply).
	Scopes []string `json:"scopes,omitempty"`

	// Network is the egress policy. Nil or empty allow-list = deny all.
	Network *NetworkPolicy `json:"network,omitempty"`

	// Filesystem is the filesystem-access policy. Nil or empty list = deny all.
	Filesystem []FilesystemRule `json:"filesystem,omitempty"`

	// Subprocess is the subprocess policy. Nil or empty list = deny all.
	// Shell invocation is forbidden; argv-only.
	Subprocess []SubprocessRule `json:"subprocess,omitempty"`

	// Safety is informational at the tool level: read-only, mutating, or destructive.
	// Effective skill safety is computed as the maximum (along the order read-only < mutating < destructive) over the safety values of all referenced tools.
	Safety Safety `json:"safety,omitempty"`

	// SensitiveInvocations is a list of predicates over the validated tool input.
	// Any matching predicate triggers HITL elicitation before the call is dispatched.
	SensitiveInvocations []SensitivePredicate `json:"sensitive_invocations,omitempty"`

	// DataClassification tags result data
	// so the observability layer can apply the right redaction policy. It does not gate execution.
	DataClassification DataClassification `json:"data_classification,omitempty"`
}

// Safety is the executable-impact tier of a tool.
type Safety string

const (
	// SafetyReadOnly indicates the tool does not modify external state.
	SafetyReadOnly Safety = "read-only"
	// SafetyMutating indicates the tool performs reversible mutations.
	SafetyMutating Safety = "mutating"
	// SafetyDestructive indicates the tool performs irreversible mutations.
	SafetyDestructive Safety = "destructive"
)

// Rank returns the total-order rank used when computing effective skill safety (max over referenced tools).
// Higher is more dangerous.
func (s Safety) Rank() int {
	switch s {
	case SafetyDestructive:
		return 3
	case SafetyMutating:
		return 2
	case SafetyReadOnly:
		return 1
	default:
		return 0
	}
}

// DataClassification labels result content sensitivity.
type DataClassification string

const (
	DataPublic DataClassification = "public"
	DataPII    DataClassification = "pii"
	DataSecret DataClassification = "secret"
)

// NetworkPolicy describes the egress allow-list. An entry matches when the host matches exactly
// or matches the suffix after a leading dot (e.g., ".example.com" matches "api.example.com" but not "evil-example.com").
// IP literals match exactly. Schemes default to "https". Loopback
// and private ranges remain denied unless explicitly listed.
// Redirect targets must also satisfy the allow-list.
// DNS rebinding is mitigated by pinning resolved IPs for the duration of a request.
type NetworkPolicy struct {
	// AllowList is the list of hosts/ports/schemes the tool may call. Nil or empty = deny all egress.
	AllowList []NetworkRule `json:"allow_list,omitempty"`
}

// NetworkRule is a single allowed network destination.
type NetworkRule struct {
	// Host is an exact name (e.g., "api.example.com"), a suffix match (e.g., ".example.com"),
	// or an IP literal.
	Host string `json:"host"`
	// Port is the allowed port; 0 means default for the scheme.
	Port int `json:"port,omitempty"`
	// Scheme is "https" (default) or "http". Use of "http" requires an explicit declaration here.
	Scheme string `json:"scheme,omitempty"`
}

// FilesystemMode is the access mode granted on a filesystem path.
type FilesystemMode string

const (
	FilesystemRead   FilesystemMode = "read"
	FilesystemWrite  FilesystemMode = "write"
	FilesystemDelete FilesystemMode = "delete"
)

// FilesystemRule is a single allowed filesystem path. Paths are normalized; ".."
// segments are rejected.
// Symlinks are not followed across the declared root unless the target also satisfies a rule.
// Globs are explicit: "/foo/*" matches one segment; "/foo/**" matches recursively.
// Temp paths must be declared like any other path.
type FilesystemRule struct {
	Path string         `json:"path"`
	Mode FilesystemMode `json:"mode"`
}

// SubprocessRule is a single allowed subprocess invocation. ArgvPattern is matched as an array;
// the first element must match exactly. Later elements may be literal strings
// or "{}" placeholders for positional arguments. Shell invocation is forbidden.
// Env is empty by default; EnvAllow lists environment variable names that may be passed through.
// Cwd is normalized like a filesystem path.
type SubprocessRule struct {
	ArgvPattern []string `json:"argv_pattern"`
	EnvAllow    []string `json:"env_allow,omitempty"`
	Cwd         string   `json:"cwd,omitempty"`
}

// SensitiveMatcher is the predicate matcher kind.
type SensitiveMatcher string

const (
	MatcherExists SensitiveMatcher = "exists"
	MatcherEquals SensitiveMatcher = "equals"
	MatcherRegex  SensitiveMatcher = "regex"
	MatcherGT     SensitiveMatcher = "gt"
	MatcherLT     SensitiveMatcher = "lt"
)

// SensitivePredicate is a predicate over the validated tool input.
// Any matching predicate triggers HITL elicitation before invocation.
type SensitivePredicate struct {
	// JSONPath addresses a value within the validated input.
	JSONPath string `json:"jsonpath"`
	// Matcher is the predicate kind.
	Matcher SensitiveMatcher `json:"matcher"`
	// Value is the comparand (string for equals/regex; numeric string for gt/lt; ignored for exists).
	Value string `json:"value,omitempty"`
}
