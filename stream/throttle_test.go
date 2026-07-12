package stream

import (
	"context"
	"testing"
	"time"
)

func TestThrottle_DropsRapidValues(t *testing.T) {
	p := FromSlice([]int{1, 2, 3, 4, 5})
	// With a very large interval, only the first value should pass
	throttled := Throttle(p, time.Hour)
	got, err := Collect(context.Background(), throttled)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("expected [1], got %v", got)
	}
}

func TestThrottle_AllPassWithZeroInterval(t *testing.T) {
	p := FromSlice([]int{1, 2, 3})
	throttled := Throttle(p, 0)
	got, err := Collect(context.Background(), throttled)
	if err != nil {
		t.Fatal(err)
	}
	if !intSliceEqual(got, []int{1, 2, 3}) {
		t.Errorf("expected [1 2 3], got %v", got)
	}
}

func TestThrottle_Empty(t *testing.T) {
	p := FromSlice([]int{})
	throttled := Throttle(p, time.Second)
	got, err := Collect(context.Background(), throttled)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty, got %v", got)
	}
}

func TestThrottle_SingleValue(t *testing.T) {
	p := FromSlice([]int{42})
	throttled := Throttle(p, time.Hour)
	got, err := Collect(context.Background(), throttled)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != 42 {
		t.Errorf("expected [42], got %v", got)
	}
}
