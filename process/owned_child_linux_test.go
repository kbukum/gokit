package process

import (
	"strings"
	"testing"
)

func childStat(state, status string) []byte {
	fields := make([]string, 50)
	for i := range fields {
		fields[i] = "0"
	}
	fields[0], fields[49] = state, status
	return []byte("123 (command ) with spaces) " + strings.Join(fields, " "))
}

func TestParseChildStat(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		data   []byte
		exited bool
		code   int
		forced bool
		err    bool
	}{
		{name: "running", data: childStat("R", "0")},
		{name: "exit zero", data: childStat("Z", "0"), exited: true},
		{name: "exit seven", data: childStat("Z", "1792"), exited: true, code: 7},
		{name: "forced", data: childStat("Z", "9"), exited: true, code: -1, forced: true},
		{name: "missing name", data: []byte("bad"), err: true},
		{name: "missing fields", data: []byte("123 (name) Z"), err: true},
		{name: "invalid status", data: childStat("Z", "invalid"), err: true},
		{name: "overflow status", data: childStat("Z", "4294967296"), err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			state, exited, err := parseChildStat(tc.data)
			if (err != nil) != tc.err || exited != tc.exited || state.forced != tc.forced {
				t.Fatalf("state=%+v exited=%t err=%v", state, exited, err)
			}
			if exited {
				code := -1
				if state.code != nil {
					code = *state.code
				}
				if code != tc.code {
					t.Fatalf("exit code=%d want=%d", code, tc.code)
				}
			}
		})
	}
}

func FuzzParseChildStat(f *testing.F) {
	f.Add(childStat("Z", "1792"))
	f.Add(childStat("R", "0"))
	f.Add([]byte("malformed"))
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 4096 {
			t.Skip("kernel stat reads have a 4 KiB bound")
		}
		state, _, _ := parseChildStat(data)
		if state.code != nil && (*state.code < 0 || *state.code > 255) {
			t.Fatalf("invalid exit code: %d", *state.code)
		}
	})
}
