package util

import (
	"fmt"
	"strings"
)

const (
	bytesPerKiB = 1024
	bytesPerMiB = bytesPerKiB * 1024
	bytesPerGiB = bytesPerMiB * 1024
	bytesPerTiB = bytesPerGiB * 1024
	bytesPerPiB = bytesPerTiB * 1024
)

// FormatBytes renders a byte count using binary (1024-based) units, choosing the
// largest unit for which the value is at least one and trimming trailing zeros.
//
//	FormatBytes(1536) == "1.5 KiB"
//	FormatBytes(1048576) == "1 MiB"
func FormatBytes(bytes uint64) string {
	switch {
	case bytes >= bytesPerPiB:
		return formatScaled(bytes, bytesPerPiB, "PiB")
	case bytes >= bytesPerTiB:
		return formatScaled(bytes, bytesPerTiB, "TiB")
	case bytes >= bytesPerGiB:
		return formatScaled(bytes, bytesPerGiB, "GiB")
	case bytes >= bytesPerMiB:
		return formatScaled(bytes, bytesPerMiB, "MiB")
	case bytes >= bytesPerKiB:
		return formatScaled(bytes, bytesPerKiB, "KiB")
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

func formatScaled(bytes, unit uint64, suffix string) string {
	value := float64(bytes) / float64(unit)
	text := strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", value), "0"), ".")
	return text + " " + suffix
}

// ParseBytes parses a human byte size into a raw byte count. It accepts an optional
// decimal magnitude followed by an optional unit; a bare number is bytes. Units are
// binary (1024-based) and case-insensitive, accepting the full forms (KiB, MiB, GiB,
// TiB, PiB), the ambiguous decimal-looking spellings (KB, MB, …), and the short
// aliases (k, ki, m, mi, g, gi, t, ti, p, pi, b). Whitespace between the number and
// unit is optional.
//
//	ParseBytes("1.5 KiB") == (1536, nil)
//	ParseBytes("10mb") == (10485760, nil)
func ParseBytes(input string) (uint64, error) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return 0, fmt.Errorf("empty byte size")
	}

	splitIdx := len(trimmed)
	for i, c := range trimmed {
		if (c >= '0' && c <= '9') || c == '.' {
			continue
		}
		splitIdx = i
		break
	}
	numberPart := strings.TrimSpace(trimmed[:splitIdx])
	unitPart := strings.TrimSpace(trimmed[splitIdx:])
	if numberPart == "" {
		return 0, fmt.Errorf("invalid byte size %q: missing magnitude", input)
	}

	multiplier, ok := bytesUnitMultiplier(unitPart)
	if !ok {
		return 0, fmt.Errorf("invalid byte size %q: unknown unit %q", input, unitPart)
	}
	scaled, ok := parseDecimalScaled(numberPart, multiplier)
	if !ok {
		return 0, fmt.Errorf("invalid byte size %q", input)
	}
	return scaled, nil
}

func bytesUnitMultiplier(unit string) (uint64, bool) {
	switch strings.ToLower(unit) {
	case "", "b":
		return 1, true
	case "k", "ki", "kb", "kib":
		return bytesPerKiB, true
	case "m", "mi", "mb", "mib":
		return bytesPerMiB, true
	case "g", "gi", "gb", "gib":
		return bytesPerGiB, true
	case "t", "ti", "tb", "tib":
		return bytesPerTiB, true
	case "p", "pi", "pb", "pib":
		return bytesPerPiB, true
	default:
		return 0, false
	}
}
