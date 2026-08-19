package util

import "strings"

// parseDecimalScaled parses a non-negative decimal string and scales it by
// multiplier using integer math, returning the scaled value and whether the input
// was well-formed and did not overflow uint64. A leading sign or non-digit content
// is rejected.
func parseDecimalScaled(value string, multiplier uint64) (uint64, bool) {
	if value == "" || value[0] == '-' || value[0] == '+' {
		return 0, false
	}

	whole, fraction, hasDot := strings.Cut(value, ".")
	if whole == "" && fraction == "" {
		return 0, false
	}
	if !allDigits(whole) || !allDigits(fraction) {
		return 0, false
	}

	var wholeVal uint64
	if whole != "" {
		v, ok := parseUint(whole)
		if !ok {
			return 0, false
		}
		wholeVal = v
	}
	wholeScaled, ok := mulChecked(wholeVal, multiplier)
	if !ok {
		return 0, false
	}
	if !hasDot || fraction == "" {
		return wholeScaled, true
	}

	scale, ok := pow10(len(fraction))
	if !ok {
		return 0, false
	}
	fracVal, ok := parseUint(fraction)
	if !ok {
		return 0, false
	}
	fracMul, ok := mulChecked(fracVal, multiplier)
	if !ok {
		return 0, false
	}
	fractionScaled := fracMul / scale
	return addChecked(wholeScaled, fractionScaled)
}

func allDigits(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}

func parseUint(s string) (uint64, bool) {
	var out uint64
	for i := 0; i < len(s); i++ {
		digit := uint64(s[i] - '0')
		next, ok := mulChecked(out, 10)
		if !ok {
			return 0, false
		}
		next, ok = addChecked(next, digit)
		if !ok {
			return 0, false
		}
		out = next
	}
	return out, true
}

func mulChecked(a, b uint64) (uint64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	product := a * b
	if product/b != a {
		return 0, false
	}
	return product, true
}

func addChecked(a, b uint64) (uint64, bool) {
	sum := a + b
	if sum < a {
		return 0, false
	}
	return sum, true
}

func pow10(n int) (uint64, bool) {
	out := uint64(1)
	for i := 0; i < n; i++ {
		next, ok := mulChecked(out, 10)
		if !ok {
			return 0, false
		}
		out = next
	}
	return out, true
}
