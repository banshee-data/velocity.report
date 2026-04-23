package report

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
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

// AppendFilesToZip merges the provided files into an existing ZIP archive.
// Existing entries with the same name are replaced.
func AppendFilesToZip(zipPath string, files map[string][]byte) error {
	original, err := os.ReadFile(zipPath)
	if err != nil {
		return err
	}

	merged, err := appendZipBytes(original, files)
	if err != nil {
		return err
	}

	return os.WriteFile(zipPath, merged, 0644)
}

func appendZipBytes(original []byte, files map[string][]byte) ([]byte, error) {
	reader, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		return nil, err
	}

	remaining := make(map[string][]byte, len(files))
	for name, data := range files {
		remaining[name] = data
	}

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)

	for _, entry := range reader.File {
		data, ok := remaining[entry.Name]
		if ok {
			delete(remaining, entry.Name)
		} else {
			rc, err := entry.Open()
			if err != nil {
				writer.Close()
				return nil, err
			}
			data, err = io.ReadAll(rc)
			rc.Close()
			if err != nil {
				writer.Close()
				return nil, err
			}
		}

		w, err := writer.Create(entry.Name)
		if err != nil {
			writer.Close()
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			writer.Close()
			return nil, err
		}
	}

	for name, data := range remaining {
		w, err := writer.Create(name)
		if err != nil {
			writer.Close()
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			writer.Close()
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}
