package util

// DeepMerge recursively merges override into base (override wins).
// Returns a new map — neither input is mutated.
// When both values for a key are map[string]any they are merged recursively;
// otherwise the override value replaces the base.
func DeepMerge(base, override map[string]any) map[string]any {
	result := make(map[string]any, len(base))
	for k, v := range base {
		result[k] = v
	}
	for k, overVal := range override {
		baseVal, exists := result[k]
		if exists {
			baseMap, baseOk := baseVal.(map[string]any)
			overMap, overOk := overVal.(map[string]any)
			if baseOk && overOk {
				result[k] = DeepMerge(baseMap, overMap)
				continue
			}
		}
		result[k] = overVal
	}
	return result
}
