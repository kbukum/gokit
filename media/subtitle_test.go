package media

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestParseSRT_Basic(t *testing.T) {
	t.Parallel()
	srt := "\ufeff1\r\n00:00:01,000 --> 00:00:02,500\r\n<b>Hello</b> world\r\n\r\n" +
		"2\n00:00:03,000 --> 00:00:04,000\nSecond line"
	track, err := ParseSRT(srt)
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(track.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(track.Entries))
	}
	if track.Entries[0].Text != "Hello world" {
		t.Errorf("text = %q, want stripped 'Hello world'", track.Entries[0].Text)
	}
	if track.Entries[0].Range != TimeRangeFromMillis(1000, 2500) {
		t.Errorf("range = %v", track.Entries[0].Range)
	}
}

func TestParseSRT_SkipsAndRejects(t *testing.T) {
	t.Parallel()
	// A block with no timestamp line is skipped; the valid block is kept.
	srt := "not a number\nno timestamp\n\n3\n00:00:01,000 --> 00:00:02,000\nok"
	track, err := ParseSRT(srt)
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(track.Entries) != 1 || track.Entries[0].Text != "ok" {
		t.Fatalf("track = %+v, want single 'ok' entry", track.Entries)
	}
	// Malformed timestamps are rejected.
	if _, err := ParseSRT("1\nbad --> 00:00:02,000\nx"); !errors.Is(err, ErrInvalidSubtitle) {
		t.Errorf("bad start: got %v", err)
	}
	if _, err := ParseSRT("1\n00:00:01,000 --> bad\nx"); !errors.Is(err, ErrInvalidSubtitle) {
		t.Errorf("bad end: got %v", err)
	}
}

func TestParseSRT_BlockWithoutText(t *testing.T) {
	t.Parallel()
	track, err := ParseSRT("1\n00:00:01,000 --> 00:00:02,000")
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(track.Entries) != 0 {
		t.Errorf("expected timestamp-only block to be skipped, got %+v", track.Entries)
	}
}

func TestParseVTT_Basic(t *testing.T) {
	t.Parallel()
	vtt := "WEBVTT\n\nNOTE ignored\n\ncue-id\n00:00:01.000 --> 00:00:02.000 align:start\n" +
		"<c>&amp; hi</c>\n\n01:02.500 --> 01:03.000\nshort form"
	track, err := ParseVTT(vtt)
	if err != nil {
		t.Fatalf("ParseVTT: %v", err)
	}
	if len(track.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(track.Entries))
	}
	if track.Entries[0].Text != "& hi" {
		t.Errorf("text = %q, want '& hi' (tags stripped, entity decoded)", track.Entries[0].Text)
	}
	if track.Entries[1].Range != TimeRangeFromMillis(62_500, 63_000) {
		t.Errorf("short-form range = %v, want 62500-63000", track.Entries[1].Range)
	}
}

func TestParseVTT_Rejects(t *testing.T) {
	t.Parallel()
	if _, err := ParseVTT("WEBVTT\n\nbad --> 00:00:02.000\nx"); !errors.Is(err, ErrInvalidSubtitle) {
		t.Errorf("bad start: got %v", err)
	}
	if _, err := ParseVTT("WEBVTT\n\n00:00:01.000 --> bad\nx"); !errors.Is(err, ErrInvalidSubtitle) {
		t.Errorf("bad end: got %v", err)
	}
}

func TestParseCueTime_MalformedVariants(t *testing.T) {
	t.Parallel()
	// Empty field before the arrow exercises firstField's empty path and a
	// rejected timestamp; trailing-dot, non-numeric, and too-few-colon forms
	// all fail closed.
	bad := []string{
		"1\n --> 00:00:02,000\nx",             // empty start field
		"1\n00:00:01. --> 00:00:02,000\nx",    // trailing separator, empty fraction
		"1\naa:00:01,000 --> 00:00:02,000\nx", // non-numeric hours
		"1\n01,000 --> 00:00:02,000\nx",       // too few colon-separated parts
	}
	for _, in := range bad {
		if _, err := ParseSRT(in); !errors.Is(err, ErrInvalidSubtitle) {
			t.Errorf("ParseSRT(%q) err = %v, want ErrInvalidSubtitle", in, err)
		}
	}
}

func TestParseCueTime_RejectsOverflowingField(t *testing.T) {
	t.Parallel()
	// A field large enough to overflow the int64-microsecond Timestamp must be
	// rejected rather than wrapping to a negative time (regression for a
	// fuzzer-found SRT round-trip break).
	if _, err := ParseSRT("1\n0:0 --> 1100000000000:0\n0"); !errors.Is(err, ErrInvalidSubtitle) {
		t.Errorf("overflowing cue time err = %v, want ErrInvalidSubtitle", err)
	}
}

func TestParseCues_NormalizesBlankAndTagOnlyLines(t *testing.T) {
	t.Parallel()
	// A tag-only first text line leaves a leading blank line that must not
	// re-serialize into a cue boundary (regression for a fuzzer-found round-trip
	// break). The cue text is normalized to its non-empty lines.
	track, err := ParseSRT("1\n00:00:00,000 --> 00:00:00,000\n<>\nkeep")
	if err != nil {
		t.Fatalf("ParseSRT: %v", err)
	}
	if len(track.Entries) != 1 || track.Entries[0].Text != "keep" {
		t.Fatalf("entries = %+v, want single 'keep'", track.Entries)
	}
	reparsed, err := ParseSRT(track.SRT())
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if len(reparsed.Entries) != len(track.Entries) {
		t.Errorf("round-trip cue count %d -> %d", len(track.Entries), len(reparsed.Entries))
	}
}

