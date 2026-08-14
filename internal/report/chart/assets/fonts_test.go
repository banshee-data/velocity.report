package assets

import (
	"encoding/base64"
	"testing"
)

func TestAllFontsReturnsEveryPackagedFace(t *testing.T) {
	fonts := AllFonts()

	want := []string{
		"AtkinsonHyperlegible-Regular.ttf",
		"AtkinsonHyperlegible-Bold.ttf",
		"AtkinsonHyperlegible-Italic.ttf",
		"AtkinsonHyperlegible-BoldItalic.ttf",
		"AtkinsonHyperlegibleMono-VariableFont_wght.ttf",
		"AtkinsonHyperlegibleMono-Italic-VariableFont_wght.ttf",
	}
	if len(fonts) != len(want) {
		t.Fatalf("got %d fonts, want %d", len(fonts), len(want))
	}
	for _, name := range want {
		data, ok := fonts[name]
		if !ok {
			t.Errorf("missing font %q", name)
			continue
		}
		// An empty entry means the //go:embed directive silently matched
		// nothing, which would produce blank text in generated reports.
		if len(data) == 0 {
			t.Errorf("font %q is empty, want embedded bytes", name)
		}
		// TrueType files start with the 0x00010000 sfnt version tag.
		if len(data) >= 4 && !(data[0] == 0x00 && data[1] == 0x01 && data[2] == 0x00 && data[3] == 0x00) {
			t.Errorf("font %q does not start with the TrueType sfnt tag: % x", name, data[:4])
		}
	}
}

func TestAtkinsonRegularBase64DecodesToTheEmbeddedFont(t *testing.T) {
	got := AtkinsonRegularBase64()
	if got == "" {
		t.Fatal("AtkinsonRegularBase64() is empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("decoding base64: %v", err)
	}
	regular := AllFonts()["AtkinsonHyperlegible-Regular.ttf"]
	if len(decoded) != len(regular) {
		t.Fatalf("decoded %d bytes, want %d (the embedded Regular face)", len(decoded), len(regular))
	}
}

func TestAtkinsonRegularBase64IsCachedAndStable(t *testing.T) {
	// The value is memoised behind a sync.Once; repeated calls must return
	// the same string rather than re-encoding or returning empty.
	first := AtkinsonRegularBase64()
	second := AtkinsonRegularBase64()
	if first != second {
		t.Error("AtkinsonRegularBase64() returned different values across calls")
	}
}
