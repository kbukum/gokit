package util

// DeepMerge recursively merges override into base.
// When both values for a key are map[string]any they are merged recursively;
// otherwise the override value replaces the base. Neither input is mutated.
func DeepMerge(base, override map[string]any) map[string]any {
	result := make(map[string]any, len(base)+len(override))
	for k, v := range base {
		result[k] = v
	}
	for k, ov := range override {
		bv, exists := result[k]
		if exists {
			bm, bOK := bv.(map[string]any)
			om, oOK := ov.(map[string]any)
			if bOK && oOK {
				result[k] = DeepMerge(bm, om)
				continue
			}
		}
		result[k] = ov
	}
	return result
}