func TestVTT_EscapesSpecialCharsForRoundTrip(t *testing.T) {
	t.Parallel()
	// A literal '<' (e.g. from a decoded &#60; entity) must be re-escaped on
	// serialization so it survives a parse round-trip instead of being read as
	// tag markup (regression for a fuzzer-found VTT round-trip break).
	track := SubtitleTrack{}.Add(TimeRangeFromMillis(0, 1000), "a < b & c")
	vtt := track.VTT()
	if !strings.Contains(vtt, "&lt;") || !strings.Contains(vtt, "&amp;") {
		t.Errorf("VTT did not escape special chars: %q", vtt)
	}
	reparsed, err := ParseVTT(vtt)
	if err != nil {
		t.Fatalf("re-parse VTT: %v", err)
	}
	if len(reparsed.Entries) != 1 || reparsed.Entries[0].Text != "a < b & c" {
		t.Errorf("VTT round-trip = %+v, want unchanged text", reparsed.Entries)
	}
}

func TestSubtitleTrack_RoundTrip(t *testing.T) {
	t.Parallel()
	track := SubtitleTrack{}.
		WithLanguage("en").
		Add(TimeRangeFromMillis(1000, 2500), "hello").
		Add(TimeRangeFromMillis(3000, 4000), "world")
	if track.Language != "en" {
		t.Fatalf("WithLanguage not applied: %q", track.Language)
	}

	srt := track.SRT()
	reparsed, err := ParseSRT(srt)
	if err != nil {
		t.Fatalf("re-parse SRT: %v", err)
	}
	if len(reparsed.Entries) != 2 || reparsed.Entries[0].Text != "hello" {
		t.Errorf("SRT round-trip mismatch: %+v", reparsed.Entries)
	}
	if reparsed.Entries[0].Range != TimeRangeFromMillis(1000, 2500) {
		t.Errorf("SRT round-trip range = %v", reparsed.Entries[0].Range)
	}

	vtt := track.VTT()
	reparsedVTT, err := ParseVTT(vtt)
	if err != nil {
		t.Fatalf("re-parse VTT: %v", err)
	}
	if len(reparsedVTT.Entries) != 2 || reparsedVTT.Entries[1].Text != "world" {
		t.Errorf("VTT round-trip mismatch: %+v", reparsedVTT.Entries)
	}
}

func TestSubtitleTrack_SerializeFormats(t *testing.T) {
	t.Parallel()
	track := SubtitleTrack{}.Add(TimeRangeFromMillis(1000, 2500), "hi")
	if want := "1\n00:00:01,000 --> 00:00:02,500\nhi\n\n"; track.SRT() != want {
		t.Errorf("SRT = %q, want %q", track.SRT(), want)
	}
	if want := "WEBVTT\n\n00:00:01.000 --> 00:00:02.500\nhi\n\n"; track.VTT() != want {
		t.Errorf("VTT = %q, want %q", track.VTT(), want)
	}
}

func TestSubtitleTrack_ShiftAndInRange(t *testing.T) {
	t.Parallel()
	track := SubtitleTrack{}.
		Add(TimeRangeFromMillis(1000, 2000), "a").
		Add(TimeRangeFromMillis(5000, 6000), "b")
	track.Shift(-500 * time.Millisecond)
	if track.Entries[0].Range.Start != TimestampFromMillis(500) {
		t.Errorf("shift start = %v, want 500ms", track.Entries[0].Range.Start)
	}

	sub := track.InRange(TimeRangeFromMillis(0, 1000))
	if len(sub.Entries) != 1 || sub.Entries[0].Text != "a" {
		t.Errorf("InRange = %+v, want only 'a'", sub.Entries)
	}
	if sub.Language != track.Language {
		t.Error("InRange should preserve language")
	}
}

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

// FuzzParseVTT asserts the WebVTT parser never panics and that any track it
// produces round-trips: re-parsing its serialization yields the same cue count.
// Entity-decoded angle brackets are legitimate cue text, so the invariant is
// round-trip stability (which requires the serializer to re-escape), not the
// absence of angle brackets.
func FuzzParseVTT(f *testing.F) {
	f.Add("WEBVTT\n\n00:00:01.000 --> 00:00:02.000\nhello")
	f.Add("WEBVTT\n\n01:02.500 --> 01:03.000 align:start\n<c>&amp; hi</c>")
	f.Add("WEBVTT\n\nNOTE x\n\nbad --> 00:00:02.000\nx")

	f.Fuzz(func(t *testing.T, content string) {
		track, err := ParseVTT(content)
		if err != nil {
			return
		}
		reparsed, err := ParseVTT(track.VTT())
		if err != nil {
			t.Fatalf("serialized VTT failed to re-parse: %v\ninput=%q", err, content)
		}
		if len(reparsed.Entries) != len(track.Entries) {
			t.Fatalf("cue count changed on round-trip: %d -> %d", len(track.Entries), len(reparsed.Entries))
		}
	})
}
