package query

import "testing"

func TestExcludeFieldConditions_ResolvesAlias(t *testing.T) {
	t.Parallel()
	cfg := Config{FieldAliases: map[string]string{"cat": "category"}}
	conds := []Condition{
		{Field: "cat", Operator: OpEq, Value: "tools"},
		{Field: "price", Operator: OpGt, Value: "5"},
	}
	got := excludeFieldConditions(conds, "category", cfg)
	if len(got) != 1 || got[0].Field != "price" {
		t.Fatalf("excludeFieldConditions = %+v", got)
	}
}
