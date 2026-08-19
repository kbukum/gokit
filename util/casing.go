package util

import (
	"strings"
	"unicode"
)

// ToSnakeCase converts a string to snake_case, collapsing runs of separators and
// inserting underscores before interior uppercase letters.
//
//	ToSnakeCase("camelCaseString") == "camel_case_string"
//	ToSnakeCase("Kebab-Case-String") == "kebab_case_string"
func ToSnakeCase(s string) string {
	return convertCase(s, '_', false)
}

// ToKebabCase converts a string to kebab-case, collapsing runs of separators and
// inserting hyphens before interior uppercase letters.
//
//	ToKebabCase("camelCaseString") == "camel-case-string"
func ToKebabCase(s string) string {
	return convertCase(s, '-', false)
}

// convertCase lowercases s and joins words with sep. Word boundaries are runs of
// '_', '-', or ' ', and transitions into an uppercase letter.
func convertCase(s string, sep rune, _ bool) string {
	out := make([]rune, 0, len(s)+4)
	isFirst := true
	runes := []rune(s)
	for i, c := range runes {
		if c == '_' || c == '-' || c == ' ' {
			if !isFirst && i+1 < len(runes) && (len(out) == 0 || out[len(out)-1] != sep) {
				out = append(out, sep)
			}
			continue
		}
		if unicode.IsUpper(c) {
			if !isFirst && (len(out) == 0 || out[len(out)-1] != sep) {
				out = append(out, sep)
			}
			out = append(out, []rune(strings.ToLower(string(c)))...)
		} else {
			out = append(out, c)
		}
		isFirst = false
	}
	return string(out)
}

// ToCamelCase converts a string to camelCase, treating '_', '-', and ' ' as word
// boundaries and lowercasing the leading character.
//
//	ToCamelCase("snake_case_string") == "snakeCaseString"
//	ToCamelCase("Kebab-Case-String") == "kebabCaseString"
func ToCamelCase(s string) string {
	out := make([]rune, 0, len(s))
	capitalizeNext := false
	isFirst := true
	for _, c := range s {
		if c == '_' || c == '-' || c == ' ' {
			capitalizeNext = true
			continue
		}
		switch {
		case isFirst:
			out = append(out, []rune(strings.ToLower(string(c)))...)
			isFirst = false
		case capitalizeNext:
			out = append(out, []rune(strings.ToUpper(string(c)))...)
			capitalizeNext = false
		default:
			out = append(out, c)
		}
	}
	return string(out)
}
