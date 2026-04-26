package chart

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"
)

func TestNewCanvas(t *testing.T) {
	c := NewCanvas(100, 50)
	data := c.Bytes()

	dec := xml.NewDecoder(bytes.NewReader(data))
	tok, err := dec.Token()
	if err != nil {
		t.Fatalf("xml parse error: %v", err)
	}
	se, ok := tok.(xml.StartElement)
	if !ok {
		t.Fatalf("expected StartElement, got %T", tok)
	}
	if se.Name.Local != "svg" {
		t.Errorf("root element = %q, want 'svg'", se.Name.Local)
	}

	var hasViewBox bool
	for _, a := range se.Attr {
		if a.Name.Local == "viewBox" {
			hasViewBox = true
			// Should contain pixel dimensions derived from 100mm × 50mm.
			if a.Value == "" {
				t.Error("viewBox is empty")
			}
		}
	}
	if !hasViewBox {
		t.Error("missing viewBox attribute")
	}
}

func TestCanvas_Rect(t *testing.T) {
	c := NewCanvas(50, 50)
	c.Rect(10, 20, 30, 40, `fill="red"`)
	data := string(c.Bytes())
	if !strings.Contains(data, "<rect") {
		t.Error("output missing <rect>")
	}
	if !strings.Contains(data, `fill="red"`) {
		t.Error("output missing fill attribute")
	}
}

func TestCanvas_Polyline(t *testing.T) {
	c := NewCanvas(50, 50)
	c.Polyline([][2]float64{{0, 0}, {10, 10}}, `stroke="blue"`)
	data := string(c.Bytes())
	if !strings.Contains(data, "<polyline") {
		t.Error("output missing <polyline>")
	}
}

func TestCanvas_Text_Escaping(t *testing.T) {
	c := NewCanvas(50, 50)
	c.Text(10, 20, "A & B < C", "")
	data := string(c.Bytes())
	if strings.Contains(data, "A & B") {
		t.Error("unescaped ampersand in text")
	}
	if !strings.Contains(data, "A &amp; B") {
		t.Error("missing escaped ampersand")
	}
}

func TestCanvas_EmbedFont(t *testing.T) {
	c := NewCanvas(50, 50)
	c.EmbedFont("TestFont", "AAAA")
	data := string(c.Bytes())
	if !strings.Contains(data, "@font-face") {
		t.Error("missing @font-face")
	}
	if !strings.Contains(data, "'TestFont'") {
		t.Error("missing font family name")
	}
}

func TestAtkinsonRegularBase64(t *testing.T) {
	b64 := AtkinsonRegularBase64()
	if b64 == "" {
		t.Error("AtkinsonRegularBase64 returned empty string")
	}
	if len(b64) < 100 {
		t.Error("base64 data suspiciously short")
	}
}
