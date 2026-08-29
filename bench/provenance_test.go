package bench

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	benchtest "github.com/kbukum/gokit/bench/testutil"
	"github.com/kbukum/gokit/util"
)

func TestRunProvenanceDefaultRecordsSeed(t *testing.T) {
	t.Parallel()

	data, err := json.Marshal(RunProvenance{})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if got := string(data); got != `{"seed":0}` {
		t.Errorf("default provenance JSON = %s, want %s", got, `{"seed":0}`)
	}
}

func TestRunProvenanceRoundTrips(t *testing.T) {
	t.Parallel()

	want := RunProvenance{
		Seed:           42,
		RNGAlgorithm:   RNGAlgorithm,
		GitCommit:      "abc123",
		ToolVersion:    "0.3.0",
		Host:           "ci-runner",
		OS:             "linux",
		Arch:           "arm64",
		DatasetHash:    "deadbeef",
		DatasetName:    "eval",
		DatasetVersion: "2.1.0",
		Branches:       []string{"primary"},
		Metrics:        []string{"accuracy"},
	}
	data, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got RunProvenance
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Seed != want.Seed || got.GitCommit != want.GitCommit ||
		got.ToolVersion != want.ToolVersion || got.DatasetHash != want.DatasetHash ||
		len(got.Branches) != 1 || len(got.Metrics) != 1 {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, want)
	}
}

func TestWithProvenanceProbeIgnoresNilAndTypedNil(t *testing.T) {
	t.Parallel()

	for name, apply := range map[string]func(*runConfig[string]){
		"untyped nil": func(c *runConfig[string]) { WithProvenanceProbe[string](nil)(c) },
		"typed nil": func(c *runConfig[string]) {
			var typedNil *SystemProvenanceProbe
			WithProvenanceProbe[string](typedNil)(c)
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg := defaultConfig[string]()
			apply(&cfg)
			if _, ok := cfg.probe.(SystemProvenanceProbe); !ok {
				t.Fatalf("%s replaced the default probe with %T", name, cfg.probe)
			}
			// The retained default must dispatch without panicking.
			_ = cfg.probe.GitCommit()
		})
	}
}

func TestSystemProvenanceProbeGitCommitPrecedence(t *testing.T) {
	t.Parallel()

	env := map[string]string{
		"GITHUB_SHA":    "  from-github  ",
		"GIT_COMMIT":    "from-git",
		"CI_COMMIT_SHA": "from-ci",
	}
	probe := SystemProvenanceProbe{lookupEnv: func(k string) string { return env[k] }}
	if got := probe.GitCommit(); got != "from-github" {
		t.Errorf("GitCommit() = %q, want trimmed %q", got, "from-github")
	}

	delete(env, "GITHUB_SHA")
	if got := probe.GitCommit(); got != "from-git" {
		t.Errorf("GitCommit() fallback = %q, want %q", got, "from-git")
	}

	delete(env, "GIT_COMMIT")
	if got := probe.GitCommit(); got != "from-ci" {
		t.Errorf("GitCommit() fallback = %q, want %q", got, "from-ci")
	}
}

func TestSystemProvenanceProbeGitCommitUnsetReturnsEmpty(t *testing.T) {
	t.Parallel()

	probe := SystemProvenanceProbe{
		lookupEnv:   func(string) string { return "" },
		buildCommit: func() string { return "" },
	}
	if got := probe.GitCommit(); got != "" {
		t.Errorf("GitCommit() = %q, want empty", got)
	}
}

func TestSystemProvenanceProbeGitCommitFallsBackToBuildCommit(t *testing.T) {
	t.Parallel()

	probe := SystemProvenanceProbe{
		lookupEnv:   func(string) string { return "" },
		buildCommit: func() string { return "  build-sha  " },
	}
	if got := probe.GitCommit(); got != "build-sha" {
		t.Errorf("GitCommit() = %q, want trimmed build fallback %q", got, "build-sha")
	}

	env := map[string]string{"GITHUB_SHA": "env-sha"}
	envProbe := SystemProvenanceProbe{
		lookupEnv:   func(k string) string { return env[k] },
		buildCommit: func() string { return "build-sha" },
	}
	if got := envProbe.GitCommit(); got != "env-sha" {
		t.Errorf("GitCommit() = %q, want env value %q to take precedence", got, "env-sha")
	}
}

