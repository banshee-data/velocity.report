package server

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
	"github.com/banshee-data/velocity.report/internal/report/typst/typstbin"
)

// TestRunHeadwayRequiresOneMode: the synthetic oracle or the provisional
// field report, never neither and never both.
func TestRunHeadwayRequiresOneMode(t *testing.T) {
	for _, args := range [][]string{nil, {"--oracle", "--db", "evidence.db"}} {
		var stdout, stderr bytes.Buffer
		if code := runHeadway(args, &stdout, &stderr); code != 2 {
			t.Fatalf("runHeadway(%v) exit %d, want 2", args, code)
		}
		if !strings.Contains(stderr.String(), "exactly one of --oracle") || !strings.Contains(stderr.String(), "--db") {
			t.Errorf("stderr = %q, want both modes named", stderr.String())
		}
		if stdout.Len() != 0 {
			t.Errorf("stdout = %q, want nothing", stdout.String())
		}
	}
}

func TestRunHeadwayRejectsBadArguments(t *testing.T) {
	for _, args := range [][]string{
		{"--oracle", "--paper", "a3"},
		{"--oracle", "extra"},
		{"--bogus"},
		{"--oracle", "--source", "source/v1/x"},
		{"--oracle", "--param-hash", "sha256:x"},
	} {
		var stdout, stderr bytes.Buffer
		if code := runHeadway(args, &stdout, &stderr); code != 2 {
			t.Errorf("runHeadway(%v) exit %d, want 2 (stderr %q)", args, code, stderr.String())
		}
	}
}

