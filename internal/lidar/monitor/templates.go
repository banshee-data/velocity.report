// Package monitor provides template abstractions for testability.
package monitor

import (
	"embed"
	"html/template"
	"io"
	"io/fs"
)

// TemplateProvider abstracts template loading and execution.
// Production uses EmbeddedTemplateProvider; tests use MockTemplateProvider.
type TemplateProvider interface {
	// GetTemplate returns a parsed template by name.
	GetTemplate(name string) (*template.Template, error)
	// ExecuteTemplate executes a template with the given data.
	ExecuteTemplate(w io.Writer, name string, data interface{}) error
}

// EmbeddedTemplateProvider loads templates from embedded filesystem.
type EmbeddedTemplateProvider struct {
	fs      embed.FS
	baseDir string
	cache   map[string]*template.Template
}

// NewEmbeddedTemplateProvider creates a provider with the given embedded FS.
func NewEmbeddedTemplateProvider(embedFS embed.FS, baseDir string) *EmbeddedTemplateProvider {
	return &EmbeddedTemplateProvider{
		fs:      embedFS,
		baseDir: baseDir,
		cache:   make(map[string]*template.Template),
	}
}

// GetTemplate parses and caches a template from the embedded FS.
func (p *EmbeddedTemplateProvider) GetTemplate(name string) (*template.Template, error) {
	if t, ok := p.cache[name]; ok {
		return t, nil
	}

	path := name
	if p.baseDir != "" {
		path = p.baseDir + "/" + name
	}

	content, err := p.fs.ReadFile(path)
	if err != nil {
		return nil, err
	}

	t, err := template.New(name).Parse(string(content))
	if err != nil {
		return nil, err
	}

	p.cache[name] = t
	return t, nil
}

// ExecuteTemplate loads and executes a template.
func (p *EmbeddedTemplateProvider) ExecuteTemplate(w io.Writer, name string, data interface{}) error {
	t, err := p.GetTemplate(name)
	if err != nil {
		return err
	}
	return t.Execute(w, data)
}

// MockTemplateProvider provides templates for testing.
type MockTemplateProvider struct {
	Templates    map[string]string
	ExecuteError error
	ExecuteCalls []executeCall
	GetError     error
}

type executeCall struct {
	Name string
	Data interface{}
}

// NewMockTemplateProvider creates a mock provider with predefined templates.
func NewMockTemplateProvider(templates map[string]string) *MockTemplateProvider {
	return &MockTemplateProvider{
		Templates:    templates,
		ExecuteCalls: []executeCall{},
	}
}

// GetTemplate returns a parsed template from the mock templates.
func (m *MockTemplateProvider) GetTemplate(name string) (*template.Template, error) {
	if m.GetError != nil {
		return nil, m.GetError
	}

	content, ok := m.Templates[name]
	if !ok {
		return nil, fs.ErrNotExist
	}

	return template.New(name).Parse(content)
}

// ExecuteTemplate records the call and executes the template.
func (m *MockTemplateProvider) ExecuteTemplate(w io.Writer, name string, data interface{}) error {
	m.ExecuteCalls = append(m.ExecuteCalls, executeCall{Name: name, Data: data})

	if m.ExecuteError != nil {
		return m.ExecuteError
	}

	t, err := m.GetTemplate(name)
	if err != nil {
		return err
	}

	return t.Execute(w, data)
}

// AssetProvider abstracts static asset serving for testability.
type AssetProvider interface {
	// Open opens a file from the asset store.
	Open(name string) (fs.File, error)
	// ReadFile reads an entire file.
	ReadFile(name string) ([]byte, error)
}

// EmbeddedAssetProvider serves assets from an embedded filesystem.
type EmbeddedAssetProvider struct {
	fs      embed.FS
	baseDir string
}

// NewEmbeddedAssetProvider creates a provider with the given embedded FS.
func NewEmbeddedAssetProvider(embedFS embed.FS, baseDir string) *EmbeddedAssetProvider {
	return &EmbeddedAssetProvider{
		fs:      embedFS,
		baseDir: baseDir,
	}
}

// Open opens a file from the embedded FS.
func (p *EmbeddedAssetProvider) Open(name string) (fs.File, error) {
	path := name
	if p.baseDir != "" {
		path = p.baseDir + "/" + name
	}
	return p.fs.Open(path)
}

// ReadFile reads an entire file from the embedded FS.
func (p *EmbeddedAssetProvider) ReadFile(name string) ([]byte, error) {
	path := name
	if p.baseDir != "" {
		path = p.baseDir + "/" + name
	}
	return p.fs.ReadFile(path)
}

// MockAssetProvider provides assets for testing.
type MockAssetProvider struct {
	Files     map[string][]byte
	OpenError error
	ReadError error
}

// NewMockAssetProvider creates a mock provider with predefined files.
func NewMockAssetProvider(files map[string][]byte) *MockAssetProvider {
	return &MockAssetProvider{Files: files}
}

// Open returns a mock file (not fully implemented - use ReadFile for testing).
func (m *MockAssetProvider) Open(name string) (fs.File, error) {
	if m.OpenError != nil {
		return nil, m.OpenError
	}
	if _, ok := m.Files[name]; !ok {
		return nil, fs.ErrNotExist
	}
	// Return a simple mock - for full fs.File implementation, use a real test FS
	return nil, fs.ErrNotExist // Simplified - tests should use ReadFile
}

// ReadFile returns the mock file content.
func (m *MockAssetProvider) ReadFile(name string) ([]byte, error) {
	if m.ReadError != nil {
		return nil, m.ReadError
	}
	content, ok := m.Files[name]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return content, nil
}