func TestSystemProvenanceProbeHostFallsBackToUnknown(t *testing.T) {
	t.Parallel()

	probe := SystemProvenanceProbe{hostname: func() (string, error) { return "  ", nil }}
	if got := probe.Host(); got != "unknown" {
		t.Errorf("Host() = %q, want %q", got, "unknown")
	}
}

func TestFixedProvenanceProbeReportsInjectedValues(t *testing.T) {
	t.Parallel()

	probe := benchtest.NewFixedProvenanceProbe(
		benchtest.WithGitCommit("feedface"),
		benchtest.WithHost("test-host"),
		benchtest.WithOS("linux"),
		benchtest.WithArch("aarch64"),
	)
	if probe.GitCommit() != "feedface" || probe.Host() != "test-host" ||
		probe.OS() != "linux" || probe.Arch() != "aarch64" {
		t.Errorf("fixed probe reported unexpected values: %+v", probe)
	}

	empty := benchtest.NewFixedProvenanceProbe(benchtest.WithHost("h"))
	if empty.GitCommit() != "" {
		t.Errorf("GitCommit() = %q, want empty when unset", empty.GitCommit())
	}
}

func sample(id, label string) Sample[string] {
	return Sample[string]{ID: id, Label: label}
}

func sampleWithInput(id, label, input string) Sample[string] {
	return Sample[string]{ID: id, Label: label, Input: []byte(input)}
}

func sampleWithSourceMetadata(id, label, source string, metadata map[string]any) Sample[string] {
	return Sample[string]{ID: id, Label: label, Source: source, Metadata: metadata}
}

func TestDatasetHashIsOrderIndependent(t *testing.T) {
	t.Parallel()

	forward := []Sample[string]{sample("a", "yes"), sample("b", "no")}
	reversed := []Sample[string]{sample("b", "no"), sample("a", "yes")}
	if datasetHash(forward) != datasetHash(reversed) {
		t.Error("dataset hash changed with sample order")
	}
}

func TestDatasetHashChangesWithContent(t *testing.T) {
	t.Parallel()

	if datasetHash([]Sample[string]{sample("a", "yes")}) ==
		datasetHash([]Sample[string]{sample("a", "no")}) {
		t.Error("dataset hash did not change when label changed")
	}
	if datasetHash([]Sample[string]{sampleWithInput("a", "yes", "first")}) ==
		datasetHash([]Sample[string]{sampleWithInput("a", "yes", "second")}) {
		t.Error("dataset hash did not change when input changed")
	}
	if datasetHash([]Sample[string]{sampleWithSourceMetadata("a", "yes", "src-A", nil)}) ==
		datasetHash([]Sample[string]{sampleWithSourceMetadata("a", "yes", "src-B", nil)}) {
		t.Error("dataset hash did not change when source changed")
	}
	if datasetHash([]Sample[string]{sampleWithSourceMetadata("a", "yes", "src", map[string]any{"k": "v1"})}) ==
		datasetHash([]Sample[string]{sampleWithSourceMetadata("a", "yes", "src", map[string]any{"k": "v2"})}) {
		t.Error("dataset hash did not change when metadata changed")
	}
}

func TestDatasetHashPointerLabelUsesValueNotAddress(t *testing.T) {
	t.Parallel()

	oneA, oneB := 1, 1
	a := []Sample[*int]{{ID: "x", Label: &oneA}}
	b := []Sample[*int]{{ID: "x", Label: &oneB}}
	if datasetHash(a) != datasetHash(b) {
		t.Error("dataset hash changed for equal pointer labels")
	}
}

func TestDatasetHashResistsDelimiterCollision(t *testing.T) {
	t.Parallel()

	if datasetHash([]Sample[string]{sample("a\tb", "c")}) ==
		datasetHash([]Sample[string]{sample("a", "b\tc")}) {
		t.Error("dataset hash aliased distinct id/label split")
	}
}

func TestSeededRandIsDeterministic(t *testing.T) {
	t.Parallel()

	seq := func(seed uint64) [4]uint64 {
		cfg := defaultConfig[string]()
		WithSeed[string](seed)(&cfg)
		r := cfg.seededRand()
		return [4]uint64{r.Uint64(), r.Uint64(), r.Uint64(), r.Uint64()}
	}
	sevenA, sevenB, eight := seq(7), seq(7), seq(8)
	if sevenA != sevenB {
		t.Error("same seed produced different sequences")
	}
	if sevenA == eight {
		t.Error("different seeds produced identical sequences")
	}
}

