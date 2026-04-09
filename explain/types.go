package explain

// Signal represents a single analysis signal with a name, numeric value,
// and human-readable label. Signals are the input to explanation generation.
type Signal struct {
	Name  string  `json:"name"`  // Machine identifier, e.g. "frequency_score".
	Value float64 `json:"value"` // Numeric value, typically 0.0–1.0.
	Label string  `json:"label"` // Human-readable description.
}

// Request configures an explanation generation call.
type Request struct {
	// Signals are the analysis results to explain.
	Signals []Signal `json:"signals"`
	// Template overrides the default prompt template.
	// Use {{.Signals}} for signal table insertion.
	// If empty, the default template is used.
	Template string `json:"template,omitempty"`
	// MaxTokens limits the LLM response length. 0 means provider default.
	MaxTokens int `json:"max_tokens,omitempty"`
	// Context provides additional context for the explanation (e.g., content type, source).
	Context string `json:"context,omitempty"`
}

// Explanation is the structured output from the LLM.
type Explanation struct {
	// Summary is a one-paragraph human-readable conclusion.
	Summary string `json:"summary"`
	// Reasoning contains step-by-step analysis for each signal.
	Reasoning []ReasoningStep `json:"reasoning"`
	// KeyFactors lists the most important signals that drove the conclusion.
	KeyFactors []string `json:"key_factors"`
	// Confidence is the LLM's self-assessed confidence in the explanation (0.0–1.0).
	Confidence float64 `json:"confidence"`
}

// ReasoningStep describes how a single signal contributed to the conclusion.
type ReasoningStep struct {
	// Signal is the name of the signal being explained.
	Signal string `json:"signal"`
	// Finding describes what the signal value indicates.
	Finding string `json:"finding"`
	// Impact is the significance level: "high", "medium", or "low".
	Impact string `json:"impact"`
}
