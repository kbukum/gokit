package util

import "testing"

func TestIsNil(t *testing.T) {
	t.Parallel()
	type iface interface{ M() }
	var nilPtr *int
	var nilMap map[string]int
	var nilSlice []int
	var nilChan chan int
	var nilFunc func()
	var nilIface iface
	cases := []struct {
		name string
		in   any
		want bool
	}{
		{"untyped nil", nil, true},
		{"typed nil pointer", nilPtr, true},
		{"nil map", nilMap, true},
		{"nil slice", nilSlice, true},
		{"nil channel", nilChan, true},
		{"nil func", nilFunc, true},
		{"nil interface value", nilIface, true},
		{"non-nil pointer", new(int), false},
		{"non-nil value", 42, false},
		{"empty string", "", false},
		{"zero struct", struct{}{}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsNil(tc.in); got != tc.want {
				t.Fatalf("IsNil(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}
