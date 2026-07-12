package util

// DeepMerge recursively merges override into base, treating both as decoded
// JSON/YAML documents. When both values for a key are map[string]any they are
// merged recursively; otherwise the override value replaces the base. Neither
// input is mutated.
//
// The map[string]any type is a deliberate, documented opaque-value exception to
// the no-any rule: the function operates on genuinely heterogeneous document
// trees whose leaf values cannot be given a closed type.
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
