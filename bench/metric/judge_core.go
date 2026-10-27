package metric

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/kbukum/gokit/ai"
	"github.com/kbukum/gokit/ai/chat"
	"github.com/kbukum/gokit/bench"
	apperrors "github.com/kbukum/gokit/errors"
	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/resilience"
)

const (
	defaultJudgeTimeout        = 30 * time.Second
	defaultJudgeMaxTokens      = 256
	defaultJudgeMaxPromptBytes = 128 * 1024
	// maxVerdictReplyBytes bounds the raw reply a verdict keeps for provenance.
	maxVerdictReplyBytes = 4 << 10
)

// JudgeInput is what the judge sees of one task: the input parts (text and media) and the expected answer. The expected answer is sent only when the rubric uses it.
type JudgeInput struct {
	Input     []ai.ContentPart
	Reference string
}

// VerdictKind tells a judged item from one the judge gave no usable verdict for.
type VerdictKind string

const (
	VerdictJudged VerdictKind = "judged"
	VerdictNone   VerdictKind = "no_verdict"
)

// Preference is a pairwise outcome, always in the caller's A/B order.
type Preference string

const (
	PreferA   Preference = "A"
	PreferB   Preference = "B"
	PreferTie Preference = "tie"
)

// Verdict is one judge call's result. A judged score has Points on the rubric's scale and Score normalized to [0, 1]; a judged pair has Winner. A reply that is malformed, out of range, truncated or too long is [VerdictNone] with a Reason; it is never a score. Reply keeps at most 4 KiB of the raw reply for provenance.
type Verdict struct {
	Kind          VerdictKind
	Points        int
	Score         float64
	Winner        Preference
	Rationale     string
	Reason        string
	Reply         string
	Model         string
	Usage         ai.Usage
	UsageReported bool
}

// PairVerdict combines the two orders of one pairwise judgment. Winner is the agreed preference, a tie when the orders disagree, and empty when either order has no verdict (Kind is then [VerdictNone]).
type PairVerdict struct {
	Kind           VerdictKind
	Winner         Preference
	OrderDisagreed bool
	Forward        Verdict
	Swapped        Verdict
}

// Judge asks an injected [llm.Provider] to judge model outputs by a [Rubric]. It scores one output on its own ([Judge.Score]) or compares two in both orders ([Judge.Compare]). Model names never reach the judge: it sees only the task input, the expected answer when the rubric uses it, and the outputs.
//
// The reply is untrusted. It must be one JSON object with exactly the schema's fields; anything else is a verdict of [VerdictNone], never a pass. A provider failure, timeout or cancellation is an error, because the call did not produce a reply to judge.
type Judge struct {
	provider       llm.Provider
	model          string
	rubric         Rubric
	maxTokens      int
	maxPromptBytes int
	policy         *resilience.Policy
}

// NewJudge builds a judge. It refuses a nil provider, a blank model, an invalid rubric, a policy without a positive timeout and a policy with retries, since a judge call is not idempotent.
func NewJudge(provider llm.Provider, model string, rubric Rubric, opts ...JudgeOption) (*Judge, error) {
	if isNilLLMProvider(provider) {
		return nil, apperrors.InvalidInput("judge", "judge requires a non-nil llm.Provider")
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, apperrors.InvalidInput("judge", "judge model must not be empty")
	}
	if err := rubric.Validate(); err != nil {
		return nil, err
	}
	j := &Judge{
		provider: provider, model: model, rubric: rubric,
		maxTokens: defaultJudgeMaxTokens, maxPromptBytes: defaultJudgeMaxPromptBytes,
		policy: resilience.NewPolicy().WithTimeout(defaultJudgeTimeout),
	}
	for _, o := range opts {
		o(j)
	}
	if j.policy == nil || j.policy.Timeout <= 0 {
		return nil, apperrors.InvalidInput("judge", "judge policy must carry a positive timeout")
	}
	if j.policy.Retry != nil {
		return nil, apperrors.InvalidInput("judge", "judge policy must not retry; a judge call is not idempotent")
	}
	return j, nil
}

// Rubric returns the judge's rubric.
func (j *Judge) Rubric() Rubric { return j.rubric }

// Provenance names the judge: provider, requested model, prompt and rubric fingerprint. The prompt id is the score prompt; a pairwise result names [JudgePairwisePromptID] itself.
func (j *Judge) Provenance(promptID string) bench.JudgeProvenance {
	return bench.JudgeProvenance{
		Provider: j.provider.Name(), Model: j.model, PromptID: promptID, PromptVersion: JudgePromptVersion,
		PromptFingerprint: j.rubric.Fingerprint(),
	}
}

// Score judges one output against the rubric.
func (j *Judge) Score(ctx context.Context, in JudgeInput, output string) (Verdict, error) {
	parts, err := j.content(in, []namedOutput{{"response", output}})
	if err != nil {
		return Verdict{}, err
	}
	return j.call(ctx, j.rubric.scoreSystem(), parts, func(reply string, v *Verdict) string {
		points, rationale, reason := parseScoreReply(reply, j.rubric.Points)
		v.Points, v.Score, v.Rationale = points, j.rubric.normalize(points), rationale
		return reason
	})
}

