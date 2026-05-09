package prompt

import "sort"

// Validate compares declared variables with placeholders used by tmpl.
func Validate(tmpl PromptTemplate) []ValidationIssue {
	used := map[string]struct{}{}
	for _, match := range placeholderPattern.FindAllStringSubmatch(tmpl.Template, -1) {
		used[match[1]] = struct{}{}
	}
	declared := map[string]struct{}{}
	for _, variable := range tmpl.Variables {
		declared[variable.Name] = struct{}{}
	}
	var issues []ValidationIssue
	for name := range used {
		if _, ok := declared[name]; !ok {
			issues = append(issues, ValidationIssue{Name: name, Kind: "undeclared"})
		}
	}
	for name := range declared {
		if _, ok := used[name]; !ok {
			issues = append(issues, ValidationIssue{Name: name, Kind: "unused"})
		}
	}
	sort.Slice(issues, func(i, j int) bool {
		if issues[i].Name == issues[j].Name {
			return issues[i].Kind < issues[j].Kind
		}
		return issues[i].Name < issues[j].Name
	})
	return issues
}
