package skill

import "github.com/kbukum/gokit/tool"

type EffectiveTool struct {
	Name     string        `json:"name"`
	Envelope tool.Envelope `json:"envelope"`
	Allowed  bool          `json:"allowed"`
	Reason   string        `json:"reason,omitempty"`
}

type ActivationDecision struct {
	SkillName       string          `json:"skill_name"`
	Allowed         bool            `json:"allowed"`
	Reason          string          `json:"reason,omitempty"`
	EffectiveSafety tool.Safety     `json:"effective_safety"`
	Tools           []EffectiveTool `json:"tools"`
}
