// Package fsutil provides filesystem abstractions for testability.
package fsutil

import (
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// FileSystem abstracts filesystem operations for testability.
// Use OSFileSystem for production; MemoryFileSystem for testing.
type FileSystem interface {
	// Open opens the named file for reading.
	Open(name string) (fs.File, error)

	// Create creates or truncates the named file.
	Create(name string) (io.WriteCloser, error)

	// ReadFile reads the named file and returns its contents.
	ReadFile(name string) ([]byte, error)

	// WriteFile writes data to the named file, creating it if necessary.
	WriteFile(name string, data []byte, perm os.FileMode) error

	// Stat returns a FileInfo describing the named file.
	Stat(name string) (fs.FileInfo, error)

	// MkdirAll creates a directory and all necessary parents.
	MkdirAll(path string, perm os.FileMode) error

	// Remove removes the named file or empty directory.
	Remove(name string) error

	// RemoveAll removes path and any children it contains.
	RemoveAll(path string) error

	// Exists checks if a file or directory exists.
	Exists(name string) bool
}

// OSFileSystem implements FileSystem using the os package.
type OSFileSystem struct{}

// Open opens the named file.
func (OSFileSystem) Open(name string) (fs.File, error) {
	return os.Open(name)
}

// Create creates the named file.
func (OSFileSystem) Create(name string) (io.WriteCloser, error) {
	return os.Create(name)
}

// ReadFile reads the named file.
func (OSFileSystem) ReadFile(name string) ([]byte, error) {
	return os.ReadFile(name)
}

// WriteFile writes data to the named file.
func (OSFileSystem) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

// Stat returns file info for the named file.
func (OSFileSystem) Stat(name string) (fs.FileInfo, error) {
	return os.Stat(name)
}

// MkdirAll creates a directory path.
func (OSFileSystem) MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// Remove removes the named file or directory.
func (OSFileSystem) Remove(name string) error {
	return os.Remove(name)
}

// RemoveAll removes the path and any children.
func (OSFileSystem) RemoveAll(path string) error {
	return os.RemoveAll(path)
}

// Exists checks if a file exists.
func (OSFileSystem) Exists(name string) bool {
	_, err := os.Stat(name)
	return err == nil
}

// MemoryFileSystem provides an in-memory filesystem for testing.
type MemoryFileSystem struct {
	mu    sync.RWMutex
	files map[string]*memFile
	dirs  map[string]bool
}

type memFile struct {
	data    []byte
	mode    os.FileMode
	modTime int64
}

// NewMemoryFileSystem creates a new in-memory filesystem.
func NewMemoryFileSystem() *MemoryFileSystem {
	return &MemoryFileSystem{
		files: make(map[string]*memFile),
		dirs:  make(map[string]bool),
	}
}

// Open opens a file for reading.
func (m *MemoryFileSystem) Open(name string) (fs.File, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	name = filepath.Clean(name)
	f, ok := m.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrNotExist}
	}

	return &memFileReader{
		name: name,
		data: f.data,
	}, nil
}

// Create creates or truncates a file.
func (m *MemoryFileSystem) Create(name string) (io.WriteCloser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	name = filepath.Clean(name)
	m.files[name] = &memFile{data: []byte{}, mode: 0644}

	return &memFileWriter{
		fs:   m,
		name: name,
	}, nil
}

// ReadFile reads a file's contents.
func (m *MemoryFileSystem) ReadFile(name string) ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	name = filepath.Clean(name)
	f, ok := m.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "read", Path: name, Err: fs.ErrNotExist}
	}

	result := make([]byte, len(f.data))
	copy(result, f.data)
	return result, nil
}

// WriteFile writes data to a file.
func (m *MemoryFileSystem) WriteFile(name string, data []byte, perm os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name = filepath.Clean(name)
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)
	m.files[name] = &memFile{data: dataCopy, mode: perm}

	return nil
}

// Stat returns file info.
func (m *MemoryFileSystem) Stat(name string) (fs.FileInfo, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	name = filepath.Clean(name)

	if m.dirs[name] {
		return &memFileInfo{name: filepath.Base(name), isDir: true}, nil
	}

	f, ok := m.files[name]
	if !ok {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrNotExist}
	}

	return &memFileInfo{
		name: filepath.Base(name),
		size: int64(len(f.data)),
		mode: f.mode,
	}, nil
}

// MkdirAll creates directories.
func (m *MemoryFileSystem) MkdirAll(path string, perm os.FileMode) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path = filepath.Clean(path)
	m.dirs[path] = true

	// Create parent directories
	for p := filepath.Dir(path); p != "." && p != "/" && p != path; p = filepath.Dir(p) {
		m.dirs[p] = true
	}

	return nil
}

// Remove removes a file or empty directory.
func (m *MemoryFileSystem) Remove(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	name = filepath.Clean(name)

	if _, ok := m.files[name]; ok {
		delete(m.files, name)
		return nil
	}

	if m.dirs[name] {
		delete(m.dirs, name)
		return nil
	}

	return &fs.PathError{Op: "remove", Path: name, Err: fs.ErrNotExist}
}

// RemoveAll removes a path and children.
func (m *MemoryFileSystem) RemoveAll(path string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	path = filepath.Clean(path)

	// Remove matching files
	for name := range m.files {
		if name == path || hasPrefix(name, path+"/") {
			delete(m.files, name)
		}
	}

	// Remove matching directories
	for name := range m.dirs {
		if name == path || hasPrefix(name, path+"/") {
			delete(m.dirs, name)
		}
	}

	return nil
}

// Exists checks if a file or directory exists.
func (m *MemoryFileSystem) Exists(name string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	name = filepath.Clean(name)

	if _, ok := m.files[name]; ok {
		return true
	}

	return m.dirs[name]
}

// hasPrefix checks if s has the given prefix.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// memFileReader implements fs.File for reading.
type memFileReader struct {
	name   string
	data   []byte
	offset int
}

func (f *memFileReader) Read(p []byte) (int, error) {
	if f.offset >= len(f.data) {
		return 0, io.EOF
	}

	n := copy(p, f.data[f.offset:])
	f.offset += n
	return n, nil
}

func (f *memFileReader) Close() error { return nil }

func (f *memFileReader) Stat() (fs.FileInfo, error) {
	return &memFileInfo{name: filepath.Base(f.name), size: int64(len(f.data))}, nil
}

// memFileWriter implements io.WriteCloser for writing.
type memFileWriter struct {
	fs   *MemoryFileSystem
	name string
	buf  []byte
}

func (f *memFileWriter) Write(p []byte) (int, error) {
	f.buf = append(f.buf, p...)
	return len(p), nil
}

func (f *memFileWriter) Close() error {
	f.fs.mu.Lock()
	defer f.fs.mu.Unlock()

	if existing, ok := f.fs.files[f.name]; ok {
		existing.data = f.buf
	} else {
		f.fs.files[f.name] = &memFile{data: f.buf, mode: 0644}
	}

	return nil
}

// memFileInfo implements fs.FileInfo.
type memFileInfo struct {
	name  string
	size  int64
	mode  os.FileMode
	isDir bool
}

func (i *memFileInfo) Name() string       { return i.name }
func (i *memFileInfo) Size() int64        { return i.size }
func (i *memFileInfo) Mode() os.FileMode  { return i.mode }
func (i *memFileInfo) ModTime() time.Time { return time.Time{} }
func (i *memFileInfo) IsDir() bool        { return i.isDir }
func (i *memFileInfo) Sys() any           { return nil }
