package util

import "testing"

func TestFormatBytes(t *testing.T) {
	t.Parallel()
	cases := map[uint64]string{
		512:           "512 B",
		1024:          "1 KiB",
		1536:          "1.5 KiB",
		1048576:       "1 MiB",
		1073741824:    "1 GiB",
		1099511627776: "1 TiB",
	}
	for in, want := range cases {
		if got := FormatBytes(in); got != want {
			t.Errorf("FormatBytes(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestParseBytes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   string
		want uint64
	}{
		{"512", 512},
		{"1KiB", 1024},
		{"1.5 KiB", 1536},
		{"10mb", 10 * 1024 * 1024},
		{"2 g", 2 * 1024 * 1024 * 1024},
		{"1024", 1024},
	}
	for _, c := range cases {
		got, err := ParseBytes(c.in)
		if err != nil || got != c.want {
			t.Errorf("ParseBytes(%q) = (%d, %v), want %d", c.in, got, err, c.want)
		}
	}
}

func TestParseBytesErrors(t *testing.T) {
	t.Parallel()
	for _, in := range []string{"", "abc", "-5", "10xb"} {
		if _, err := ParseBytes(in); err == nil {
			t.Errorf("ParseBytes(%q) expected error", in)
		}
	}
}

func TestParseBytesRoundTripUnits(t *testing.T) {
	t.Parallel()
	if got, _ := ParseBytes("1kb"); got != 1024 {
		t.Errorf("kb alias = %d", got)
	}
	if got, _ := ParseBytes("1ki"); got != 1024 {
		t.Errorf("ki alias = %d", got)
	}
}
