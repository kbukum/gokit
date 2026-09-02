package media

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTypeJSONLowercaseStrings(t *testing.T) {
	cases := map[Type]string{
		Unknown: "unknown",
		Video:   "video",
		Audio:   "audio",
		Image:   "image",
		Text:    "text",
	}
	for typ, want := range cases {
		data, err := json.Marshal(typ)
		if err != nil {
			t.Fatalf("marshal %v: %v", typ, err)
		}
		if string(data) != `"`+want+`"` {
			t.Errorf("Marshal(%v) = %s, want %q", typ, data, want)
		}
		var got Type
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("unmarshal %s: %v", data, err)
		}
		if got != typ {
			t.Errorf("round-trip %v -> %v", typ, got)
		}
	}
}

func TestInfoJSONNeverEmitsIntegerType(t *testing.T) {
	data, err := json.Marshal(Info{Type: Text, Format: FormatText, MimeType: "text/plain"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if !strings.Contains(string(data), `"type":"text"`) {
		t.Fatalf("type must serialize as lowercase string: %s", data)
	}
}

func TestSubtitleOmitsAbsentOptionalFields(t *testing.T) {
	track := SubtitleTrack{Entries: []SubtitleEntry{{Text: "hi"}}}
	data, err := json.Marshal(track)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(data)
	for _, absent := range []string{"language", "default_style", "style", "null"} {
		if strings.Contains(got, absent) {
			t.Fatalf("absent optional field %q must be omitted: %s", absent, got)
		}
	}
}
