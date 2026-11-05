package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/eligibility"
	fx "github.com/banshee-data/velocity.report/internal/lidar/eligibility/eligibilitytest"
)

// allowDevelopment gives a test server an eligibility policy under which the
// named capture files and its live sensor are development data, nothing is
// held out, and uses are recorded in memory: what a test of an entry point's
// own behaviour needs once the guard runs in front of it.
func allowDevelopment(t *testing.T, ws *Server, captures ...string) *fx.MemoryLedger {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "splits"), 0o755); err != nil {
		t.Fatal(err)
	}
	if ws.sensorID == "" {
		ws.sensorID = "hesai-test"
	}
	var entries []any
	for i, path := range captures {
		sha, err := eligibility.FileSHA256(path)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, fx.CaptureEntry("capture-"+string(rune('a'+i)), "dev", sha, filepath.Base(path)))
	}
	manifest := fx.Manifest(t, dir, map[string]any{
		"sites": []any{fx.Site("dev", "development")}, "captures": append([]any{}, entries...),
		"live_sources": []any{map[string]any{"sensor_id": ws.sensorID, "site_id": "dev"}},
	})
	ledger := &fx.MemoryLedger{}
	ws.eligibilityOnce.Do(func() {})
	ws.eligibility = eligibility.New(eligibility.Config{ManifestPath: manifest, SplitsDir: filepath.Join(dir, "splits"), Ledger: ledger})
	return ledger
}

// allowDevelopmentCapture writes a capture into a fresh PCAP directory and
// allows it, for tests that need a sweep or HINT request to reach its runner.
func allowDevelopmentCapture(t *testing.T, ws *Server) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	path, _ := fx.Capture(t, dir, "dev.pcap", "synthetic development capture")
	ws.pcapSafeDir = dir
	ws.clipFilesForTest = func(string) ([]string, error) { return []string{"dev.pcap"}, nil }
	allowDevelopment(t, ws, path)
	return "dev.pcap"
}