func benchProvenanceRunner(t *testing.T, probe ProvenanceProbe) (first, second *RunResult) {
	t.Helper()
	clock := util.NewFakeClock(time.Date(2026, 8, 23, 4, 30, 54, 0, time.UTC))
	loader := setupTestDataset(t)
	newRunner := func() *BenchRunner[string] {
		r := NewBenchRunner(
			WithClock[string](clock),
			WithTag[string]("prov"),
			WithSeed[string](99),
			WithProvenanceProbe[string](probe),
			WithMetrics(&simpleMetric{name: "accuracy"}),
		)
		r.Register("model", EvaluatorFunc("m", func(context.Context, []byte) (Prediction[string], error) {
			return Prediction[string]{Label: "positive", Score: 0.5}, nil
		}))
		return r
	}
	first, err := newRunner().Run(context.Background(), loader)
	if err != nil {
		t.Fatalf("first Run() error: %v", err)
	}
	second, err = newRunner().Run(context.Background(), loader)
	if err != nil {
		t.Fatalf("second Run() error: %v", err)
	}
	return first, second
}

func TestRunnerAttachesDeterministicProvenance(t *testing.T) {
	t.Parallel()

	probe := benchtest.NewFixedProvenanceProbe(
		benchtest.WithGitCommit("cafebabe"),
		benchtest.WithHost("ci-runner"),
		benchtest.WithOS("linux"),
		benchtest.WithArch("arm64"),
	)
	first, second := benchProvenanceRunner(t, probe)

	p := first.Provenance
	if p.Seed != 99 {
		t.Errorf("Seed = %d, want 99", p.Seed)
	}
	if p.RNGAlgorithm != RNGAlgorithm {
		t.Errorf("RNGAlgorithm = %q, want %q", p.RNGAlgorithm, RNGAlgorithm)
	}
	if p.GitCommit != "cafebabe" {
		t.Errorf("GitCommit = %q, want %q", p.GitCommit, "cafebabe")
	}
	if p.Host != "ci-runner" || p.OS != "linux" || p.Arch != "arm64" {
		t.Errorf("host/os/arch = %q/%q/%q", p.Host, p.OS, p.Arch)
	}
	if p.ToolVersion == "" {
		t.Error("ToolVersion is empty")
	}
	if p.DatasetHash == "" {
		t.Error("DatasetHash is empty")
	}
	if p.DatasetName != "test-dataset" || p.DatasetVersion != "1.0" {
		t.Errorf("dataset name/version = %q/%q", p.DatasetName, p.DatasetVersion)
	}
	if len(p.Branches) != 1 || p.Branches[0] != "model" {
		t.Errorf("Branches = %v, want [model]", p.Branches)
	}
	if len(p.Metrics) != 1 || p.Metrics[0] != "accuracy" {
		t.Errorf("Metrics = %v, want [accuracy]", p.Metrics)
	}

	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("marshal first: %v", err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatalf("marshal second: %v", err)
	}
	if !bytes.Equal(firstJSON, secondJSON) {
		t.Error("fixed clock + fixed probe + seed did not yield byte-identical JSON")
	}
}

func TestRunnerProvenanceOmitsUnresolvedGitCommit(t *testing.T) {
	t.Parallel()

	probe := benchtest.NewFixedProvenanceProbe(benchtest.WithHost("h"), benchtest.WithOS("linux"))
	first, _ := benchProvenanceRunner(t, probe)
	if first.Provenance.GitCommit != "" {
		t.Errorf("GitCommit = %q, want empty", first.Provenance.GitCommit)
	}
	data, err := json.Marshal(first.Provenance)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(data), `"git_commit"`) {
		t.Errorf("empty git_commit was serialized: %s", data)
	}
}

func TestRunnerProvenanceSurvivesStorageRoundTrip(t *testing.T) {
	t.Parallel()

	probe := benchtest.NewFixedProvenanceProbe(
		benchtest.WithGitCommit("abc123"),
		benchtest.WithHost("h"),
	)
	first, _ := benchProvenanceRunner(t, probe)

	storage := NewFileStorage(t.TempDir())
	if _, err := storage.Save(context.Background(), first); err != nil {
		t.Fatalf("Save() error: %v", err)
	}
	loaded, err := storage.Load(context.Background(), first.ID)
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if !reflect.DeepEqual(loaded.Provenance, first.Provenance) {
		t.Errorf("provenance mismatch after round-trip:\n got %+v\nwant %+v", loaded.Provenance, first.Provenance)
	}
}
