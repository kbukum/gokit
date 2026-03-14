package explain

import (
	"bytes"
	"context"
	"fmt"
	"text/template"

	"github.com/kbukum/gokit/llm"
	"github.com/kbukum/gokit/provider"
)

// defaultTemplate is the built-in prompt for explanation generation.
const defaultTemplate = `You are an expert analyst. Given the following analysis signals, provide a structured explanation of the findings.

## Analysis Signals

{{range .Signals}}| {{.Name}} | {{printf "%.4f" .Value}} | {{.Label}} |
{{end}}
{{if .Context}}## Additional Context

{{.Context}}
{{end}}
## Instructions

Analyze each signal and provide:
1. A clear summary of the overall finding
2. Step-by-step reasoning for each signal
3. The key factors that most influenced the conclusion
4. Your confidence level (0.0 to 1.0)

Respond with ONLY a JSON object in this exact format:
{
  "summary": "One paragraph summary",
  "reasoning": [
    {"signal": "signal_name", "finding": "what this indicates", "impact": "high|medium|low"}
  ],
  "key_factors": ["factor1", "factor2"],
  "confidence": 0.85
}`

// Generate produces a structured explanation from analysis signals using an LLM.
// It renders the prompt template with the provided signals, calls the LLM,
// and parses the structured JSON response.
//
// The provider parameter accepts any gokit RequestResponse that handles
// LLM completion requests — this includes llm.Adapter directly, or
// any middleware-wrapped version (resilience, caching, etc.).
func Generate(
	ctx context.Context,
	p provider.RequestResponse[llm.CompletionRequest, llm.CompletionResponse],
	req Request,
) (*Explanation, error) {
	if len(req.Signals) == 0 {
		return nil, fmt.Errorf("explain: at least one signal is required")
	}

	prompt, err := renderTemplate(req)
	if err != nil {
		return nil, fmt.Errorf("explain: render template: %w", err)
	}

	var result Explanation
	if err := llm.CompleteStructured(ctx, p, prompt,
		"Explain these analysis results.", &result); err != nil {
		return nil, fmt.Errorf("explain: generate: %w", err)
	}

	return &result, nil
}

// renderTemplate renders the prompt template with the request data.
func renderTemplate(req Request) (string, error) {
	tmplStr := req.Template
	if tmplStr == "" {
		tmplStr = defaultTemplate
	}

	tmpl, err := template.New("explain").Parse(tmplStr)
	if err != nil {
		return "", fmt.Errorf("parse template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, req); err != nil {
		return "", fmt.Errorf("execute template: %w", err)
	}

	return buf.String(), nil
}
