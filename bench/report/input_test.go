package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/kbukum/gokit/bench"
)

func fixtureInput(t *testing.T, result *bench.RunResult) Input {
	t.Helper()
	snapshot := *result
	objects := bench.NewDirStore(t.TempDir())
	var data bytes.Buffer
	for i := 0; i < result.Dataset.SampleCount; i++ {
		record := bench.Record{ID: fmt.Sprintf("s%d", i+1), SampleIndex: i, Label: json.RawMessage(`"positive"`), Predicted: json.RawMessage(`"positive"`), Correct: i%3 != 2, Score: .9, Usage: bench.TokenUsage{Source: bench.UsageUnavailable}}
		if i == 4 {
			record.Error = "low confidence"
		}
		if err := json.NewEncoder(&data).Encode(record); err != nil {
			t.Fatal(err)
		}
	}
	if data.Len() != 0 {
		snapshot.RecordSegments = []int{1}
		snapshot.RecordReadLimit = 1 << 20
		if err := objects.Put(t.Context(), "records/"+snapshot.ID+"/0/00000000.jsonl", data.Bytes()); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := json.Marshal(&snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if err := objects.Put(t.Context(), "runs/"+snapshot.ID+".json", raw); err != nil {
		t.Fatal(err)
	}
	return Input{Result: &snapshot, Store: bench.NewResultStore(objects)}
}

func testBounds(targets map[string]float64) []bench.Bound {
	bounds := make([]bench.Bound, 0, len(targets))
	for metric, limit := range targets {
		bounds = append(bounds, bench.Bound{Metric: metric, Kind: bench.Min, Limit: limit})
	}

	return bounds
}

func testVegaLiteSpecs(t *testing.T, result *bench.RunResult) map[string]any {
	t.Helper()
	raw, err := VegaLiteSpecs(result)
	if err != nil {
		t.Fatal(err)
	}
	specs := make(map[string]any)
	for name, data := range raw {
		var spec map[string]any
		if err := json.Unmarshal(data, &spec); err != nil {
			t.Fatal(err)
		}
		specs[name] = spec
	}
	return specs
}
