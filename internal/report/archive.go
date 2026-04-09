package report

import (
	"archive/zip"
	"bytes"
)

// BuildZip creates a ZIP archive from the provided file map.
// Keys are zip-internal paths, values are file contents.
func BuildZip(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range files {
		f, err := w.Create(name)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(data); err != nil {
			return nil, err
		}
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
