package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadTuningConfigOrEmbedded(t *testing.T) {
	// Round-trip the canonical defaults to stand in for the binary-embedded bytes.
	embedded, err := json.Marshal(MustLoadDefaultConfig())
	if err != nil {
		t.Fatalf("marshalling defaults: %v", err)
	}

	dir := t.TempDir()

	t.Run("absent path falls back to embedded", func(t *testing.T) {
		cfg, err := LoadTuningConfigOrEmbedded(filepath.Join(dir, "nope.json"), embedded)
		if err != nil {
			t.Fatalf("embedded fallback failed: %v", err)
		}
		if cfg == nil {
			t.Fatal("nil config from embedded fallback")
		}
	})

	t.Run("present file is preferred", func(t *testing.T) {
		p := filepath.Join(dir, "tuning.json")
		if err := os.WriteFile(p, embedded, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := LoadTuningConfigOrEmbedded(p, nil); err != nil {
			t.Fatalf("file load failed: %v", err)
		}
	})

	t.Run("absent path with no embedded errors", func(t *testing.T) {
		if _, err := LoadTuningConfigOrEmbedded(filepath.Join(dir, "missing.json"), nil); err == nil {
			t.Fatal("expected error with no file and no embedded defaults")
		}
	})
}
