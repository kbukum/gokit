package media

import (
	"fmt"
	"math"
)

// Resolution is a width × height in pixels. It is the light-kit parallel of
// rskit's media Resolution and is used to enrich probe [Metadata].
type Resolution struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

// NewResolution builds a [Resolution].
func NewResolution(w, h int) Resolution { return Resolution{Width: w, Height: h} }

// Common resolution presets.
func Resolution360p() Resolution  { return Resolution{640, 360} }
func Resolution480p() Resolution  { return Resolution{854, 480} }
func Resolution720p() Resolution  { return Resolution{1280, 720} }
func Resolution1080p() Resolution { return Resolution{1920, 1080} }
func Resolution1440p() Resolution { return Resolution{2560, 1440} }
func Resolution4K() Resolution    { return Resolution{3840, 2160} }

// AspectRatio returns the aspect ratio as a simplified integer fraction (e.g.
// 16, 9). It returns (0, 0) when either dimension is zero.
func (r Resolution) AspectRatio() (w, h int) {
	g := gcd(r.Width, r.Height)
	if g == 0 {
		return 0, 0
	}
	return r.Width / g, r.Height / g
}

// AspectRatioFloat returns width divided by height, or zero when height is zero.
func (r Resolution) AspectRatioFloat() float64 {
	if r.Height == 0 {
		return 0
	}
	return float64(r.Width) / float64(r.Height)
}

// IsPortrait reports whether the height exceeds the width.
func (r Resolution) IsPortrait() bool { return r.Height > r.Width }

// IsLandscape reports whether the width exceeds the height.
func (r Resolution) IsLandscape() bool { return r.Width > r.Height }

// IsSquare reports whether width equals height.
func (r Resolution) IsSquare() bool { return r.Width == r.Height }

// IsZero reports whether either dimension is unset.
func (r Resolution) IsZero() bool { return r.Width == 0 || r.Height == 0 }

// PixelCount returns the total number of pixels.
func (r Resolution) PixelCount() int64 { return int64(r.Width) * int64(r.Height) }

// ScaleToFit scales the resolution to fit within maxW × maxH while preserving
// aspect ratio (the result never exceeds either bound).
func (r Resolution) ScaleToFit(maxW, maxH int) Resolution {
	return r.scale(min(ratio(maxW, r.Width), ratio(maxH, r.Height)))
}

// ScaleToFill scales the resolution to fill w × h while preserving aspect ratio
// (the result covers the bounds and may exceed one of them).
func (r Resolution) ScaleToFill(w, h int) Resolution {
	return r.scale(max(ratio(w, r.Width), ratio(h, r.Height)))
}

// ScaleBy scales both dimensions by a factor (e.g. 0.5 for half size).
func (r Resolution) ScaleBy(factor float64) Resolution { return r.scale(factor) }

// String formats the resolution as "WxH".
func (r Resolution) String() string { return fmt.Sprintf("%dx%d", r.Width, r.Height) }

func (r Resolution) scale(factor float64) Resolution {
	return Resolution{Width: scaleDim(r.Width, factor), Height: scaleDim(r.Height, factor)}
}

// scaleDim multiplies a dimension by factor, rounds to nearest, and clamps the
// result to a valid non-negative int: NaN and negative results become zero and
// overflow saturates, so scaling never yields a negative or wrapped dimension.
func scaleDim(d int, factor float64) int {
	v := math.Round(float64(d) * factor)
	if !(v > 0) { // negative, zero, or NaN
		return 0
	}
	if v >= float64(math.MaxInt) {
		return math.MaxInt
	}
	return int(v)
}

func ratio(bound, dim int) float64 {
	if dim == 0 {
		return 0
	}
	return float64(bound) / float64(dim)
}

func gcd(a, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

// FrameRate is a rational frame rate (numerator / denominator) for exact
// representation of rates such as NTSC 29.97 fps.
type FrameRate struct {
	Num int `json:"num"`
	Den int `json:"den"`
}

// NewFrameRate builds a [FrameRate].
func NewFrameRate(num, den int) FrameRate { return FrameRate{Num: num, Den: den} }

// FPS builds an integer frame rate of n frames per second.
func FPS(n int) FrameRate { return FrameRate{Num: n, Den: 1} }

// Common frame-rate presets.
func FPS24() FrameRate  { return FrameRate{24, 1} }
func FPS25() FrameRate  { return FrameRate{25, 1} }
func FPS30() FrameRate  { return FrameRate{30, 1} }
func FPS50() FrameRate  { return FrameRate{50, 1} }
func FPS60() FrameRate  { return FrameRate{60, 1} }
func NTSC24() FrameRate { return FrameRate{24000, 1001} } // 23.976 fps
func NTSC30() FrameRate { return FrameRate{30000, 1001} } // 29.97 fps
func NTSC60() FrameRate { return FrameRate{60000, 1001} } // 59.94 fps

// Float returns the frame rate as a floating-point fps value, or zero when the
// denominator is zero.
func (f FrameRate) Float() float64 {
	if f.Den == 0 {
		return 0
	}
	return float64(f.Num) / float64(f.Den)
}
