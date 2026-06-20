//go:build pcap
// +build pcap

package pcapanalyse

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestRun_MotionJSON verifies --motion-json writes the timeline JSON and
// implies --motion (samples recorded even without an explicit -motion).
func TestRun_MotionJSON(t *testing.T) {
	cfg := baseConfig(t, testCapture(t, 1500))
	cfg.MotionJSONPath = filepath.Join(cfg.OutputDir, "motion.json")

	var code int
	captureOutput(t, func() { code = Run(cfg) })
	if code != 0 {
		t.Fatalf("Run with --motion-json = %d, want 0", code)
	}

	data, err := os.ReadFile(cfg.MotionJSONPath)
	if err != nil {
		t.Fatalf("read motion json: %v", err)
	}
	var rep motionTimelineReport
	if err := json.Unmarshal(data, &rep); err != nil {
		t.Fatalf("unmarshal motion json: %v", err)
	}
	if rep.File == "" {
		t.Error("expected file field in the motion JSON")
	}
	if len(rep.MotionTimeline) == 0 {
		t.Error("expected motion timeline periods in the motion JSON")
	}
}

func TestRun_MotionJSON_WriteError(t *testing.T) {
	cfg := baseConfig(t, testCapture(t, 1200))
	f := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(f, []byte("x"), 0o644)
	cfg.MotionJSONPath = filepath.Join(f, "sub", "motion.json") // parent is a file
	var code int
	captureOutput(t, func() { code = Run(cfg) })
	if code != 1 {
		t.Errorf("Run with unwritable --motion-json = %d, want 1", code)
	}
}
