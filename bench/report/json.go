package report

import (
	"context"
	"encoding/json"
	"io"

	"github.com/kbukum/gokit/bench"
)

func JSON() Reporter { return &jsonReporter{} }

type jsonReporter struct{}

func (r *jsonReporter) Name() string { return "json" }
func (r *jsonReporter) Generate(ctx context.Context, w io.Writer, input Input) error {
	if err := validateInput(ctx, input); err != nil {
		return err
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if input.Diff == nil {
		return encoder.Encode(input.Result)
	}
	return encoder.Encode(struct {
		Result      *bench.RunResult  `json:"result"`
		Eligibility bench.Eligibility `json:"eligibility"`
		Verdict     string            `json:"verdict"`
	}{Result: input.Result, Eligibility: input.Diff.Eligibility, Verdict: input.Diff.Verdict()})
}
