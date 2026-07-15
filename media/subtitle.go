package media

import (
	"errors"
	"fmt"
	"html"
	"strings"
	"time"
)

// ErrInvalidSubtitle is returned when subtitle content contains an unparseable
// timestamp on an otherwise well-formed cue line.
var ErrInvalidSubtitle = errors.New("media: invalid subtitle timestamp")

// SubtitleEntry is a single timed subtitle cue.
type SubtitleEntry struct {
	Range TimeRange `json:"range"`
	Text  string    `json:"text"`
}

// SubtitleTrack is an ordered collection of subtitle cues. It is the light-kit
// parallel of rskit's SubtitleTrack, covering the pure-Go concerns — parsing,
// serialization, and time math — without renderer-specific styling.
type SubtitleTrack struct {
	Entries  []SubtitleEntry `json:"entries"`
	Language string          `json:"language,omitempty"` // BCP 47 tag, optional
}

// Add appends a cue and returns the track for chaining.
func (t SubtitleTrack) Add(r TimeRange, text string) SubtitleTrack {
	t.Entries = append(t.Entries, SubtitleEntry{Range: r, Text: text})
	return t
}

// WithLanguage sets the track language (BCP 47 tag) and returns the track.
func (t SubtitleTrack) WithLanguage(lang string) SubtitleTrack {
	t.Language = lang
	return t
}

// ParseSRT parses SubRip (SRT) subtitle content.
//
// It tolerates common malformations: a UTF-8 BOM, Windows or Unix line endings,
// extra blank lines between cues, missing or non-numeric sequence numbers, and
// inline HTML tags (which are stripped). Blocks without a timestamp line are
// skipped; a malformed timestamp on a cue line returns [ErrInvalidSubtitle].
func ParseSRT(content string) (SubtitleTrack, error) {
	return parseCues(content, false)
}

// ParseVTT parses WebVTT subtitle content.
//
// In addition to the tolerances of [ParseSRT], it drops the leading WEBVTT
// header, ignores cue settings after the end timestamp, and decodes HTML
// entities in cue text.
func ParseVTT(content string) (SubtitleTrack, error) {
	content = strings.TrimSpace(strings.TrimPrefix(normalizeSubtitle(content), "WEBVTT"))
	return parseCues(content, true)
}

func parseCues(content string, vtt bool) (SubtitleTrack, error) {
	var track SubtitleTrack
	for _, block := range strings.Split(normalizeSubtitle(content), "\n\n") {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		lines := strings.Split(block, "\n")
		idx := timeLineIndex(lines)
		if idx < 0 {
			continue
		}
		parts := strings.SplitN(lines[idx], " --> ", 2)
		if len(parts) != 2 {
			continue
		}
		start, ok := parseCueTime(firstField(parts[0]))
		if !ok {
			return SubtitleTrack{}, fmt.Errorf("%w: %q", ErrInvalidSubtitle, parts[0])
		}
		end, ok := parseCueTime(firstField(parts[1]))
		if !ok {
			return SubtitleTrack{}, fmt.Errorf("%w: %q", ErrInvalidSubtitle, parts[1])
		}
		textLines := lines[idx+1:]
		if len(textLines) == 0 {
			continue
		}
		text := stripTags(strings.Join(textLines, "\n"))
		if vtt {
			text = html.UnescapeString(text)
		}
		if strings.TrimSpace(text) == "" {
			continue
		}
		track.Entries = append(track.Entries, SubtitleEntry{
			Range: NewTimeRange(start, end),
			Text:  text,
		})
	}
	return track, nil
}

// SRT serializes the track to SubRip (SRT) format.
func (t SubtitleTrack) SRT() string {
	var b strings.Builder
	for i, e := range t.Entries {
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n",
			i+1, formatClock(e.Range.Start, ','), formatClock(e.Range.End, ','), e.Text)
	}
	return b.String()
}

// VTT serializes the track to WebVTT format.
func (t SubtitleTrack) VTT() string {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for _, e := range t.Entries {
		fmt.Fprintf(&b, "%s --> %s\n%s\n\n",
			formatClock(e.Range.Start, '.'), formatClock(e.Range.End, '.'), e.Text)
	}
	return b.String()
}

// Shift moves every cue by a signed offset, clamping each bound at zero.
func (t *SubtitleTrack) Shift(offset time.Duration) {
	for i := range t.Entries {
		t.Entries[i].Range = t.Entries[i].Range.Shift(offset)
	}
}

// InRange returns a new track containing only the cues overlapping r.
func (t SubtitleTrack) InRange(r TimeRange) SubtitleTrack {
	out := SubtitleTrack{Language: t.Language}
	for _, e := range t.Entries {
		if e.Range.Overlaps(r) {
			out.Entries = append(out.Entries, e)
		}
	}
	return out
}

func normalizeSubtitle(s string) string {
	return strings.ReplaceAll(strings.TrimPrefix(s, "\ufeff"), "\r\n", "\n")
}

func timeLineIndex(lines []string) int {
	for i, l := range lines {
		if strings.Contains(l, " --> ") {
			return i
		}
	}
	return -1
}

func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}

// parseCueTime parses "HH:MM:SS.mmm", "HH:MM:SS,mmm", or "MM:SS.mmm" into a
// [Timestamp]. SRT uses a comma as the fractional separator; VTT uses a dot.
func parseCueTime(s string) (Timestamp, bool) {
	s = strings.Replace(s, ",", ".", 1)
	main, frac := s, int64(0)
	if dot := strings.IndexByte(s, '.'); dot >= 0 {
		var ok bool
		if frac, ok = atoi(s[dot+1:]); !ok {
			return 0, false
		}
		main = s[:dot]
	}
	parts := strings.Split(main, ":")
	var h, m, sec int64
	switch len(parts) {
	case 3:
		h, m, sec = mustAtoi(parts[0]), mustAtoi(parts[1]), mustAtoi(parts[2])
	case 2:
		m, sec = mustAtoi(parts[0]), mustAtoi(parts[1])
	default:
		return 0, false
	}
	if h < 0 || m < 0 || sec < 0 {
		return 0, false
	}
	return TimestampFromMillis(h*3_600_000 + m*60_000 + sec*1000 + frac), true
}

func atoi(s string) (int64, bool) {
	if s == "" {
		return 0, false
	}
	var n int64
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, false
		}
		n = n*10 + int64(c-'0')
	}
	return n, true
}

// mustAtoi returns -1 on failure so callers can reject the whole timestamp.
func mustAtoi(s string) int64 {
	n, ok := atoi(s)
	if !ok {
		return -1
	}
	return n
}

// formatClock renders a timestamp as "HH:MM:SS<sep>mmm".
func formatClock(ts Timestamp, sep byte) string {
	ms := ts.Millis()
	millis := ms % 1000
	totalSecs := ms / 1000
	secs := totalSecs % 60
	totalMins := totalSecs / 60
	mins := totalMins % 60
	hours := totalMins / 60
	return fmt.Sprintf("%02d:%02d:%02d%c%03d", hours, mins, secs, sep, millis)
}

// stripTags removes angle-bracket markup (e.g. <b>, <i>, <c>, <v Bob>).
func stripTags(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inTag := false
	for _, ch := range s {
		switch {
		case ch == '<':
			inTag = true
		case ch == '>':
			inTag = false
		case !inTag:
			b.WriteRune(ch)
		}
	}
	return b.String()
}