// Compare judges a against b in both orders. Agreeing orders give their preference; disagreeing orders give a tie with OrderDisagreed; a missing verdict in either order gives no verdict.
func (j *Judge) Compare(ctx context.Context, in JudgeInput, a, b string) (PairVerdict, error) {
	forward, err := j.prefer(ctx, in, a, b)
	if err != nil {
		return PairVerdict{}, err
	}
	swapped, err := j.prefer(ctx, in, b, a)
	if err != nil {
		return PairVerdict{}, err
	}
	swapped.Winner = flip(swapped.Winner)
	out := PairVerdict{Forward: forward, Swapped: swapped}
	switch {
	case forward.Kind == VerdictNone || swapped.Kind == VerdictNone:
		out.Kind = VerdictNone
	case forward.Winner == swapped.Winner:
		out.Kind, out.Winner = VerdictJudged, forward.Winner
	default:
		out.Kind, out.Winner, out.OrderDisagreed = VerdictJudged, PreferTie, true
	}
	return out, nil
}

func (j *Judge) prefer(ctx context.Context, in JudgeInput, first, second string) (Verdict, error) {
	parts, err := j.content(in, []namedOutput{{"response_a", first}, {"response_b", second}})
	if err != nil {
		return Verdict{}, err
	}
	return j.call(ctx, j.rubric.pairwiseSystem(), parts, func(reply string, v *Verdict) string {
		winner, rationale, reason := parsePairReply(reply)
		v.Winner, v.Rationale = winner, rationale
		return reason
	})
}

type namedOutput struct {
	tag  string
	text string
}

// content frames the input, the reference and the outputs in tags. Text that would close a tag is escaped so an output cannot step outside its frame.
func (j *Judge) content(in JudgeInput, outputs []namedOutput) ([]ai.ContentPart, error) {
	if j.rubric.UseReference && strings.TrimSpace(in.Reference) == "" {
		return nil, apperrors.InvalidInput("judge", "the rubric uses the expected answer, but the item has none")
	}
	parts := []ai.ContentPart{ai.Text{Text: "<input>\n"}}
	size := 0
	for _, p := range in.Input {
		if t, ok := p.(ai.Text); ok {
			p = ai.Text{Text: escapeFrame(t.Text)}
		}
		parts = append(parts, p)
		size += partBytes(p)
	}
	tail := "\n</input>\n"
	if j.rubric.UseReference {
		tail += "<reference>\n" + escapeFrame(in.Reference) + "\n</reference>\n"
	}
	for _, o := range outputs {
		tail += "<" + o.tag + ">\n" + escapeFrame(o.text) + "\n</" + o.tag + ">\n"
	}
	size += len(tail)
	if size > j.maxPromptBytes {
		return nil, apperrors.InvalidInput("judge", fmt.Sprintf("judge prompt of %d bytes exceeds the %d-byte bound", size, j.maxPromptBytes))
	}
	return append(parts, ai.Text{Text: tail}), nil
}

// call asks the judge and reads its whole reply with read, which fills v and returns a nonempty reason when the reply
// is not a verdict. Only the provenance copy in Reply is truncated.
func (j *Judge) call(ctx context.Context, system string, parts []ai.ContentPart, read func(reply string, v *Verdict) string) (Verdict, error) {
	temperature := 0.0
	req := llm.CompletionRequest{
		Model: j.model, SystemPrompt: system, Messages: []chat.Message{chat.UserMessage{Content: parts}},
		Temperature: &temperature, MaxTokens: j.maxTokens,
	}
	resp, err := resilience.Execute(ctx, j.policy, func(callCtx context.Context) (llm.CompletionResponse, error) {
		return j.provider.Execute(callCtx, req)
	})
	if err != nil {
		return Verdict{}, judgeProviderError(err)
	}
	reply := resp.Text()
	v := Verdict{
		Kind: VerdictJudged, Reply: truncate(reply, maxVerdictReplyBytes), Model: strings.TrimSpace(resp.Model),
		Usage: resp.Usage, UsageReported: resp.UsageReported,
	}
	switch {
	case resp.StopReason != "" && resp.StopReason != chat.FinishReasonStop:
		return none(v, fmt.Sprintf("the reply did not finish normally (%s)", resp.StopReason)), nil
	case len(reply) > maxJudgeReplyBytes:
		return none(v, fmt.Sprintf("the reply is longer than %d bytes", maxJudgeReplyBytes)), nil
	}
	if reason := read(reply, &v); reason != "" {
		return none(v, reason), nil
	}
	return v, nil
}

func none(v Verdict, reason string) Verdict {
	v.Kind, v.Points, v.Score, v.Winner, v.Rationale, v.Reason = VerdictNone, 0, 0, "", "", reason
	return v
}

func flip(p Preference) Preference {
	switch p {
	case PreferA:
		return PreferB
	case PreferB:
		return PreferA
	default:
		return p
	}
}

var frameEscaper = strings.NewReplacer("</", "<\\/")

func escapeFrame(s string) string { return frameEscaper.Replace(s) }

func partBytes(p ai.ContentPart) int {
	switch v := p.(type) {
	case ai.Text:
		return len(v.Text)
	case ai.Image:
		return len(v.Data) + len(v.Source)
	case ai.Audio:
		return len(v.Data) + len(v.Source)
	case ai.Video:
		return len(v.Data) + len(v.Source)
	case ai.File:
		return len(v.Data) + len(v.Source)
	default:
		return 0
	}
}

// truncate cuts s to at most n bytes on a rune boundary.
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !isRuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }

// isNilLLMProvider reports whether provider is nil, including a typed nil.
func isNilLLMProvider(provider llm.Provider) bool {
	if provider == nil {
		return true
	}
	v := reflect.ValueOf(provider)
	switch v.Kind() {
	case reflect.Pointer, reflect.Func, reflect.Map, reflect.Slice, reflect.Chan, reflect.Interface:
		return v.IsNil()
	default:
		return false
	}
}
