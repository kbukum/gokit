package skill

import (
	"fmt"

	"github.com/kbukum/gokit/tool"
)

// Activate composes the effective safety and per-tool envelopes for a skill
// into an ActivationDecision. The skill is Allowed only when every referenced
// tool resolves to a present envelope whose required scopes survive
// intersection with the principal grants and operator ceiling; otherwise the
// decision fails closed and Reason names the first blocking tool.
func Activate(m Manifest, principalGrants, operatorCeiling []string, toolEnvelopes map[string]tool.Envelope) ActivationDecision {
	tools := EffectiveEnvelope(m, principalGrants, operatorCeiling, toolEnvelopes)
	decision := ActivationDecision{
		SkillName: m.Name,
		Allowed:   true,
		EffectiveSafety: EffectiveSafety(m, func(name string) tool.Safety {
			return toolEnvelopes[name].Safety
		}),
		Tools: tools,
	}
	for i := range tools {
		if !tools[i].Allowed {
			decision.Allowed = false
			decision.Reason = fmt.Sprintf("tool %q: %s", tools[i].Name, tools[i].Reason)
			break
		}
	}
	return decision
}

func EffectiveSafety(m Manifest, lookup func(toolName string) tool.Safety) tool.Safety {
	maxSafety := toToolSafety(m.Safety)
	for _, name := range m.References.Tools {
		if s := lookup(name); s.Rank() > maxSafety.Rank() {
			maxSafety = s
		}
	}
	return maxSafety
}

func toToolSafety(s Safety) tool.Safety {
	switch s {
	case SafetyDestructive:
		return tool.SafetyDestructive
	case SafetyMutating:
		return tool.SafetyMutating
	default:
		return tool.SafetyReadOnly
	}
}

func EffectiveEnvelope(m Manifest, principalGrants, operatorCeiling []string, toolEnvelopes map[string]tool.Envelope) []EffectiveTool {
	grants := set(principalGrants)
	ceiling := set(operatorCeiling)
	out := make([]EffectiveTool, 0, len(m.References.Tools))
	for _, name := range m.References.Tools {
		declared, ok := toolEnvelopes[name]
		if !ok {
			out = append(out, EffectiveTool{Name: name, Allowed: false, Reason: "tool envelope missing"})
			continue
		}
		eff := declared
		eff.Scopes = intersectScopes(declared.Scopes, grants, ceiling)
		allowed := len(declared.Scopes) == len(eff.Scopes)
		reason := ""
		if !allowed {
			reason = "scope intersection removed required scopes"
		}
		out = append(out, EffectiveTool{Name: name, Envelope: eff, Allowed: allowed, Reason: reason})
	}
	return out
}

func set(values []string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, v := range values {
		m[v] = struct{}{}
	}
	return m
}

func intersectScopes(scopes []string, grants, ceiling map[string]struct{}) []string {
	out := []string{}
	for _, s := range scopes {
		_, g := grants[s]
		_, c := ceiling[s]
		if g && c {
			out = append(out, s)
		}
	}
	return out
}
