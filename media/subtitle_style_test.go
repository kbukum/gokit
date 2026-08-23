package media

import "testing"

func TestSubtitlePosition_DefaultAndCustom(t *testing.T) {
	t.Parallel()
	var zero SubtitlePosition
	if zero.Anchor != AnchorBottom {
		t.Errorf("zero position anchor = %v, want AnchorBottom", zero.Anchor)
	}
	if zero.Anchor.String() != "bottom" {
		t.Errorf("bottom string = %q", zero.Anchor.String())
	}
	pos := CustomPosition(12, 34)
	if pos.Anchor != AnchorCustom || pos.X != 12 || pos.Y != 34 {
		t.Errorf("custom position = %+v", pos)
	}
	if pos.Anchor.String() != "custom" {
		t.Errorf("custom string = %q", pos.Anchor.String())
	}
	if AnchorTop.String() != "top" || AnchorCenter.String() != "center" {
		t.Error("anchor String mismatch for top/center")
	}
}

func TestSubtitleTrack_EffectiveStyleResolution(t *testing.T) {
	t.Parallel()
	entryStyle := SubtitleStyle{Bold: true, Color: "#FF0000"}
	defaultStyle := SubtitleStyle{Italic: true, Position: CustomPosition(0, 10)}

	track := SubtitleTrack{}.
		WithLanguage("en").
		WithDefaultStyle(defaultStyle).
		AddStyled(TimeRangeFromMillis(0, 1000), "styled", entryStyle).
		Add(TimeRangeFromMillis(1000, 2000), "inherits")

	if track.DefaultStyle == nil || !track.DefaultStyle.Italic {
		t.Fatalf("default style not applied: %+v", track.DefaultStyle)
	}

	// Entry 0 carries its own style.
	got, ok := track.EffectiveStyle(0)
	if !ok || !got.Bold || got.Color != "#FF0000" {
		t.Errorf("entry 0 effective style = %+v ok=%v, want its own style", got, ok)
	}
	if track.Entries[0].Style == nil {
		t.Error("AddStyled must set the per-entry style")
	}

	// Entry 1 has no own style: it inherits the track default.
	got, ok = track.EffectiveStyle(1)
	if !ok || !got.Italic || got.Position.Anchor != AnchorCustom {
		t.Errorf("entry 1 effective style = %+v ok=%v, want inherited default", got, ok)
	}
	if track.Entries[1].Style != nil {
		t.Error("Add must not set a per-entry style")
	}

	// Out-of-range index fails closed rather than panicking.
	if _, ok := track.EffectiveStyle(99); ok {
		t.Error("out-of-range index must report no style")
	}
}

func TestSubtitleTrack_EffectiveStyleNoStyleReturnsFalse(t *testing.T) {
	t.Parallel()
	track := SubtitleTrack{}.Add(TimeRangeFromMillis(0, 1000), "plain")
	if style, ok := track.EffectiveStyle(0); ok || style != (SubtitleStyle{}) {
		t.Errorf("plain cue effective style = %+v ok=%v, want zero/false", style, ok)
	}
}

func TestSubtitleTrack_InRangePreservesDefaultStyle(t *testing.T) {
	t.Parallel()
	track := SubtitleTrack{}.
		WithDefaultStyle(SubtitleStyle{Bold: true}).
		Add(TimeRangeFromMillis(0, 1000), "a")
	sub := track.InRange(TimeRangeFromMillis(0, 500))
	if sub.DefaultStyle == nil || !sub.DefaultStyle.Bold {
		t.Errorf("InRange dropped the default style: %+v", sub.DefaultStyle)
	}
}

func TestSubtitleTrack_AddStyledDoesNotMutateReceiver(t *testing.T) {
	t.Parallel()
	base := SubtitleTrack{}.Add(TimeRangeFromMillis(0, 1000), "a")
	b := base.AddStyled(TimeRangeFromMillis(1000, 2000), "b", SubtitleStyle{Bold: true})
	c := base.AddStyled(TimeRangeFromMillis(2000, 3000), "c", SubtitleStyle{Italic: true})
	if len(base.Entries) != 1 {
		t.Fatalf("base mutated: len = %d, want 1", len(base.Entries))
	}
	if b.Entries[1].Text != "b" || c.Entries[1].Text != "c" {
		t.Errorf("styled appends shared backing array: b=%q c=%q", b.Entries[1].Text, c.Entries[1].Text)
	}
}
