package chart

import (
	"fmt"
	"image/color"
)

const (
	ColourP50       = "#fbd92f" // Yellow — 50th percentile
	ColourP85       = "#f7b32b" // Orange — 85th percentile
	ColourP98       = "#f25f5c" // Red/Pink — 98th percentile
	ColourMax       = "#2d1e2f" // Dark purple — maximum
	ColourCountBar  = "#2d1e2f" // Count bars
	ColourLowSample = "#f7b32b" // Low-sample period highlight
	ColourSteelBlue = "#4682b4" // Histogram bars
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

func hex(c color.RGBA) string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}