// headwayMockPDF is a structurally valid PDF with an Info dictionary, enough
// for the report's metadata stamp.
func headwayMockPDF() string {
	objects := []string{
		"1 0 obj\n<< /Type /Catalog /Pages 2 0 R >>\nendobj\n",
		"2 0 obj\n<< /Type /Pages /Count 0 >>\nendobj\n",
		"3 0 obj\n<< /Creator (Typst 0.13.1) >>\nendobj\n",
	}
	var pdf strings.Builder
	pdf.WriteString("%PDF-1.7\n")
	offsets := make([]int, len(objects)+1)
	for i, obj := range objects {
		offsets[i+1] = pdf.Len()
		pdf.WriteString(obj)
	}
	xref := pdf.Len()
	fmt.Fprintf(&pdf, "xref\n0 %d\n%010d %05d f \n", len(objects)+1, 0, 65535)
	for i := 1; i <= len(objects); i++ {
		fmt.Fprintf(&pdf, "%010d %05d n \n", offsets[i], 0)
	}
	fmt.Fprintf(&pdf, "trailer\n<< /Size %d /Root 1 0 R /Info 3 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objects)+1, xref)
	return pdf.String()
}

// TestRunHeadwayOracleWritesTheReport runs the CLI end to end with a
// stand-in typst first on PATH.
func TestRunHeadwayOracleWritesTheReport(t *testing.T) {
	if typstbin.Embedded() {
		t.Skip("an embedded typst takes precedence over PATH")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\ncat > /dev/null\ncat <<'EOF'\n" + headwayMockPDF() + "EOF\n"
	if err := os.WriteFile(filepath.Join(bin, "typst"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(typstbin.EnvNoDownload, "1")

	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := runHeadway([]string{"--oracle", "--output", out, "--paper", "a4"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if !strings.HasPrefix(stdout.String(), "Status: SYNTHETIC ORACLE\n") {
		t.Errorf("stdout = %q, want the status first", stdout.String())
	}
	for _, name := range []string{"headway_synthetic_oracle_report.pdf", "headway_synthetic_oracle_report_sources.zip"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
		if !strings.Contains(stdout.String(), filepath.Join(out, name)) {
			t.Errorf("stdout does not name %s", name)
		}
	}
}

const headwayTestSource = "source/v1/cafe"

// seedHeadwayEvidence writes the steady-approach scenario's poses as online
// estimates in a new evidence database, each linked to an observation row.
func seedHeadwayEvidence(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "evidence.db")
	database, err := db.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	for _, tr := range l8behaviour.ScenarioSteadyApproach().Trajectories {
		for _, s := range tr.Samples {
			obs := fmt.Sprintf("observation/%s/%d", tr.Passage.TrackID, s.CaptureUnixNanos)
			if _, err := database.Exec(`INSERT INTO lidar_observations
				(observation_id, schema_version, source_id, calibration_id, sensor_id, frame_id, frame_unix_nanos,
				 cluster_unix_nanos, cluster_id, record_json, inserted_at_ns)
				VALUES (?, 1, ?, 'calibration/test', 'sensor_test', 'frame', ?, ?, 1, '{}', 1)`,
				obs, headwayTestSource, s.CaptureUnixNanos, s.CaptureUnixNanos); err != nil {
				t.Fatal(err)
			}
			var cov [16]float32
			for i, c := range s.Covariance {
				cov[i] = float32(c)
			}
			id := fmt.Sprintf("estimate/%s/%d", tr.Passage.TrackID, s.CaptureUnixNanos)
			if err := sqlite.InsertStateEstimate(database, sqlite.TrackEstimate{
				EstimateID: id, TrackID: tr.Passage.TrackID, ObservationID: obs, SourceID: headwayTestSource,
				CalibrationID: "calibration/test", FrameUnixNanos: s.CaptureUnixNanos, MeasurementUnixNanos: s.CaptureUnixNanos,
				EstimatorID: "cv_kf_v1", ObservationModelID: "obb_centre_v1", ParamHash: "sha256:online", Stage: "online",
				MeasurementSource: "obb_centre_v1", X: float32(s.X), Y: float32(s.Y), VX: float32(s.VX), VY: float32(s.VY),
				Covariance: cov,
			}, sqlite.TrackResidual{EstimateID: id, ObservationID: obs, Disposition: "accepted", Reason: "association_accepted"}); err != nil {
				t.Fatal(err)
			}
		}
	}
	return path
}

// TestRunHeadwayFieldWritesTheReport runs the provisional field report end
// to end with a stand-in typst: the status and the run's figures first, then
// the provisional PDF and archive.
func TestRunHeadwayFieldWritesTheReport(t *testing.T) {
	if typstbin.Embedded() {
		t.Skip("an embedded typst takes precedence over PATH")
	}
	bin := t.TempDir()
	script := "#!/bin/sh\ncat > /dev/null\ncat <<'EOF'\n" + headwayMockPDF() + "EOF\n"
	if err := os.WriteFile(filepath.Join(bin, "typst"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv(typstbin.EnvNoDownload, "1")

	evidence := seedHeadwayEvidence(t)
	out := t.TempDir()
	var stdout, stderr bytes.Buffer
	args := []string{"--db", evidence, "--source", headwayTestSource, "--stage", "online", "--output", out}
	if code := runHeadway(args, &stdout, &stderr); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	got := stdout.String()
	for _, want := range []string{
		"Status: PROVISIONAL\n", "Estimates: online cv_kf_v1, obb_centre_v1, sha256:online",
		"Encounters: 1 (1 written, 0 already stored)", "suppressed class_not_supported",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stdout lacks %q:\n%s", want, got)
		}
	}
	if !strings.HasPrefix(got, "Status: PROVISIONAL\n") {
		t.Errorf("stdout does not lead with the status:\n%s", got)
	}
	for _, name := range []string{"headway_provisional_report.pdf", "headway_provisional_report_sources.zip"} {
		if _, err := os.Stat(filepath.Join(out, name)); err != nil {
			t.Errorf("missing %s: %v", name, err)
		}
	}

	// Again: the encounter is already stored and nothing is written.
	stdout.Reset()
	if code := runHeadway(args, &stdout, &stderr); code != 0 || !strings.Contains(stdout.String(), "(0 written, 1 already stored)") {
		t.Errorf("second run exit %d:\n%s", code, stdout.String())
	}
}

// TestRunHeadwayFieldRefuses: a missing database is not created, a missing
// source lists what the database holds, the default final stage says what
// is there instead, and an unknown stage is a usage error.
func TestRunHeadwayFieldRefuses(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "absent.db")
	var stdout, stderr bytes.Buffer
	if code := runHeadway([]string{"--db", missing, "--source", headwayTestSource}, &stdout, &stderr); code != 1 {
		t.Errorf("missing database: exit %d", code)
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Error("a missing --db was created")
	}

	evidence := seedHeadwayEvidence(t)
	for _, c := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"--db", evidence}, 2, headwayTestSource + " (stages: [online])"},
		{[]string{"--db", evidence, "--source", headwayTestSource}, 1, "holds no final estimates"},
		{[]string{"--db", evidence, "--source", headwayTestSource, "--stage", "smoothed"}, 2, "want final, fixed_lag or online"},
	} {
		stdout.Reset()
		stderr.Reset()
		if code := runHeadway(append(c.args, "--output", t.TempDir()), &stdout, &stderr); code != c.code ||
			!strings.Contains(stderr.String(), c.want) {
			t.Errorf("runHeadway(%v) exit %d, stderr %q; want %d and %q", c.args, code, stderr.String(), c.code, c.want)
		}
	}
}
