package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func digest(raw []byte) string { s := sha256.Sum256(raw); return "sha256:" + hex.EncodeToString(s[:]) }

// atomicJSON syncs before rename, including the parent directory, so progress
// survives a power loss without a partial record being mistaken for success.
func atomicJSON(path string, v any) error {
	raw, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	return atomicFile(path, append(raw, '\n'))
}
func atomicFile(path string, raw []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".harness-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(raw); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), path); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}
func readJSON(path string, v any) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(raw, v); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
func outside(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && (rel == ".." || len(rel) > 3 && rel[:3] == ".."+string(filepath.Separator))
}

// archiveStage preserves failed databases on the evidence share before freeing
// VM scratch. Copy first, sync, then remove the source only after every file has
// arrived. A failed copy is safe to retry on the next scheduling pass.
func archiveStage(stage, destination string) error {
	if _, e := os.Stat(stage); os.IsNotExist(e) {
		return nil
	} else if e != nil {
		return e
	}
	err := filepath.WalkDir(stage, func(path string, d os.DirEntry, e error) error {
		if e != nil {
			return e
		}
		if d.IsDir() {
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing staging symlink %s", path)
		}
		rel, e := filepath.Rel(stage, path)
		if e != nil {
			return e
		}
		target := filepath.Join(destination, rel)
		if e = os.MkdirAll(filepath.Dir(target), 0755); e != nil {
			return e
		}
		input, e := os.Open(path)
		if e != nil {
			return e
		}
		defer input.Close()
		out, e := os.CreateTemp(filepath.Dir(target), ".stage-*")
		if e != nil {
			return e
		}
		defer os.Remove(out.Name())
		if _, e = io.Copy(out, input); e != nil {
			out.Close()
			return e
		}
		if e = out.Sync(); e != nil {
			out.Close()
			return e
		}
		if e = out.Close(); e != nil {
			return e
		}
		if e = os.Rename(out.Name(), target); e != nil {
			return e
		}
		dir, e := os.Open(filepath.Dir(target))
		if e != nil {
			return e
		}
		defer dir.Close()
		return dir.Sync()
	})
	if err != nil {
		return fmt.Errorf("archive staging %s to %s: %w", stage, destination, err)
	}
	return os.RemoveAll(stage)
}
