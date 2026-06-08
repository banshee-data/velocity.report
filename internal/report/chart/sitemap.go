package chart

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"math"
)

// SiteMapOptions controls the site-map. When OSMTiles is explicitly enabled,
// we fetch real OpenStreetMap raster tiles centred on (Latitude, Longitude)
// and overlay the radar marker + coverage triangle. Otherwise we draw a
// schematic placeholder.
//
// PRIVACY: enabling OSMTiles makes outbound HTTP requests to a third-party
// tile server (tile.openstreetmap.org), disclosing the site coordinates
// off-device. Keep it off in local-only / privacy-sensitive code paths; the
// schematic fallback needs no network.
type SiteMapOptions struct {
	Latitude          float64
	Longitude         float64
	MapAngleDegrees   float64 // rotation of the coverage triangle, clockwise from north
	WidthPx, HeightPx int
	// OSMTiles fetches and embeds real OSM tiles. This is opt-in because it
	// sends the site coordinates to tile.openstreetmap.org (off-device network access).
	OSMTiles bool
	OSMZoom  int // zoom level for OSM (default 17)
}

// RenderSiteMap returns an SVG of the survey site. When opts.OSMTiles is
// true, real OpenStreetMap raster tiles are fetched and embedded as a
// base64 PNG inside the SVG; the radar marker and coverage triangle are
// drawn on top. When false (or when tile fetch fails), a schematic
// placeholder with a grid background is rendered instead.
func RenderSiteMap(opts SiteMapOptions) ([]byte, error) {
	if opts.WidthPx == 0 {
		opts.WidthPx = 600
	}
	if opts.HeightPx == 0 {
		opts.HeightPx = 600
	}
	if opts.OSMZoom == 0 {
		opts.OSMZoom = 17
	}

	cx := float64(opts.WidthPx) / 2
	cy := float64(opts.HeightPx) / 2

	// Try OSM tiles first; on failure, fall through to the schematic.
	var tilePNG []byte
	if opts.OSMTiles {
		fetcher := NewOSMTileFetcher()
		if data, err := fetcher.StitchTiles(opts.Latitude, opts.Longitude, opts.OSMZoom); err == nil {
			tilePNG = data
		}
	}

	// Coverage triangle: 20° apex at the marker, opening "north" (negative Y),
	// then rotated by MapAngleDegrees clockwise from north. The OPS243-A's
	// azimuth FoV is ~20°, matching pdf_generator/core/map_utils.py
	// RadarMarker.coverage_angle default.
	const apexHalfAngleDeg = 10.0
	rad := math.Pi / 180
	rot := opts.MapAngleDegrees * rad
	radius := 0.42 * math.Min(float64(opts.WidthPx), float64(opts.HeightPx))

	// Triangle vertices in canvas coordinates (Y axis points down).
	apex := [2]float64{cx, cy}
	left := [2]float64{
		cx + radius*math.Sin(rot-apexHalfAngleDeg*rad),
		cy - radius*math.Cos(rot-apexHalfAngleDeg*rad),
	}
	right := [2]float64{
		cx + radius*math.Sin(rot+apexHalfAngleDeg*rad),
		cy - radius*math.Cos(rot+apexHalfAngleDeg*rad),
	}

	var b bytes.Buffer
	fmt.Fprintf(&b, `<?xml version="1.0" encoding="UTF-8"?>`)
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" font-family="Atkinson Hyperlegible, sans-serif">`,
		opts.WidthPx, opts.HeightPx, opts.WidthPx, opts.HeightPx)

	if tilePNG != nil {
		// Embed real OSM tiles as a base64 PNG covering the entire canvas.
		enc := base64.StdEncoding.EncodeToString(tilePNG)
		fmt.Fprintf(&b, `<image x="0" y="0" width="%d" height="%d" preserveAspectRatio="xMidYMid slice" href="data:image/png;base64,%s"/>`,
			opts.WidthPx, opts.HeightPx, enc)
		// © OpenStreetMap contributors attribution (required by tile policy).
		fmt.Fprintf(&b, `<text x="%d" y="%d" font-size="9" text-anchor="end" fill="#222" stroke="#fff" stroke-width="2" paint-order="stroke">© OpenStreetMap contributors</text>`,
			opts.WidthPx-6, opts.HeightPx-6)
	} else {
		// Schematic fallback: muted "tile" gradient + grid.
		fmt.Fprintf(&b, `<defs>
<linearGradient id="bg" x1="0" y1="0" x2="0" y2="1">
  <stop offset="0%%" stop-color="#dfe7ec"/>
  <stop offset="100%%" stop-color="#c8d4dc"/>
</linearGradient>
<pattern id="grid" width="40" height="40" patternUnits="userSpaceOnUse">
  <path d="M40 0H0V40" fill="none" stroke="#a9b9c4" stroke-width="0.4"/>
</pattern>
</defs>`)
		fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="url(#bg)"/>`)
		fmt.Fprintf(&b, `<rect width="100%%" height="100%%" fill="url(#grid)"/>`)
	}

	// "North" indicator in the top-left corner.
	fmt.Fprintf(&b, `<g transform="translate(20, 20)">
<line x1="0" y1="20" x2="0" y2="0" stroke="#333" stroke-width="1.5"/>
<polygon points="-3,3 0,-3 3,3" fill="#333"/>
<text x="0" y="34" font-size="9" text-anchor="middle" fill="#333">N</text>
</g>`)

	// Coverage triangle (filled, semi-transparent).
	fmt.Fprintf(&b, `<polygon points="%.2f,%.2f %.2f,%.2f %.2f,%.2f" fill="%s" fill-opacity="0.32" stroke="%s" stroke-width="1.2"/>`,
		apex[0], apex[1], left[0], left[1], right[0], right[1],
		hex(Palette.P98), hex(Palette.P98))

	// Survey marker (white circle with red ring at the apex).
	fmt.Fprintf(&b, `<circle cx="%.2f" cy="%.2f" r="9" fill="#ffffff" stroke="%s" stroke-width="2.5"/>`,
		apex[0], apex[1], hex(Palette.P98))
	fmt.Fprintf(&b, `<circle cx="%.2f" cy="%.2f" r="2.5" fill="%s"/>`,
		apex[0], apex[1], hex(Palette.P98))

	// Coordinate label below the canvas.
	fmt.Fprintf(&b, `<text x="%.2f" y="%d" font-size="9" text-anchor="middle" fill="#222">%.4f, %.4f &#xb7; bearing %.0f&#xb0;</text>`,
		cx, opts.HeightPx-10, opts.Latitude, opts.Longitude, opts.MapAngleDegrees)

	fmt.Fprintf(&b, `</svg>`)
	return b.Bytes(), nil
}
