package report

import (
	"archive/zip"
	"bytes"
	"io"
	"maps"
	"os"
	"sort"
	"time"
)

var deterministicZipModified = time.Unix(0, 0).UTC()

// sortedKeys returns the keys of a string-keyed map in lexicographic
// order. Used to make ZIP entry order deterministic across runs.
func sortedKeys(m map[string][]byte) []string {
	names := make([]string, 0, len(m))
	for name := range m {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BuildZip creates a ZIP archive from the provided file map.
// Keys are zip-internal paths, values are file contents. Entries are
// written in lexicographic key order so the output is byte-identical
// between runs for the same input.
func BuildZip(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for _, name := range sortedKeys(files) {
		f, err := createDeterministicZipEntry(w, name)
		if err != nil {
			return nil, err
		}
		if _, err := f.Write(files[name]); err != nil {
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
	maps.Copy(remaining, files)

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

		w, err := createDeterministicZipEntry(writer, entry.Name)
		if err != nil {
			writer.Close()
			return nil, err
		}
		if _, err := w.Write(data); err != nil {
			writer.Close()
			return nil, err
		}
	}

	for _, name := range sortedKeys(remaining) {
		w, err := createDeterministicZipEntry(writer, name)
		if err != nil {
			writer.Close()
			return nil, err
		}
		if _, err := w.Write(remaining[name]); err != nil {
			writer.Close()
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func createDeterministicZipEntry(writer *zip.Writer, name string) (io.Writer, error) {
	header := &zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: deterministicZipModified,
	}
	return writer.CreateHeader(header)
}
