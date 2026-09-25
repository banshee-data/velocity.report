package chart

import "image/color"

const (
	ColourP50       = "#fbd92f" // Yellow — 50th percentile
	ColourP85       = "#f7b32b" // Orange — 85th percentile
	ColourP98       = "#f25f5c" // Red/Pink — 98th percentile
	ColourMax       = "#2d1e2f" // Dark purple — maximum
	ColourCountBar  = "#2d1e2f" // Count bars
	ColourLowSample = "#f7b32b" // Low-sample period highlight
	ColourSteelBlue = "#4682b4" // Histogram bars
)

// Following-evidence colours (headway report). These are not the percentile
// palette: the charts show one pair's evidence or a time-weighted gap
// distribution, not a speed population, and a band threshold is a
// descriptive bin rather than an alarm, so nothing here uses the percentile
// reds. docs/ui/DESIGN.md records the pairing.
const (
	ColourFollowingObserved   = "#2d1e2f"       // observed spatial gap: the report's ink
	ColourFollowingTimeGap    = ColourSteelBlue // net time gap line and distribution bars
	ColourFollowingPredicted  = "#8a817c"       // review-only predicted gap: dashed, hollow markers
	ColourFollowingSuppressed = "#d8d3cf"       // suppressed intervals and excluded shares
	ColourFollowingThreshold  = "#4a4a4a"       // band rules: neutral, dashed
)

// Palette mirrors the typst prototype's RGBA palette for helpers that emit
// raw SVG without going through the existing chart style configuration.
var Palette = struct {
	P50, P85, P98, Max  color.RGBA
	CountBar, LowSample color.RGBA
}{
	P50:       color.RGBA{R: 0xfb, G: 0xd9, B: 0x2f, A: 0xff},
	P85:       color.RGBA{R: 0xf7, G: 0xb3, B: 0x2b, A: 0xff},
	P98:       color.RGBA{R: 0xf2, G: 0x5f, B: 0x5c, A: 0xff},
	Max:       color.RGBA{R: 0x2d, G: 0x1e, B: 0x2f, A: 0xff},
	CountBar:  color.RGBA{R: 0xa8, G: 0x9c, B: 0x95, A: 0xff},
	LowSample: color.RGBA{R: 0xf7, G: 0xb3, B: 0x2b, A: 0xff},
}
