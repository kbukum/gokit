package util

// MaskSecret hides sensitive parts of a string for safe display in logs.
// If the string is shorter than visiblePrefix, it is fully masked.
func MaskSecret(s string, visiblePrefix int) string {
	if len(s) <= visiblePrefix {
		return "***"
	}
	return s[:visiblePrefix] + "***"
}
