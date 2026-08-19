package util

import "testing"

func TestToSnakeCase(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"camelCaseString":  "camel_case_string",
		"Kebab-Case-Str":   "kebab_case_str",
		"already_snake":    "already_snake",
		"With Spaces Here": "with_spaces_here",
		"HTTPServer":       "h_t_t_p_server",
	}
	for in, want := range cases {
		if got := ToSnakeCase(in); got != want {
			t.Errorf("ToSnakeCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToKebabCase(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"camelCaseString": "camel-case-string",
		"snake_case_str":  "snake-case-str",
		"With Spaces":     "with-spaces",
	}
	for in, want := range cases {
		if got := ToKebabCase(in); got != want {
			t.Errorf("ToKebabCase(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToCamelCase(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"snake_case_string": "snakeCaseString",
		"Kebab-Case-String": "kebabCaseString",
		"with spaces here":  "withSpacesHere",
		"already":           "already",
	}
	for in, want := range cases {
		if got := ToCamelCase(in); got != want {
			t.Errorf("ToCamelCase(%q) = %q, want %q", in, got, want)
		}
	}
}
