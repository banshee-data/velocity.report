package chart

import (
	_ "embed"
	"encoding/base64"
	"fmt"
	"strings"
	"sync"
)

const pxPerMM = 96.0 / 25.4

//go:embed assets/AtkinsonHyperlegible-Regular.ttf
var atkinsonRegularTTF []byte

//go:embed assets/AtkinsonHyperlegible-Bold.ttf
var atkinsonBoldTTF []byte

//go:embed assets/AtkinsonHyperlegible-Italic.ttf
var atkinsonItalicTTF []byte

//go:embed assets/AtkinsonHyperlegible-BoldItalic.ttf
var atkinsonBoldItalicTTF []byte

var (
	atkinsonRegularB64  string
	atkinsonRegularOnce sync.Once
)

// AtkinsonRegularBase64 returns the base64-encoded Atkinson Hyperlegible Regular font.
func AtkinsonRegularBase64() string {
	atkinsonRegularOnce.Do(func() {
		atkinsonRegularB64 = base64.StdEncoding.EncodeToString(atkinsonRegularTTF)
	})
	return atkinsonRegularB64
}

// SVGCanvas is a low-level builder for SVG documents.
// All coordinates are in pixels; use pxPerMM to convert from millimetres.
type SVGCanvas struct {
	widthPx  float64
	heightPx float64
	buf      strings.Builder
	closed   bool
}

// NewCanvas creates an SVG canvas sized from millimetre dimensions.
// Both a physical width/height (in mm) and a pixel viewBox are emitted so
// rasterisers and LaTeX preserve the intended print size.
func NewCanvas(widthMM, heightMM float64) *SVGCanvas {
	wPx := widthMM * pxPerMM
	hPx := heightMM * pxPerMM
	c := &SVGCanvas{
		widthPx:  wPx,
		heightPx: hPx,
	}
	fmt.Fprintf(&c.buf,
		`<svg xmlns="http://www.w3.org/2000/svg" width="%.3fmm" height="%.3fmm" viewBox="0 0 %.4f %.4f">`,
		widthMM, heightMM, wPx, hPx)
	c.buf.WriteByte('\n')
	return c
}

// EmbedFont writes a @font-face declaration into <defs>.
func (c *SVGCanvas) EmbedFont(family, base64Data string) {
	fmt.Fprintf(&c.buf,
		"<defs><style>@font-face { font-family: '%s'; src: url('data:font/truetype;base64,%s') format('truetype'); }</style></defs>\n",
		family, base64Data)
}

// Rect emits an SVG <rect> element.
func (c *SVGCanvas) Rect(x, y, w, h float64, attrs string) {
	fmt.Fprintf(&c.buf,
		`<rect x="%.4f" y="%.4f" width="%.4f" height="%.4f"`,
		x, y, w, h)
	if attrs != "" {
		c.buf.WriteByte(' ')
		c.buf.WriteString(attrs)
	}
	c.buf.WriteString("/>\n")
}

// Polyline emits an SVG <polyline> element.
func (c *SVGCanvas) Polyline(points [][2]float64, attrs string) {
	if len(points) == 0 {
		return
	}
	c.buf.WriteString(`<polyline points="`)
	for i, p := range points {
		if i > 0 {
			c.buf.WriteByte(' ')
		}
		fmt.Fprintf(&c.buf, "%.4f,%.4f", p[0], p[1])
	}
	c.buf.WriteByte('"')
	if attrs != "" {
		c.buf.WriteByte(' ')
		c.buf.WriteString(attrs)
	}
	c.buf.WriteString("/>\n")
}

// Circle emits an SVG <circle> element.
func (c *SVGCanvas) Circle(cx, cy, r float64, attrs string) {
	fmt.Fprintf(&c.buf,
		`<circle cx="%.4f" cy="%.4f" r="%.4f"`,
		cx, cy, r)
	if attrs != "" {
		c.buf.WriteByte(' ')
		c.buf.WriteString(attrs)
	}
	c.buf.WriteString("/>\n")
}

// Line emits an SVG <line> element.
func (c *SVGCanvas) Line(x1, y1, x2, y2 float64, attrs string) {
	fmt.Fprintf(&c.buf,
		`<line x1="%.4f" y1="%.4f" x2="%.4f" y2="%.4f"`,
		x1, y1, x2, y2)
	if attrs != "" {
		c.buf.WriteByte(' ')
		c.buf.WriteString(attrs)
	}
	c.buf.WriteString("/>\n")
}

// Text emits an SVG <text> element. Content is XML-escaped.
func (c *SVGCanvas) Text(x, y float64, content string, attrs string) {
	fmt.Fprintf(&c.buf, `<text x="%.4f" y="%.4f"`, x, y)
	if attrs != "" {
		c.buf.WriteByte(' ')
		c.buf.WriteString(attrs)
	}
	c.buf.WriteByte('>')
	xmlEscape(&c.buf, content)
	c.buf.WriteString("</text>\n")
}

// BeginGroup opens a <g> element.
func (c *SVGCanvas) BeginGroup(attrs string) {
	c.buf.WriteString("<g")
	if attrs != "" {
		c.buf.WriteByte(' ')
		c.buf.WriteString(attrs)
	}
	c.buf.WriteString(">\n")
}

// EndGroup closes a </g> element.
func (c *SVGCanvas) EndGroup() {
	c.buf.WriteString("</g>\n")
}

// Bytes closes the SVG document and returns its content.
func (c *SVGCanvas) Bytes() []byte {
	if !c.closed {
		c.buf.WriteString("</svg>\n")
		c.closed = true
	}
	return []byte(c.buf.String())
}

// xmlEscape writes s to buf with XML-safe escaping for &, <, >, ", '.
func xmlEscape(buf *strings.Builder, s string) {
	for _, r := range s {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		case '"':
			buf.WriteString("&quot;")
		case '\'':
			buf.WriteString("&apos;")
		default:
			buf.WriteRune(r)
		}
	}
}
