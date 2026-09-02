package bench

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"slices"
	"strings"

	"github.com/kbukum/gokit/util"
	"github.com/kbukum/gokit/version"
)

// RunProvenance captures everything needed to reproduce and audit a benchmark
// run: the deterministic seed and RNG algorithm, the source-control commit, the
// tool and host identity, and an order-independent content hash of the evaluated
// dataset. Host and commit values are gathered through an injected
// [ProvenanceProbe], so unit tests supply fixed values with no process,
// environment, or network access.
//
// Seed always serializes: it drives reproducibility, so a run records which seed
// produced it even when that seed is zero. Genuinely-absent fields (an unresolved
// commit, an unnamed dataset) are omitted so the record stays sparse rather than
// padded with empty placeholders.
type RunProvenance struct {
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
	// DatasetHash is an order-independent content hash of the evaluated dataset.
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
// identity. It is lifted from the metric's [MetricResult.Detail] so scores are
// reproducible and two runs are never silently compared across different judges.
type JudgeProvenance struct {
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

// Detail keys a judge metric records in its [MetricResult.Detail], read back by
// the runner to lift judge identity into [RunProvenance]. They live in bench (the
// lower package) so the metric package can reference them without a back-edge
// while the runner reads them without importing metric.
const (
	// DetailJudgeModel is the [MetricResult.Detail] key holding the judge model id.
	DetailJudgeModel = "judge_model"
	// DetailJudgeProvider is the [MetricResult.Detail] key holding the judge provider name.
	DetailJudgeProvider = "judge_provider"
	// DetailJudgeResolvedModel is the [MetricResult.Detail] key holding the
	// provider-resolved backend model id, present only when it differs from the
	// requested model.
	DetailJudgeResolvedModel = "judge_resolved_model"
	// DetailJudgePromptID is the [MetricResult.Detail] key holding the judge prompt id.
	DetailJudgePromptID = "judge_prompt_id"
	// DetailJudgePromptVersion is the [MetricResult.Detail] key holding the judge prompt version.
	DetailJudgePromptVersion = "judge_prompt_version"
	// DetailJudgePromptFingerprint is the [MetricResult.Detail] key holding the judge rubric fingerprint.
	DetailJudgePromptFingerprint = "judge_prompt_fingerprint"
)

// judgeProvenance collects the identity of every judge metric in results, in
// result order, so a run that mixes several judge model/prompt pairs preserves
// each one rather than dropping all but the first. A judge metric is identified
// by the [DetailJudgeModel] and [DetailJudgePromptVersion] keys it writes into
// [MetricResult.Detail]; the metric name is the comparison key. Returns nil when
// no judge metric ran.
func judgeProvenance(results []MetricResult) []JudgeProvenance {
	var judges []JudgeProvenance
	for _, r := range results {
		detail, ok := r.Detail.(map[string]any)
		if !ok {
			continue
		}
		model, mOK := detail[DetailJudgeModel].(string)
		promptVersion, vOK := detail[DetailJudgePromptVersion].(string)
		if !mOK || !vOK || model == "" {
			continue
		}
		provider, _ := detail[DetailJudgeProvider].(string)
		resolved, _ := detail[DetailJudgeResolvedModel].(string)
		promptID, _ := detail[DetailJudgePromptID].(string)
		fingerprint, _ := detail[DetailJudgePromptFingerprint].(string)
		judges = append(judges, JudgeProvenance{
			Metric:            r.Name,
			Provider:          provider,
			Model:             model,
			ResolvedModel:     resolved,
			PromptID:          promptID,
			PromptVersion:     promptVersion,
			PromptFingerprint: fingerprint,
		})
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

// datasetHash computes an order-independent content hash of a dataset from each
// sample's id, raw input bytes, label, source, and metadata, so the same dataset
// hashes identically regardless of load order while changing any metric-visible
// sample field changes the hash. Labels are rendered type-qualified and normalized
// for stable cross-process hashing (for example pointer labels hash by pointed
// value instead of process-specific address). Metadata is normalized with JSON
// canonicalization where possible. Each field is folded with length-prefixed
// framing via [util.ContentHasher.UpdateFramed], so delimiter-like payloads
// cannot collide, and no large intermediate buffer is materialized regardless of
// dataset size.
func datasetHash[L comparable](samples []Sample[L]) string {
	type record struct {
		id       string
		input    []byte
		label    string
		source   string
		metadata string
	}
	records := make([]record, len(samples))
	for i, s := range samples {
		records[i] = record{
			id:       s.ID,
			input:    s.Input,
			label:    stableLabelHash(s.Label),
			source:   s.Source,
			metadata: stableAnyHash(s.Metadata),
		}
	}
	slices.SortFunc(records, func(a, b record) int {
		if c := strings.Compare(a.id, b.id); c != 0 {
			return c
		}
		if c := bytes.Compare(a.input, b.input); c != 0 {
			return c
		}
		if c := strings.Compare(a.label, b.label); c != 0 {
			return c
		}
		if c := strings.Compare(a.source, b.source); c != 0 {
			return c
		}
		return strings.Compare(a.metadata, b.metadata)
	})
	h := util.NewContentHasher()
	for _, r := range records {
		h.UpdateFramed([]byte("id"), []byte(r.id))
		h.UpdateFramed([]byte("input"), r.input)
		h.UpdateFramed([]byte("label"), []byte(r.label))
		h.UpdateFramed([]byte("source"), []byte(r.source))
		h.UpdateFramed([]byte("metadata"), []byte(r.metadata))
	}
	return h.FinalizeHex()
}

func stableLabelHash[L comparable](label L) string {
	rv := reflect.ValueOf(label)
	if rv.IsValid() && rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			return fmt.Sprintf("%T\x1f<nil>", label)
		}
		return fmt.Sprintf("%T\x1f%s", label, stableAnyHash(rv.Elem().Interface()))
	}
	return fmt.Sprintf("%T\x1f%s", label, stableAnyHash(label))
}

func stableAnyHash(value any) string {
	if value == nil {
		return "<nil>"
	}
	if raw, err := json.Marshal(value); err == nil {
		return string(raw)
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return fmt.Sprintf("%T\x1f<runtime-identity>", value)
	case reflect.Pointer:
		if rv.IsNil() {
			return fmt.Sprintf("%T\x1f<nil>", value)
		}
		return fmt.Sprintf("%T\x1f%s", value, stableAnyHash(rv.Elem().Interface()))
	default:
		return fmt.Sprintf("%T\x1f%v", value, value)
	}
}
