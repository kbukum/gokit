package bench

import (
	"encoding/json"
	"testing"
)

func TestDirectionJSONRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dir  Direction
		want string
	}{
		{HigherIsBetter, `"higher_is_better"`},
		{LowerIsBetter, `"lower_is_better"`},
		{Neutral, `"neutral"`},
	}

	for _, tc := range tests {
		t.Run(tc.want, func(t *testing.T) {
			t.Parallel()

			data, err := json.Marshal(tc.dir)
			if err != nil {
				t.Fatalf("Marshal(%v): %v", tc.dir, err)
			}
			if string(data) != tc.want {
				t.Errorf("Marshal(%v) = %s, want %s", tc.dir, data, tc.want)
			}
			var got Direction
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("Unmarshal(%s): %v", data, err)
			}
			if got != tc.dir {
				t.Errorf("round trip = %v, want %v", got, tc.dir)
			}
		})
	}
}

func TestDirectionZeroValueIsHigherIsBetter(t *testing.T) {
	t.Parallel()

	var d Direction
	if d != HigherIsBetter {
		t.Errorf("zero Direction = %v, want HigherIsBetter", d)
	}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if string(data) != `"higher_is_better"` {
		t.Errorf("zero Direction marshals to %s, want higher_is_better", data)
	}
}

func TestDirectionUnmarshalDecodesJSONEscapes(t *testing.T) {
	t.Parallel()

	// A valid escaped spelling must decode the same as its plain form: the JSON
	// string is decoded before the enum value is matched, matching serde.
	for _, in := range []string{
		`"\u0068igher_is_better"`,
		`"lower\u005fis_better"`,
	} {
		want := HigherIsBetter
		if in == `"lower\u005fis_better"` {
			want = LowerIsBetter
		}
		var d Direction
		if err := json.Unmarshal([]byte(in), &d); err != nil {
			t.Fatalf("Unmarshal(%s): %v", in, err)
		}
		if d != want {
			t.Errorf("Unmarshal(%s) = %v, want %v", in, d, want)
		}
	}
}

func TestDirectionUnmarshalRejectsUnknown(t *testing.T) {
	t.Parallel()

	for _, in := range []string{`"sideways"`, `42`, `"HIGHER_IS_BETTER"`, `""`} {
		var d Direction
		if err := json.Unmarshal([]byte(in), &d); err == nil {
			t.Errorf("Unmarshal(%s) = nil error, want a typed error", in)
		}
	}
}

func TestDirectionClassification(t *testing.T) {
	t.Parallel()

	tests := []struct {
		dir         Direction
		delta       float64
		improvement bool
		regression  bool
	}{
		{HigherIsBetter, 0.1, true, false},
		{HigherIsBetter, -0.1, false, true},
		{HigherIsBetter, 0, false, false},
		{LowerIsBetter, -0.1, true, false},
		{LowerIsBetter, 0.1, false, true},
		{LowerIsBetter, 0, false, false},
		{Neutral, 0.1, false, false},
		{Neutral, -0.1, false, false},
	}

	for _, tc := range tests {
		if got := tc.dir.IsImprovement(tc.delta); got != tc.improvement {
			t.Errorf("%v.IsImprovement(%v) = %v, want %v", tc.dir, tc.delta, got, tc.improvement)
		}
		if got := tc.dir.IsRegression(tc.delta); got != tc.regression {
			t.Errorf("%v.IsRegression(%v) = %v, want %v", tc.dir, tc.delta, got, tc.regression)
		}
	}
}
