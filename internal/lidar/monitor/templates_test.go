package monitor

import (
	"bytes"
	"io/fs"
	"testing"
)

func TestMockTemplateProvider_GetTemplate(t *testing.T) {
	provider := NewMockTemplateProvider(map[string]string{
		"test.html": "<h1>{{.Title}}</h1>",
	})

	tmpl, err := provider.GetTemplate("test.html")
	if err != nil {
		t.Fatalf("GetTemplate failed: %v", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]string{"Title": "Hello"}); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	expected := "<h1>Hello</h1>"
	if buf.String() != expected {
		t.Errorf("got %q, want %q", buf.String(), expected)
	}
}

func TestMockTemplateProvider_GetTemplate_NotFound(t *testing.T) {
	provider := NewMockTemplateProvider(map[string]string{})

	_, err := provider.GetTemplate("nonexistent.html")
	if err == nil {
		t.Error("expected error for nonexistent template")
	}
	if err != fs.ErrNotExist {
		t.Errorf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestMockTemplateProvider_GetTemplate_Error(t *testing.T) {
	provider := NewMockTemplateProvider(map[string]string{})
	provider.GetError = fs.ErrPermission

	_, err := provider.GetTemplate("any.html")
	if err != fs.ErrPermission {
		t.Errorf("expected fs.ErrPermission, got %v", err)
	}
}

func TestMockTemplateProvider_ExecuteTemplate(t *testing.T) {
	provider := NewMockTemplateProvider(map[string]string{
		"page.html": "Welcome {{.Name}}!",
	})

	var buf bytes.Buffer
	err := provider.ExecuteTemplate(&buf, "page.html", map[string]string{"Name": "User"})
	if err != nil {
		t.Fatalf("ExecuteTemplate failed: %v", err)
	}

	expected := "Welcome User!"
	if buf.String() != expected {
		t.Errorf("got %q, want %q", buf.String(), expected)
	}

	// Verify call was recorded
	if len(provider.ExecuteCalls) != 1 {
		t.Errorf("expected 1 call, got %d", len(provider.ExecuteCalls))
	}
	if provider.ExecuteCalls[0].Name != "page.html" {
		t.Errorf("expected name 'page.html', got %q", provider.ExecuteCalls[0].Name)
	}
}

func TestMockTemplateProvider_ExecuteTemplate_Error(t *testing.T) {
	provider := NewMockTemplateProvider(map[string]string{
		"page.html": "content",
	})
	provider.ExecuteError = fs.ErrClosed

	var buf bytes.Buffer
	err := provider.ExecuteTemplate(&buf, "page.html", nil)
	if err != fs.ErrClosed {
		t.Errorf("expected fs.ErrClosed, got %v", err)
	}
}

func TestMockAssetProvider_ReadFile(t *testing.T) {
	provider := NewMockAssetProvider(map[string][]byte{
		"style.css": []byte("body { color: red; }"),
	})

	content, err := provider.ReadFile("style.css")
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	expected := "body { color: red; }"
	if string(content) != expected {
		t.Errorf("got %q, want %q", string(content), expected)
	}
}

func TestMockAssetProvider_ReadFile_NotFound(t *testing.T) {
	provider := NewMockAssetProvider(map[string][]byte{})

	_, err := provider.ReadFile("nonexistent.css")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
	if err != fs.ErrNotExist {
		t.Errorf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestMockAssetProvider_ReadFile_Error(t *testing.T) {
	provider := NewMockAssetProvider(map[string][]byte{})
	provider.ReadError = fs.ErrPermission

	_, err := provider.ReadFile("any.css")
	if err != fs.ErrPermission {
		t.Errorf("expected fs.ErrPermission, got %v", err)
	}
}

func TestMockAssetProvider_Open_NotFound(t *testing.T) {
	provider := NewMockAssetProvider(map[string][]byte{})

	_, err := provider.Open("nonexistent.js")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestMockAssetProvider_Open_Error(t *testing.T) {
	provider := NewMockAssetProvider(map[string][]byte{})
	provider.OpenError = fs.ErrPermission

	_, err := provider.Open("any.js")
	if err != fs.ErrPermission {
		t.Errorf("expected fs.ErrPermission, got %v", err)
	}
}
