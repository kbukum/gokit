package workload

import (
	"fmt"
	"strconv"
	"strings"
)

// ParseMemory converts human-readable memory strings to bytes.
// Supported suffixes: k/ki (KiB), m/mi (MiB), g/gi (GiB), t/ti (TiB).
// Without suffix, the value is treated as bytes.
func ParseMemory(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("workload: empty memory string")
	}

	multiplier := int64(1)
	switch {
	case strings.HasSuffix(s, "ti"):
		multiplier = 1024 * 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "ti")
	case strings.HasSuffix(s, "gi"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "gi")
	case strings.HasSuffix(s, "mi"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "mi")
	case strings.HasSuffix(s, "ki"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "ki")
	case strings.HasSuffix(s, "t"):
		multiplier = 1024 * 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "t")
	case strings.HasSuffix(s, "g"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "g")
	case strings.HasSuffix(s, "m"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "k"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "k")
	}

	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("workload: parse memory %q: %w", s, err)
	}
	if val < 0 {
		return 0, fmt.Errorf("workload: memory must be non-negative: %d", val)
	}
	return val * multiplier, nil
}

// ParseCPU converts human-readable CPU strings to nanocores.
// Supported formats: "0.5" (cores), "500m" (millicores), "1" (1 core).
func ParseCPU(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("workload: empty CPU string")
	}

	if strings.HasSuffix(s, "m") {
		// Millicores: "500m" = 0.5 CPU = 500,000,000 nanocores
		val, err := strconv.ParseFloat(strings.TrimSuffix(s, "m"), 64)
		if err != nil {
			return 0, fmt.Errorf("workload: parse CPU %q: %w", s, err)
		}
		return int64(val * 1e6), nil
	}

	// Cores: "0.5" = 500,000,000 nanocores, "1" = 1,000,000,000
	val, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, fmt.Errorf("workload: parse CPU %q: %w", s, err)
	}
	return int64(val * 1e9), nil
}

// FormatMemory converts bytes to a human-readable string.
func FormatMemory(bytes int64) string {
	switch {
	case bytes >= 1024*1024*1024:
		return fmt.Sprintf("%dg", bytes/(1024*1024*1024))
	case bytes >= 1024*1024:
		return fmt.Sprintf("%dm", bytes/(1024*1024))
	case bytes >= 1024:
		return fmt.Sprintf("%dk", bytes/1024)
	default:
		return fmt.Sprintf("%d", bytes)
	}
}

// FormatCPU converts nanocores to a human-readable string.
func FormatCPU(nanocores int64) string {
	if nanocores%1e9 == 0 {
		return fmt.Sprintf("%d", nanocores/1e9)
	}
	if nanocores%1e6 == 0 {
		return fmt.Sprintf("%dm", nanocores/1e6)
	}
	return fmt.Sprintf("%.3f", float64(nanocores)/1e9)
}
