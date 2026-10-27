package metric

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strings"

	apperrors "github.com/kbukum/gokit/errors"
)

const (
	// maxJudgeReplyBytes bounds a reply the judge parses at all.
	maxJudgeReplyBytes = 64 * 1024
	// maxRationaleBytes bounds the rationale a verdict keeps.
	maxRationaleBytes = 1024
)

type scoreReply struct {
	Score     *float64 `json:"score"`
	Rationale *string  `json:"rationale"`
}

type pairReply struct {
	Winner    *string `json:"winner"`
	Rationale *string `json:"rationale"`
}

// parseScoreReply reads {"score": <integer 1..points>, "rationale": <string>}. It returns a nonempty reason when the reply is not exactly that.
func parseScoreReply(reply string, points int) (score int, rationale, reason string) {
	var r scoreReply
	if reason := decodeStrict(reply, &r, "score", "rationale"); reason != "" {
		return 0, "", reason
	}
	if r.Score == nil {
		return 0, "", "the reply has no score"
	}
	f := *r.Score
	if math.IsNaN(f) || math.IsInf(f, 0) || f != math.Trunc(f) || f < 1 || f > float64(points) {
		return 0, "", fmt.Sprintf("the score %v is not an integer from 1 to %d", f, points)
	}
	return int(f), rationaleOf(r.Rationale), ""
}

// parsePairReply reads {"winner": "A" | "B" | "tie", "rationale": <string>}.
func parsePairReply(reply string) (winner Preference, rationale, reason string) {
	var r pairReply
	if reason := decodeStrict(reply, &r, "winner", "rationale"); reason != "" {
		return "", "", reason
	}
	if r.Winner == nil {
		return "", "", "the reply has no winner"
	}
	switch Preference(*r.Winner) {
	case PreferA, PreferB, PreferTie:
		return Preference(*r.Winner), rationaleOf(r.Rationale), ""
	default:
		return "", "", fmt.Sprintf("the winner %q is not A, B or tie", *r.Winner)
	}
}

// decodeStrict decodes one JSON object, optionally inside one code fence, with only the named fields, each at most once, and nothing after it.
func decodeStrict(reply string, into any, fields ...string) string {
	body := stripCodeFence(strings.TrimSpace(reply))
	if body == "" {
		return "the reply is empty"
	}
	if reason := duplicateKey(body, fields); reason != "" {
		return reason
	}
	dec := json.NewDecoder(strings.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(into); err != nil {
		return "the reply is not the JSON verdict object"
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return "the reply has text after the JSON verdict"
	}
	return ""
}

// duplicateKey refuses an object that repeats a field, comparing known fields without case, since encoding/json matches them that way and the last one would silently win.
func duplicateKey(body string, fields []string) string {
	dec := json.NewDecoder(bytes.NewReader([]byte(body)))
	if tok, err := dec.Token(); err != nil || tok != json.Delim('{') {
		return ""
	}
	seen := map[string]bool{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return ""
		}
		key, _ := tok.(string)
		canon := key
		for _, f := range fields {
			if strings.EqualFold(f, key) {
				canon = f
			}
		}
		if seen[canon] {
			return fmt.Sprintf("the reply repeats the %q field", key)
		}
		seen[canon] = true
		var skip json.RawMessage
		if err := dec.Decode(&skip); err != nil {
			return ""
		}
	}
	return ""
}

func rationaleOf(s *string) string {
	if s == nil {
		return ""
	}
	return truncate(strings.TrimSpace(*s), maxRationaleBytes)
}

// stripCodeFence unwraps one Markdown code fence around the reply, if present.
func stripCodeFence(text string) string {
	rest, ok := strings.CutPrefix(text, "```")
	if !ok {
		return text
	}
	if newline := strings.IndexByte(rest, '\n'); newline >= 0 {
		rest = rest[newline+1:]
	}
	inner, ok := strings.CutSuffix(strings.TrimRight(rest, " \t\r\n"), "```")
	if !ok {
		return text
	}
	return strings.TrimSpace(inner)
}

// judgeProviderError classifies a failed judge call.
func judgeProviderError(err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return apperrors.Timeout("judge").WithCause(err)
	case errors.Is(err, context.Canceled):
		return apperrors.Canceled("judge").WithCause(err)
	default:
		return apperrors.New(apperrors.ErrCodeExternalService, "judge provider failed").WithCause(err)
	}
}
