package media

import (
	"strings"
	"testing"
)

// FuzzParseSRT asserts the SRT parser never panics and that any track it
// produces round-trips: re-parsing its serialization yields the same cue count.
func FuzzParseSRT(f *testing.F) {
	f.Add("1\n00:00:01,000 --> 00:00:02,000\nhello")
	f.Add("\ufeff1\r\n00:00:01,000 --> 00:00:02,500\r\n<b>hi</b>\r\n\r\n")
	f.Add("garbage\nno arrow here\n\n")
	f.Add("1\n00:00:01,000 --> bad\nx")

	f.Fuzz(func(t *testing.T, content string) {
		track, err := ParseSRT(content)
		if err != nil {
			return
		}
		out := track.SRT()
		reparsed, err := ParseSRT(out)
		if err != nil {
			t.Fatalf("serialized SRT failed to re-parse: %v\ninput=%q\nout=%q", err, content, out)
		}
		if len(reparsed.Entries) != len(track.Entries) {
			t.Fatalf("cue count changed on round-trip: %d -> %d", len(track.Entries), len(reparsed.Entries))
		}
	})
}

// FuzzParseVTT asserts the WebVTT parser never panics and never emits
// angle-bracket markup in decoded cue text.
func FuzzParseVTT(f *testing.F) {
	f.Add("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nhello")
	f.Add("WEBVTT\n\n01:02.500 --> 01:03.000 align:start\n<c>&amp; hi</c>")
	f.Add("WEBVTT\n\nNOTE x\n\nbad --> 00:00:02.000\nx")

	f.Fuzz(func(t *testing.T, content string) {
		track, err := ParseVTT(content)
		if err != nil {
			return
		}
		for _, e := range track.Entries {
			if strings.ContainsAny(e.Text, "<>") {
				t.Fatalf("tags not stripped from cue text: %q", e.Text)
			}
		}
	})
}
