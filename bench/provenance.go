package bench

import (
	"os"
	"runtime"
	"runtime/debug"
	"slices"
	"strings"

	"github.com/kbukum/gokit/version"
)

// RunProvenance captures everything needed to reproduce and audit a benchmark
// run: the deterministic seed and RNG algorithm, the source-control commit, the
// tool and host identity, and an order-dependent content hash of the evaluated
// dataset. Host and commit values are gathered through an injected
// [ProvenanceProbe], so unit tests supply fixed values with no process,
// environment, or network access.
//
// Seed always serializes: it drives reproducibility, so a run records which seed
// produced it even when that seed is zero. Genuinely-absent fields (an unresolved
// commit, an unnamed dataset) are omitted so the record stays sparse rather than
// padded with empty placeholders.
type RunProvenance struct {
	GitTreeState string `json:"git_tree_state,omitempty"`
	// Seed is the deterministic run seed (see [WithSeed]).
	Seed uint64 `json:"seed"`
	// RNGAlgorithm names the generator the seed drives (see [RNGAlgorithm]),
	// so a seed maps to the same sequence across rebuilds.
	RNGAlgorithm string `json:"rng_algorithm,omitempty"`
	// GitCommit is the source-control commit the run was built from, when resolvable.
	GitCommit string `json:"git_commit,omitempty"`
	// ToolVersion is the version of the tool that produced the run.
	ToolVersion string `json:"tool_version,omitempty"`
	// Host is the host name the run executed on.
	Host string `json:"host,omitempty"`
	// OS is the operating system the run executed on (runtime.GOOS).
	OS string `json:"os,omitempty"`
	// Arch is the CPU architecture the run executed on (runtime.GOARCH).
	Arch string `json:"arch,omitempty"`
	// DatasetHash is an order-dependent, framed content hash of the evaluated dataset.
	DatasetHash string `json:"dataset_hash,omitempty"`
	// DatasetName is the dataset name from the manifest.
	DatasetName string `json:"dataset_name,omitempty"`
	// DatasetVersion is the dataset version from the manifest.
	DatasetVersion string `json:"dataset_version,omitempty"`
	// Branches lists evaluator branch names, in registration order.
	Branches []string `json:"branches,omitempty"`
	// Metrics lists metric names computed for the run, in suite order.
	Metrics []string `json:"metrics,omitempty"`
	// Judges records the identity of every LLM-judge metric that scored the run,
	// in suite order, so a run that mixes several judge model/prompt pairs maps
	// each score set to the exact judge that produced it rather than collapsing to
	// a single identity. Empty when no judge metric ran.
	Judges []JudgeProvenance `json:"judges,omitempty"`
}

// JudgeProvenance is the recorded identity of one LLM-judge metric in a run: the
// full metric name (the comparison key), the provider and requested model, the
// provider-resolved backend model when it differed, and the versioned prompt
// identity. It is lifted from the metric's [MetricResult.Judge] so scores are
// reproducible and two runs are never silently compared across different judges.
type JudgeProvenance struct {
	// Branch identifies the evaluated branch whose records the judge scored.
	Branch string `json:"branch,omitempty"`
	// Metric is the full judge metric name, the identity two runs are joined on.
	Metric string `json:"metric"`
	// Provider is the judge provider name.
	Provider string `json:"provider,omitempty"`
	// Model is the requested judge model id.
	Model string `json:"model"`
	// ResolvedModel is the provider-resolved backend model id, present only when it
	// differs from Model (a provider resolved an alias or routed to a backend).
	ResolvedModel string `json:"resolved_model,omitempty"`
	// PromptID is the versioned judge prompt id.
	PromptID string `json:"prompt_id,omitempty"`
	// PromptVersion is the semver prompt version that produced the scores.
	PromptVersion string `json:"prompt_version,omitempty"`
	// PromptFingerprint is a content hash of the judge rubric (template body +
	// system instruction), so a rubric edited without a version bump stays visible.
	PromptFingerprint string `json:"prompt_fingerprint,omitempty"`
}

