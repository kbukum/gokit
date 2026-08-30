package metric

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"

	"github.com/kbukum/gokit/ai/chat"
	apperrors "github.com/kbukum/gokit/errors"
)

// maxJudgeReplyBytes hard-caps the byte length of a judge reply accepted for
// parsing. max_tokens is only a request hint the provider may ignore, so this is
// the local resource boundary on untrusted model output: a longer reply is
// rejected before it is parsed rather than deserialized in full.
const maxJudgeReplyBytes = 64 * 1024

// JudgeVerdict is a judge's structured verdict for one prediction/reference pair, parsed from the model's reply as untrusted output: Score is required and must be a finite number in [0, 1]; Rationale is optional. Unknown fields in the reply are ignored so a judge may add its own metadata without breaking parsing.
type JudgeVerdict struct {
	// Score is the judge's grade in [0, 1]; higher means a closer match to the reference.
	Score float64 `json:"score"`
	// Rationale is an optional short justification supplied by the judge.
	Rationale string `json:"rationale,omitempty"`
}

// judgeReply mirrors a judge reply for parsing. Score is a pointer so a reply that
// omits it is rejected rather than defaulting to a spurious zero score.
type judgeReply struct {
	Score     *float64 `json:"score"`
	Rationale string   `json:"rationale"`
}

// invalidJudgeReply types a malformed judge reply. The judge is an external
// provider, so a reply that is not a well-formed, in-range verdict is an
// external-service failure (HTTP 502), matching how the sibling semantic metric
// classifies a malformed provider response — not the bench caller's invalid
// input. The caller supplied a valid request; the untrusted model returned an
// unusable response.
func invalidJudgeReply(msg string) *apperrors.AppError {
	return apperrors.New(apperrors.ErrCodeExternalService,
		"llm_judge: "+msg, http.StatusBadGateway)
}

// judgeProviderError classifies a judge call failure by cause so consumers receive the actionable code: the metric's own timeout and cancellation surface as timeout/canceled rather than being blanket-labeled external-service. All preserve the cause.
func judgeProviderError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return apperrors.Timeout("llm_judge").WithCause(err)
	case errors.Is(err, context.Canceled):
		return apperrors.Canceled("llm_judge").WithCause(err)
	default:
		return apperrors.New(apperrors.ErrCodeExternalService,
			"llm_judge: judge provider failed", http.StatusBadGateway).WithCause(err)
	}
}

// ensureCompleteReason rejects a completion that did not end cleanly before its body is trusted as a verdict. Only a natural stop (or an unreported reason) is accepted: a length truncation, content-filter block, or provider error/cancellation means generation did not finish normally, so its body — even if it happens to be valid JSON — is an external-service error rather than a score. A tool-use finish is likewise rejected, since the judge is called without tools. An empty reason is accepted rather than fabricated into a failure, since parsing still rejects any body that is not a well-formed verdict.
func ensureCompleteReason(reason chat.FinishReason) error {
	switch reason {
	case "", chat.FinishReasonStop:
		return nil
	default:
		return invalidJudgeReply(fmt.Sprintf(
			"judge completion did not finish normally (stop reason %q); its reply is not a trustworthy verdict", reason))
	}
}

// parseJudgeVerdict parses an untrusted judge reply into a validated [JudgeVerdict]. The reply must be a single JSON object (a surrounding Markdown code fence is tolerated); any other prose, a non-JSON body, a missing score, or a non-finite or out-of-range score is a typed external-service error rather than a trusted score. This is the trust boundary for reply shape and range, not a prompt-injection detector.
func parseJudgeVerdict(reply string) (JudgeVerdict, error) {
	body := stripCodeFence(strings.TrimSpace(reply))
	var parsed judgeReply
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return JudgeVerdict{}, invalidJudgeReply("judge reply was not a valid JSON verdict").WithCause(err)
	}
	if err := rejectDuplicateJudgeKeys(body); err != nil {
		return JudgeVerdict{}, err
	}
	if parsed.Score == nil {
		return JudgeVerdict{}, invalidJudgeReply("judge reply is missing the required score field")
	}
	score := *parsed.Score
	if math.IsNaN(score) || math.IsInf(score, 0) || score < 0 || score > 1 {
		return JudgeVerdict{}, invalidJudgeReply(
			fmt.Sprintf("judge score %v is out of the required range [0, 1]", score))
	}
	return JudgeVerdict{Score: score, Rationale: parsed.Rationale}, nil
}

// rejectDuplicateJudgeKeys rejects a reply whose top-level JSON object repeats a
// field. [json.Unmarshal] silently keeps the last of duplicate keys, so a reply
// like {"score":0,"score":1} would parse to a single, attacker-chosen score; a
// verdict with duplicate fields is ambiguous and is treated as an untrusted,
// malformed reply. Only the top-level object is scanned — nested objects are the
// judge's own opaque metadata, not the verdict fields. A body that is not a JSON
// object, or is otherwise malformed, is left for [json.Unmarshal] to report.
func rejectDuplicateJudgeKeys(body string) error {
	dec := json.NewDecoder(strings.NewReader(body))
	tok, _ := dec.Token()
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	seen := make(map[string]struct{})
	var dupErr error
	depth := 0
	expectKey := true
	for {
		t, err := dec.Token()
		if err != nil {
			break
		}
		if d, ok := t.(json.Delim); ok {
			switch d {
			case '{', '[':
				depth++
			case '}', ']':
				if depth == 0 {
					return nil
				}
				depth--
			}
			expectKey = depth == 0
			continue
		}
		if depth != 0 {
			continue
		}
		if !expectKey {
			expectKey = true
			continue
		}
		key, _ := t.(string)
		canon := canonicalJudgeKey(key)
		if _, dup := seen[canon]; dup {
			dupErr = invalidJudgeReply(fmt.Sprintf(
				"judge reply repeats the %q field; a verdict with duplicate fields is ambiguous", key))
			break
		}
		seen[canon] = struct{}{}
		expectKey = false
	}
	return dupErr
}

// judgeVerdictFields is the set of top-level fields [judgeReply] binds. Membership
// is tested against a lowercased key because [encoding/json] matches struct fields
// case-insensitively.
var judgeVerdictFields = map[string]struct{}{"score": {}, "rationale": {}}

// canonicalJudgeKey folds a verdict field's key to the case-insensitive identity
// [encoding/json] actually binds it under, so a reply like {"score":0,"Score":1}
// is caught as a duplicate rather than silently binding the same struct field
// twice and keeping the last value. Keys that are not verdict fields keep their
// exact spelling: they are opaque judge metadata, not scored, so only an exact
// repeat of one is ambiguous.
func canonicalJudgeKey(key string) string {
	lower := strings.ToLower(key)
	if _, ok := judgeVerdictFields[lower]; ok {
		return lower
	}
	return key
}

// stripCodeFence strips a single surrounding Markdown code fence (```json … ```), returning the inner body; input without a fence is returned unchanged.
func stripCodeFence(text string) string {
	rest, ok := strings.CutPrefix(text, "```")
	if !ok {
		return text
	}
	// Drop an optional language tag on the opening fence line (for example ```json).
	if newline := strings.IndexByte(rest, '\n'); newline >= 0 {
		rest = rest[newline+1:]
	}
	inner, ok := strings.CutSuffix(strings.TrimRight(rest, " \t\r\n"), "```")
	if !ok {
		return text
	}
	return strings.TrimSpace(inner)
}
