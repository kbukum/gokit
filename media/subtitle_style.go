package media

import "slices"

// SubtitleAnchor is the reference point a cue is drawn from.
// Its zero value is [AnchorBottom], the conventional subtitle placement,
// so an unset [SubtitlePosition] defaults to the bottom of the frame.
type SubtitleAnchor int

const (
	// AnchorBottom draws the cue at the bottom of the frame (default).
	AnchorBottom SubtitleAnchor = iota
	// AnchorTop draws the cue at the top of the frame.
	AnchorTop
	// AnchorCenter draws the cue at the vertical center of the frame.
	AnchorCenter
	// AnchorCustom draws the cue at the pixel coordinates carried by [SubtitlePosition].
	AnchorCustom
)

// String returns the lowercase name of the anchor, or "unknown" for a value
// outside the defined set (e.g. a malformed JSON payload or bad cast).
func (a SubtitleAnchor) String() string {
	switch a {
	case AnchorBottom:
		return "bottom"
	case AnchorTop:
		return "top"
	case AnchorCenter:
		return "center"
	case AnchorCustom:
		return "custom"
	default:
		return "unknown"
	}
}

// SubtitlePosition is where a cue is displayed on screen.
//
// It is the light-kit parallel of rskit's SubtitlePosition. The zero value is the
// bottom anchor; X and Y are pixel coordinates that apply only when Anchor is
// [AnchorCustom] (they are ignored otherwise).
type SubtitlePosition struct {
	Anchor SubtitleAnchor `json:"anchor"`
	X      uint32         `json:"x,omitempty"`
	Y      uint32         `json:"y,omitempty"`
}

// CustomPosition returns a position anchored at the given pixel coordinates.
func CustomPosition(x, y uint32) SubtitlePosition {
	return SubtitlePosition{Anchor: AnchorCustom, X: x, Y: y}
}

// SubtitleStyle is the visual styling applied to a cue.
//
// It is the light-kit parallel of rskit's SubtitleStyle: it carries the styling
// vocabulary (font, colors, weight, position) without a renderer. Optional fields
// use their zero value to mean "unset" — an empty font family or color and a zero
// font size are renderer defaults. Style resolution ([SubtitleTrack.EffectiveStyle])
// selects one whole style (the cue's own or the track default); it does not merge
// fields, so a cue that sets only [SubtitleStyle.Color] does not inherit the track's
// other settings.
type SubtitleStyle struct {
	FontFamily string           `json:"font_family,omitempty"`
	FontSize   uint16           `json:"font_size,omitempty"`  // points; 0 means unset
	Color      string           `json:"color,omitempty"`      // CSS color, e.g. "#FFFFFF"
	Background string           `json:"background,omitempty"` // CSS color
	Bold       bool             `json:"bold,omitempty"`
	Italic     bool             `json:"italic,omitempty"`
	Position   SubtitlePosition `json:"position"`
}

// WithDefaultStyle returns a copy of the track with a default style that applies
// to every cue lacking its own [SubtitleEntry.Style]; the receiver is unchanged,
// so callers must use the returned value.
func (t SubtitleTrack) WithDefaultStyle(style SubtitleStyle) SubtitleTrack {
	t.DefaultStyle = &style
	return t
}

// AddStyled returns a copy of the track with a styled cue appended; the receiver
// is unchanged, so callers must use the returned value (supports chaining).
func (t SubtitleTrack) AddStyled(r TimeRange, text string, style SubtitleStyle) SubtitleTrack {
	entry := SubtitleEntry{Range: r, Text: text, Style: &style}
	t.Entries = append(slices.Clip(t.Entries), entry)
	return t
}

// EffectiveStyle resolves the style for the cue at index i: its own
// [SubtitleEntry.Style] if set, otherwise the track [SubtitleTrack.DefaultStyle],
// otherwise the zero style. The chosen style is returned whole — fields are never
// merged across the cue and track styles. The second result reports whether an
// explicit style (entry or default) was found. It returns the zero style and false
// for an out-of-range index rather than panicking.
func (t SubtitleTrack) EffectiveStyle(i int) (SubtitleStyle, bool) {
	if i < 0 || i >= len(t.Entries) {
		return SubtitleStyle{}, false
	}
	if s := t.Entries[i].Style; s != nil {
		return *s, true
	}
	if t.DefaultStyle != nil {
		return *t.DefaultStyle, true
	}
	return SubtitleStyle{}, false
}
