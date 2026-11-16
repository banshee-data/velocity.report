package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/egotrack"
)

func TestRunWritesSummaryAndKinematics(t *testing.T) {
	dir := t.TempDir()
	tr := egotrack.Trajectory{Schema: egotrack.SchemaV0, Frame: "local", TimeScale: "session_seconds"}
	for i := 0; i < 300; i++ {
		tt := float64(i) / 10
		x := 8 * tt // constant speed: this test checks the plumbing, not the estimator
		tr.Samples = append(tr.Samples, egotrack.Sample{Frame: i, T: tt, P: [3]float64{x, 0, 0}, Q: [4]float64{1, 0, 0, 0}})
	}
	b, _ := json.Marshal(tr)
	traj := filepath.Join(dir, "traj.json")
	if err := os.WriteFile(traj, b, 0o600); err != nil {
		t.Fatal(err)
	}
	anchors := filepath.Join(dir, "anchors.json")
	if err := os.WriteFile(anchors, []byte(`[{"name":"a","at":"start","p_m":[0,0,0],"sigma_h_m":0.1}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "out")
	var stdout bytes.Buffer
	if err := run([]string{"-trajectory", traj, "-anchors", anchors, "-out-dir", out}, &stdout); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "anchor a (start") {
		t.Errorf("stdout: %s", stdout.String())
	}
	var sum map[string]any
	sb, err := os.ReadFile(filepath.Join(out, "summary.json"))
	if err != nil || json.Unmarshal(sb, &sum) != nil {
		t.Fatalf("summary: %v", err)
	}
	if s := sum["max_speed_ms"].(float64); s < 7.8 || s > 8.2 {
		t.Errorf("max speed %v", s)
	}
	csv, err := os.ReadFile(filepath.Join(out, "kinematics.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(csv), "\n"); n != 301 {
		t.Errorf("kinematics rows %d, want 301", n)
	}
	if err := run([]string{"-trajectory", traj, "-out-dir", out}, &stdout); err == nil {
		t.Error("wrote into a non-empty output directory")
	}
}
