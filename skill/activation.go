package skill

import "github.com/kbukum/gokit/tool"

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
