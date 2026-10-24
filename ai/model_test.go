package ai_test

import (
	"encoding/json"
	"testing"

	"github.com/kbukum/gokit/ai"
)

func TestZeroModelIsUnknown(t *testing.T) {
	t.Parallel()
	var m ai.Model
	caps := m.Capabilities
	for name, f := range map[string]ai.Fact{
		"streaming": caps.Streaming, "vision": caps.Vision, "audio": caps.Audio,
		"tool_use": caps.ToolUse, "json_mode": caps.JSONMode, "reasoning": caps.Reasoning,
	} {
		if f.Known() || f.Supported() {
			t.Fatalf("%s: zero fact must be unknown, got %#v", name, f)
		}
	}
	if m.Family.Known() || m.Task.Known() || m.Inputs.Known() || m.Outputs.Known() || m.Parameters.Known() {
		t.Fatalf("zero descriptor must be unknown: %#v", m)
	}
	if caps.ContextLength.Known() || caps.MaxOutputTokens.Known() {
		t.Fatalf("zero limits must be unknown: %#v", caps)
	}
}

func TestFactsAndValuesCarryTheirSource(t *testing.T) {
	t.Parallel()
	yes := ai.Supported(ai.SourceRuntime)
	no := ai.Unsupported(ai.SourceProbe)
	if !yes.Known() || !yes.Supported() || yes.Source != ai.SourceRuntime {
		t.Fatalf("supported fact: %#v", yes)
	}
	if !no.Known() || no.Supported() || no.Source != ai.SourceProbe {
		t.Fatalf("unsupported fact: %#v", no)
	}
	ctx := ai.Stated(8192, ai.SourceRuntime)
	if !ctx.Known() || ctx.Value != 8192 || ctx.Source != ai.SourceRuntime {
		t.Fatalf("stated value: %#v", ctx)
	}
	if ai.Stated(0, "").Known() {
		t.Fatal("a value without a source is unknown")
	}
}

func TestFactWithoutSourceIsUnknown(t *testing.T) {
	t.Parallel()
	if ai.Supported("").Known() || ai.Supported("").Supported() {
		t.Fatal("a supported fact without a source must stay unknown")
	}
}

func TestModelJSONKeepsUnknownDistinct(t *testing.T) {
	t.Parallel()
	in := ai.Model{
		Name:     "ai/smollm2",
		Provider: ai.ProviderDMR,
		Digest:   "sha256:abc",
		Family:   ai.Stated("llama", ai.SourceRuntime),
		Task:     ai.Stated(ai.TaskTextGeneration, ai.SourceDeclared),
		Inputs:   ai.Stated([]ai.Modality{ai.ModalityText}, ai.SourceDeclared),
		Variant:  ai.Variant{Parameters: "361.82 M", Quantization: "IQ2_XXS/Q4_K_M", Format: "gguf", SizeBytes: 270_000_000},
		Capabilities: ai.Capabilities{
			Vision:        ai.Unsupported(ai.SourceProbe),
			ContextLength: ai.Stated(8192, ai.SourceRuntime),
		},
	}
	raw, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out ai.Model
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	if out.Family != in.Family || out.Task != in.Task || out.Capabilities.Vision != in.Capabilities.Vision ||
		out.Capabilities.ContextLength != in.Capabilities.ContextLength || out.Variant != in.Variant || out.Digest != in.Digest {
		t.Fatalf("round trip changed the descriptor:\n in %#v\nout %#v", in, out)
	}
	if out.Capabilities.ToolUse.Known() || out.Outputs.Known() {
		t.Fatalf("unknown fields must stay unknown after a round trip: %#v", out)
	}
	if len(out.Inputs.Value) != 1 || out.Inputs.Value[0] != ai.ModalityText {
		t.Fatalf("inputs: %#v", out.Inputs)
	}
}

func TestSupportJSONIsNamed(t *testing.T) {
	t.Parallel()
	raw, err := json.Marshal(ai.Capabilities{Vision: ai.Supported(ai.SourceRuntime)})
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]map[string]any
	if err := json.Unmarshal(raw, &generic); err != nil {
		t.Fatal(err)
	}
	if generic["vision"]["support"] != "supported" || generic["vision"]["source"] != "runtime" {
		t.Fatalf("vision JSON: %s", raw)
	}
	if _, ok := generic["tool_use"]; ok {
		t.Fatalf("unknown facts are omitted: %s", raw)
	}
	var bad ai.Support
	if err := json.Unmarshal([]byte(`"maybe"`), &bad); err == nil {
		t.Fatal("an undeclared support value must be refused")
	}
}

func TestInputBudget(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		caps ai.Capabilities
		want int
	}{
		"unknown":         {ai.Capabilities{}, 0},
		"context only":    {ai.Capabilities{ContextLength: ai.Stated(8192, ai.SourceRuntime)}, 8192},
		"output reserved": {ai.Capabilities{ContextLength: ai.Stated(8192, ai.SourceRuntime), MaxOutputTokens: ai.Stated(1024, ai.SourceDeclared)}, 7168},
		"output too big":  {ai.Capabilities{ContextLength: ai.Stated(100, ai.SourceRuntime), MaxOutputTokens: ai.Stated(100, ai.SourceDeclared)}, 0},
		"negative":        {ai.Capabilities{ContextLength: ai.Stated(-5, ai.SourceRuntime)}, 0},
	} {
		if got := tc.caps.InputBudget(); got != tc.want {
			t.Errorf("%s: InputBudget() = %d, want %d", name, got, tc.want)
		}
	}
}