// judgeProvenance collects typed judge identities, sorted by metric name.
func judgeProvenance(results []MetricResult) []JudgeProvenance {
	var judges []JudgeProvenance
	for resultIndex := range results {
		r := &results[resultIndex]
		if r.Judge != nil {
			judges = append(judges, *r.Judge)
		}
		judges = append(judges, judgeProvenance(r.Components)...)
	}
	// Sort by metric name so the recorded provenance is deterministic regardless of suite order
	// (D2), mirroring the sibling rskit BTreeMap-keyed judges.
	slices.SortFunc(judges, func(a, b JudgeProvenance) int {
		return strings.Compare(a.Metric, b.Metric)
	})
	return judges
}

// ProvenanceProbe gathers host and source-control provenance for a benchmark run.
// It is injected into the [BenchRunner] so tests supply deterministic values with
// no process, environment, or network access.
type ProvenanceProbe interface {
	GitTreeState() string
	// GitCommit returns the source-control commit for the run, or "" when unresolvable.
	GitCommit() string
	// Host returns the host name the run executes on.
	Host() string
	// OS returns the operating system identifier (for example runtime.GOOS).
	OS() string
	// Arch returns the CPU architecture identifier (for example runtime.GOARCH).
	Arch() string
}

// gitCommitEnvVars are the environment variables inspected, in precedence order,
// for the run commit.
var gitCommitEnvVars = []string{"GITHUB_SHA", "GIT_COMMIT", "CI_COMMIT_SHA", "SOURCE_COMMIT"}

// SystemProvenanceProbe is the default probe: it reads host/os/arch from the
// standard library and the git commit best-effort from well-known CI environment
// variables, falling back to the commit embedded in the binary by the version
// package. Resolving the commit from the environment or build info rather than
// invoking git keeps bench free of a git-module dependency and performs no
// process or network I/O. A caller wanting an authoritative commit (for example
// via the git module) can resolve it and inject a fixed probe instead.
//
// The zero value is ready to use and reads the real process environment, host
// name, and build commit; the injectable lookupEnv, hostname, and buildCommit
// seams exist only so the resolution logic can be tested deterministically.
type SystemProvenanceProbe struct {
	lookupEnv   func(string) string
	hostname    func() (string, error)
	buildCommit func() string
}

// GitTreeState is unknown for environment-supplied commits; build info owns dirtiness.
func (p SystemProvenanceProbe) GitTreeState() string {
	for _, key := range gitCommitEnvVars {
		if strings.TrimSpace(p.getenv(key)) != "" {
			return ""
		}
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.modified" {
			switch setting.Value {
			case "true":
				return "dirty"
			case "false":
				return "clean"
			}
		}
	}
	return ""
}

func (p SystemProvenanceProbe) getenv(key string) string {
	if p.lookupEnv != nil {
		return p.lookupEnv(key)
	}
	return os.Getenv(key)
}

// GitCommit resolves the commit from the CI environment variables in precedence
// order, then falls back to the commit embedded in the binary by the version
// package (linker-injected or debug.ReadBuildInfo vcs.revision), returning ""
// when neither resolves.
func (p SystemProvenanceProbe) GitCommit() string {
	for _, key := range gitCommitEnvVars {
		if v := strings.TrimSpace(p.getenv(key)); v != "" {
			return v
		}
	}
	fn := p.buildCommit
	if fn == nil {
		fn = func() string { return version.GetVersionInfo().GitCommit }
	}
	return strings.TrimSpace(fn())
}

// Host returns the trimmed host name, or "unknown" when it cannot be resolved.
func (p SystemProvenanceProbe) Host() string {
	fn := p.hostname
	if fn == nil {
		fn = os.Hostname
	}
	if h, err := fn(); err == nil {
		if h = strings.TrimSpace(h); h != "" {
			return h
		}
	}
	return "unknown"
}

// OS returns runtime.GOOS.
func (p SystemProvenanceProbe) OS() string { return runtime.GOOS }

// Arch returns runtime.GOARCH.
func (p SystemProvenanceProbe) Arch() string { return runtime.GOARCH }
