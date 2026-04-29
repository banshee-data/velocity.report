package assets

import (
	_ "embed"
	"encoding/base64"
	"sync"
)

//go:embed AtkinsonHyperlegible-Regular.ttf
var fontRegular []byte

//go:embed AtkinsonHyperlegible-Bold.ttf
var fontBold []byte

//go:embed AtkinsonHyperlegible-Italic.ttf
var fontItalic []byte

//go:embed AtkinsonHyperlegible-BoldItalic.ttf
var fontBoldItalic []byte

//go:embed AtkinsonHyperlegibleMono-VariableFont_wght.ttf
var fontMono []byte

//go:embed AtkinsonHyperlegibleMono-Italic-VariableFont_wght.ttf
var fontMonoItalic []byte

var (
	atkinsonRegularB64  string
	atkinsonRegularOnce sync.Once
)

// AtkinsonRegularBase64 returns the base64-encoded Atkinson Hyperlegible
// Regular font, cached after first use.
func AtkinsonRegularBase64() string {
	atkinsonRegularOnce.Do(func() {
		atkinsonRegularB64 = base64.StdEncoding.EncodeToString(fontRegular)
	})
	return atkinsonRegularB64
}

// AllFonts returns the canonical mapping of packaged Atkinson font filenames
// to font bytes used by report generation and source ZIP packaging.
func AllFonts() map[string][]byte {
	return map[string][]byte{
		"AtkinsonHyperlegible-Regular.ttf":                      fontRegular,
		"AtkinsonHyperlegible-Bold.ttf":                         fontBold,
		"AtkinsonHyperlegible-Italic.ttf":                       fontItalic,
		"AtkinsonHyperlegible-BoldItalic.ttf":                   fontBoldItalic,
		"AtkinsonHyperlegibleMono-VariableFont_wght.ttf":        fontMono,
		"AtkinsonHyperlegibleMono-Italic-VariableFont_wght.ttf": fontMonoItalic,
	}
}
