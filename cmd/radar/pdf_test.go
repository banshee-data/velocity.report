package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/report"
)

func TestRunPDF_MissingConfigFlag(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runPDF([]string{}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("--config is required")) {
		t.Errorf("expected error about --config, got: %s", stderr.String())
	}
}

func TestRunPDF_MissingDBFlag(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "test.json")
	if err := os.WriteFile(cfgPath, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := runPDF([]string{"--config", cfgPath}, &stdout, &stderr)
	if code != 2 {
		t.Errorf("expected exit code 2, got %d", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("--db is required")) {
		t.Errorf("expected error about --db, got: %s", stderr.String())
	}
}

func TestRunPDF_Version(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runPDF([]string{"--version"}, &stdout, &stderr)
	if code != 0 {
		t.Errorf("expected exit code 0, got %d", code)
	}
	if !bytes.Contains(stdout.Bytes(), []byte("velocity-report pdf")) {
		t.Errorf("expected version output, got: %s", stdout.String())
	}
}

func TestRunPDF_InvalidConfigJSON(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "bad.json")
	if err := os.WriteFile(cfgPath, []byte(`{not json`), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(tmpDir, "nonexistent.db")

	var stdout, stderr bytes.Buffer
	code := runPDF([]string{"--config", cfgPath, "--db", dbPath}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("parse config JSON")) {
		t.Errorf("expected JSON parse error, got: %s", stderr.String())
	}
}

func TestRunPDF_MissingConfigFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runPDF([]string{"--config", "/nonexistent/config.json", "--db", "/tmp/test.db"}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("read config file")) {
		t.Errorf("expected file read error, got: %s", stderr.String())
	}
}

func TestRunPDF_OutputDirOverride(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := report.Config{
		SiteID:    1,
		Location:  "Test",
		StartDate: "2025-01-01",
		EndDate:   "2025-01-02",
		Timezone:  "UTC",
		Units:     "mph",
		Group:     "1h",
		Source:    "radar_objects",
		OutputDir: "/should/be/overridden",
	}
	cfgData, _ := json.Marshal(cfg)
	cfgPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(cfgPath, cfgData, 0o644); err != nil {
		t.Fatal(err)
	}

	// Use a nonexistent DB to get a DB open error — proves we got past config parsing.
	dbPath := filepath.Join(tmpDir, "nonexistent.db")

	var stdout, stderr bytes.Buffer
	code := runPDF([]string{"--config", cfgPath, "--db", dbPath, "--output", tmpDir}, &stdout, &stderr)
	if code != 1 {
		t.Errorf("expected exit code 1, got %d", code)
	}
	// Should fail at report generation, not config parsing — proves we got past config.
	if !bytes.Contains(stderr.Bytes(), []byte("report generation failed")) {
		t.Errorf("expected report generation error, got: %s", stderr.String())
	}
}
