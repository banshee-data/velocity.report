//go:build pcap
// +build pcap

package lidar

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
)

// writeObservationFixture writes a closed container of frames failed at L3
// (identity, times and the L2 count only) with one gap, varying reason so
// two fixtures can differ in a single value.
func writeObservationFixture(t *testing.T, dir string, frames int, reason string) {
	t.Helper()
	calibration := replayCalibration("cli-test")
	calibrationID, err := l4bobserve.CalibrationID(calibration)
	if err != nil {
		t.Fatal(err)
	}
	w, err := vrlog.Create(dir, vrlog.Manifest{
		Capture:     vrlog.CaptureIdentity{SensorID: "cli-test", SourceType: "synthetic"},
		Extraction:  vrlog.ExtractionIdentity{SourceID: "source/v1/cli-test", CalibrationID: calibrationID, CoordinateFrame: "site/cli-test", ExtractorID: "l4.test/v1"},
		Calibration: calibration,
	})
	if err != nil {
		t.Fatal(err)
	}
	for seq := range uint64(frames) {
		if seq == 1 {
			if err := w.AppendGap(l4bobserve.GapRecord{Cause: "l2-callback-queue-overflow"}); err != nil {
				t.Fatal(err)
			}
		}
		start := int64(1_700_000_000_000_000_000) + int64(seq)*100_000_000
		if err := w.AppendFrame(l4bobserve.FrameRecord{Sequence: seq, SensorFrameID: "f", FrameUnixNanos: start,
			CaptureStartUnixNanos: start, CaptureEndUnixNanos: start + 99_000_000,
			Disposition: l4bobserve.Disposition{Kind: l4bobserve.DispositionFailed, Stage: l4bobserve.StageL3Foreground, Reason: reason},
			Stages:      []l4bobserve.StageCount{{Stage: l4bobserve.StageL2Frame, Output: l4bobserve.KnownCount(0)}}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestObservationsRoutingAndFlags(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "a.vrlog")
	writeObservationFixture(t, dir, 3, "no background model")
	for name, tc := range map[string]struct {
		args []string
		want int
	}{
		"usage":                {[]string{"observations"}, 0},
		"help":                 {[]string{"observations", "help"}, 0},
		"unknown command":      {[]string{"observations", "repair"}, 2},
		"verify needs a dir":   {[]string{"observations", "verify"}, 2},
		"verify bad profile":   {[]string{"observations", "verify", "--require", "everything", dir}, 2},
		"verify help":          {[]string{"observations", "verify", "-h"}, 0},
		"verify":               {[]string{"observations", "verify", "--require", "foreground-complete", dir}, 0},
		"inspect needs a dir":  {[]string{"observations", "inspect"}, 2},
		"inspect":              {[]string{"observations", "inspect", "--records", "-1", dir}, 0},
		"inspect not found":    {[]string{"observations", "inspect", filepath.Join(t.TempDir(), "absent")}, 1},
		"compare needs two":    {[]string{"observations", "compare", dir}, 2},
		"compare bad flag":     {[]string{"observations", "compare", "-nope", dir, dir}, 2},
		"compare missing side": {[]string{"observations", "compare", dir, t.TempDir()}, 1},
	} {
		t.Run(name, func(t *testing.T) {
			if code := silence(t, func() int { return Main(tc.args) }); code != tc.want {
				t.Fatalf("exited %d, want %d", code, tc.want)
			}
		})
	}
}

// verify fails a damaged container and a legacy recording, and passes a
// sound one: the exit code is what a corpus script branches on.
func TestObservationsVerifyExitCodes(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good.vrlog")
	writeObservationFixture(t, good, 4, "no background model")
	damaged := filepath.Join(root, "damaged.vrlog")
	writeObservationFixture(t, damaged, 4, "no background model")
	chunk := filepath.Join(damaged, "chunks", "00000000.chunk")
	data, err := os.ReadFile(chunk)
	if err != nil {
		t.Fatal(err)
	}
	data[len(data)/3] ^= 0x10
	if err := os.WriteFile(chunk, data, 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root, "legacy")
	if err := os.MkdirAll(legacy, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacy, "header.json"), []byte(`{"version":"0.5"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for dir, want := range map[string]int{good: 0, damaged: 1, legacy: 1} {
		if code := silence(t, func() int { return ObservationsMain([]string{"verify", dir}) }); code != want {
			t.Fatalf("verify %s exited %d, want %d", filepath.Base(dir), code, want)
		}
	}
	if code := silence(t, func() int { return ObservationsMain([]string{"verify", good, damaged}) }); code != 1 {
		t.Fatalf("a batch with one damaged container exited %d", code)
	}
}

func TestObservationsCompare(t *testing.T) {
	root := t.TempDir()
	a, b, c := filepath.Join(root, "a.vrlog"), filepath.Join(root, "b.vrlog"), filepath.Join(root, "c.vrlog")
	writeObservationFixture(t, a, 3, "no background model")
	writeObservationFixture(t, b, 3, "no background model")
	writeObservationFixture(t, c, 3, "foreground mask unavailable")

	var out bytes.Buffer
	same, err := compareContainers(&out, a, b)
	if err != nil || !same || !strings.Contains(out.String(), "identical evidence: 4 records") {
		t.Fatalf("same evidence: %v %v\n%s", same, err, out.String())
	}
	out.Reset()
	same, err = compareContainers(&out, a, c)
	if err != nil || same || !strings.Contains(out.String(), "DIFFERENT at record 0: disposition") {
		t.Fatalf("different evidence: %v %v\n%s", same, err, out.String())
	}
	short := filepath.Join(root, "short.vrlog")
	writeObservationFixture(t, short, 2, "no background model")
	out.Reset()
	if same, err = compareContainers(&out, a, short); err != nil || same || !strings.Contains(out.String(), "ends after 3 records") {
		t.Fatalf("a shorter container: %v %v\n%s", same, err, out.String())
	}
	if code := silence(t, func() int { return ObservationsMain([]string{"compare", a, c}) }); code != 1 {
		t.Fatalf("compare of different evidence exited %d", code)
	}
}

func TestReplayObservationsNeedACaseIdentity(t *testing.T) {
	code := silence(t, func() int {
		return ReplayEvalMain([]string{"--pcap", "capture.pcap", "--output", t.TempDir(), "--observations", filepath.Join(t.TempDir(), "o.vrlog")})
	})
	if code != 2 {
		t.Fatalf("exited %d, want 2", code)
	}
}

// captured runs fn with stdout written to a file and returns what it printed.
func captured(t *testing.T, fn func() int) (int, string) {
	t.Helper()
	f, err := os.Create(filepath.Join(t.TempDir(), "stdout"))
	if err != nil {
		t.Fatal(err)
	}
	oldOut, oldErr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = f, f
	code := func() int {
		// Restore even if fn panics, so a failure stays in its own test.
		defer func() {
			os.Stdout, os.Stderr = oldOut, oldErr
			f.Close()
		}()
		return fn()
	}()
	out, err := os.ReadFile(f.Name())
	if err != nil {
		t.Fatal(err)
	}
	return code, string(out)
}

// recover makes an interrupted capture consistent once, reports what it
// did, refuses a container a live writer holds, and says so when there is
// nothing to do; verify and inspect report the capture's state.
func TestObservationsRecover(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "open.vrlog")
	calibration := replayCalibration("cli-test")
	calibrationID, err := l4bobserve.CalibrationID(calibration)
	if err != nil {
		t.Fatal(err)
	}
	w, err := vrlog.Create(dir, vrlog.Manifest{
		Capture:     vrlog.CaptureIdentity{SensorID: "cli-test", SourceType: "synthetic"},
		Extraction:  vrlog.ExtractionIdentity{SourceID: "source/v1/cli-test", CalibrationID: calibrationID, CoordinateFrame: "site/cli-test", ExtractorID: "l4.test/v1"},
		Calibration: calibration,
	})
	if err != nil {
		t.Fatal(err)
	}
	if code, out := captured(t, func() int { return ObservationsMain([]string{"recover", dir}) }); code != 1 || !strings.Contains(out, "in use") {
		t.Fatalf("recover beside a live writer exited %d:\n%s", code, out)
	}
	if err := w.Abandon(); err != nil {
		t.Fatal(err)
	}
	if code, out := captured(t, func() int { return ObservationsMain([]string{"verify", dir}) }); code != 0 || !strings.Contains(out, "OPEN: no terminal generation") {
		t.Fatalf("verify of an interrupted capture exited %d:\n%s", code, out)
	}
	if code, out := captured(t, func() int { return ObservationsMain([]string{"recover", dir}) }); code != 0 || !strings.Contains(out, "wrote recovery generation 1") {
		t.Fatalf("recover exited %d:\n%s", code, out)
	}
	if code, out := captured(t, func() int { return ObservationsMain([]string{"recover", dir}) }); code != 0 || !strings.Contains(out, "nothing to do") {
		t.Fatalf("a second recover exited %d:\n%s", code, out)
	}
	code, out := captured(t, func() int { return ObservationsMain([]string{"inspect", "--records", "0", dir}) })
	if code != 0 || !strings.Contains(out, "INCOMPLETE") || !strings.Contains(out, "commit       group commit at 100ms") ||
		!strings.Contains(out, "not power loss") {
		t.Fatalf("inspect exited %d:\n%s", code, out)
	}
	for _, args := range [][]string{{"recover"}, {"recover", dir, dir}} {
		if code := silence(t, func() int { return ObservationsMain(args) }); code != 2 {
			t.Fatalf("%v exited %d", args, code)
		}
	}
}
