package metric

import (
	"math"
	"regexp"
	"strings"

	"github.com/kbukum/gokit/bench"
)

// BLEU matches sacreBLEU 2.4.3 corpus BLEU, 13a, exp smoothing, no effective order.
func BLEU() Metric[string] { return &textMetric{kind: "bleu"} }

// ROUGE1 computes mean unigram overlap F, precision, and recall.
func ROUGE1() Metric[string] { return &textMetric{kind: "rouge1"} }

// ROUGE2 computes mean bigram overlap F, precision, and recall.
func ROUGE2() Metric[string] { return &textMetric{kind: "rouge2"} }

// ROUGEL computes mean longest-common-subsequence F, precision, and recall.
func ROUGEL() Metric[string] { return &textMetric{kind: "rougeL"} }

type textMetric struct{ kind string }

func (m *textMetric) Identity() bench.MetricIdentity {
	config := map[string]string{"tokenizer": "rouge-default", "stemmer": "false", "aggregation": "mean"}
	version := "rouge-score:0.1.2"
	if m.kind == "bleu" {
		version = "sacrebleu:2.4.3"
		config = map[string]string{"tokenizer": "13a", "smoothing": "exp", "max_order": "4", "effective_order": "false", "case_sensitive": "true"}
	}
	return bench.MetricIdentity{Name: m.kind, Version: version, Config: config}
}

func (m *textMetric) NewAccumulator() bench.Accumulator[string] {
	return &textAccumulator{kind: m.kind}
}

func (m *textMetric) Compute(samples []bench.ScoredSample[string]) Result {
	a := &textAccumulator{kind: m.kind}
	for _, s := range samples {
		a.add(s)
	}
	return a.Result()
}

type textAccumulator struct {
	kind                 string
	count                int
	matches              [4]int
	totals               [4]int
	hypLen, refLen       int
	precision, recall, f float64
}

func (a *textAccumulator) Add(s bench.ScoredSample[string]) error { a.add(s); return nil }
func (a *textAccumulator) add(s bench.ScoredSample[string]) {
	a.count++
	if a.kind == "bleu" {
		hyp, ref := tokenize13a(s.Prediction.Label), tokenize13a(s.Sample.Label)
		a.hypLen += len(hyp)
		a.refLen += len(ref)
		for n := 1; n <= 4; n++ {
			h, r := ngrams(hyp, n), ngrams(ref, n)
			for gram, count := range h {
				a.matches[n-1] += min(count, r[gram])
				a.totals[n-1] += count
			}
		}
		return
	}
	hyp, ref := rougeTokens(s.Prediction.Label), rougeTokens(s.Sample.Label)
	hypCount, refCount, overlap := len(hyp), len(ref), 0
	if a.kind == "rougeL" {
		overlap = lcs(hyp, ref)
	} else {
		n := 1
		if a.kind == "rouge2" {
			n = 2
		}
		h, r := ngrams(hyp, n), ngrams(ref, n)
		hypCount, refCount = max(0, len(hyp)-n+1), max(0, len(ref)-n+1)
		for gram, count := range h {
			overlap += min(count, r[gram])
		}
	}
	p, r := 0.0, 0.0
	if hypCount > 0 {
		p = float64(overlap) / float64(hypCount)
	}
	if refCount > 0 {
		r = float64(overlap) / float64(refCount)
	}
	a.precision += p
	a.recall += r
	if p+r > 0 {
		a.f += 2 * p * r / (p + r)
	}
}

func (a *textAccumulator) Result() Result {
	result := Result{Name: a.kind}
	if a.kind != "bleu" {
		if a.count > 0 {
			result.Value = a.f / float64(a.count)
			result.Values = map[string]float64{"precision": a.precision / float64(a.count), "recall": a.recall / float64(a.count)}
		}
		return result
	}
	if a.hypLen == 0 || a.matches == [4]int{} {
		return result
	}
	logSum, smooth := 0.0, 1.0
	for i, total := range a.totals {
		if total == 0 {
			return result
		}
		p := float64(a.matches[i]) / float64(total)
		if a.matches[i] == 0 {
			smooth *= 2
			p = 1 / (smooth * float64(total))
		}
		logSum += math.Log(p)
	}
	brevity := math.Min(0, 1-float64(a.refLen)/float64(a.hypLen))
	result.Value = math.Exp(brevity + logSum/4)
	return result
}

func ngrams(tokens []string, n int) map[string]int {
	out := make(map[string]int)
	for i := 0; i+n <= len(tokens); i++ {
		out[strings.Join(tokens[i:i+n], "\x00")]++
	}
	return out
}

var rougeSeparator = regexp.MustCompile(`[^a-z0-9]+`)

func rougeTokens(text string) []string {
	return strings.Fields(rougeSeparator.ReplaceAllString(strings.ToLower(text), " "))
}

func lcs(a, b []string) int {
	if len(a) < len(b) {
		a, b = b, a
	}
	row := make([]int, len(b)+1)
	for _, x := range a {
		previous := 0
		for j, y := range b {
			old := row[j+1]
			if x == y {
				row[j+1] = previous + 1
			} else {
				row[j+1] = max(row[j], row[j+1])
			}
			previous = old
		}
	}
	return row[len(b)]
}

var bleuEntities = strings.NewReplacer("<skipped>", "", "-\n", "", "\n", " ", "&quot;", "\"", "&amp;", "&", "&lt;", "<", "&gt;", ">")

func tokenize13a(text string) []string {
	runes := []rune(" " + bleuEntities.Replace(text) + " ")
	var b strings.Builder
	digit := func(r rune) bool { return r >= '0' && r <= '9' }
	for i, r := range runes {
		separate := r >= '{' && r <= '~' || r >= '[' && r <= '`' || r >= ' ' && r <= '&' || r >= '(' && r <= '+' || r >= ':' && r <= '@' || r == '/'
		if r == '.' || r == ',' {
			separate = i > 0 && !digit(runes[i-1]) || i+1 < len(runes) && !digit(runes[i+1])
		}
		if r == '-' {
			separate = i > 0 && digit(runes[i-1])
		}
		if separate {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
		if separate {
			b.WriteByte(' ')
		}
	}
	return strings.Fields(b.String())
}
