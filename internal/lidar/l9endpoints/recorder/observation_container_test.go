package recorder

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
	"google.golang.org/protobuf/proto"
)

// observationContainer writes a small VRLOG 1.x container with one frame.
func observationContainer(t *testing.T) string {
	t.Helper()
	calibration := l4bobserve.Calibration{SensorID: "s", FromFrame: "sensor", ToFrame: "site",
		Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
	calibrationID, err := l4bobserve.CalibrationID(calibration)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "capture.vrlog")
	w, err := vrlog.Create(dir, vrlog.Manifest{
		Capture:     vrlog.CaptureIdentity{SensorID: "s", SourceType: "live"},
		Extraction:  vrlog.ExtractionIdentity{SourceID: "source/v1/test", CalibrationID: calibrationID, CoordinateFrame: "site/s", ExtractorID: "l4.test/v1"},
		Calibration: calibration,
	})
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now().UnixNano()
	frame := l4bobserve.FrameRecord{Sequence: 0, SensorFrameID: "f0", FrameUnixNanos: start, CaptureStartUnixNanos: start, CaptureEndUnixNanos: start + 1,
		Disposition: l4bobserve.Disposition{Kind: l4bobserve.DispositionFailed, Stage: l4bobserve.StageL3Foreground, Reason: "test"},
		Stages:      []l4bobserve.StageCount{{Stage: l4bobserve.StageL2Frame, Output: l4bobserve.KnownCount(0)}}}
	if err := w.AppendFrame(frame); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

// G-OBS-COMPAT, legacy side. The FrameBundle replayer refuses an observation
// container by its root, even one that also holds legacy-named files, and
// never probes its payloads; a 0.5 recording still replays.
func TestReplayerRefusesObservationContainers(t *testing.T) {
	dir := observationContainer(t)
	_, err := NewReplayer(dir)
	if !errors.Is(err, ErrObservationContainer) || !strings.Contains(err.Error(), "observations inspect") {
		t.Fatalf("NewReplayer on a container = %v", err)
	}

	// Legacy-named files beside the root do not make it a recording.
	legacy := filepath.Join(t.TempDir(), "legacy")
	rec, err := NewRecorder(legacy, "s")
	if err != nil {
		t.Fatal(err)
	}
	if err := rec.Record(testFrameBundle(1, time.Now().UnixNano())); err != nil {
		t.Fatal(err)
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"header.json", "index.bin"} {
		data, err := os.ReadFile(filepath.Join(legacy, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := NewReplayer(dir); !errors.Is(err, ErrObservationContainer) {
		t.Fatalf("a container with a stray header.json was opened: %v", err)
	}

	rep, err := NewReplayer(legacy)
	if err != nil {
		t.Fatalf("a legacy recording no longer opens: %v", err)
	}
	defer rep.Close()
	if frame, err := rep.ReadFrame(); err != nil || frame.FrameID != 1 {
		t.Fatalf("legacy frame = %+v, %v", frame, err)
	}
	if _, err := rep.ReadFrame(); !errors.Is(err, io.EOF) {
		t.Fatalf("legacy EOF = %v", err)
	}
	if _, err := vrlog.Open(legacy, vrlog.Options{}); !errors.Is(err, vrlog.ErrLegacyRecording) {
		t.Fatalf("the observation reader opened a legacy recording: %v", err)
	}
}

// Why the refusal must be by root, not by payload: the legacy probe decodes
// an observation frame record as a FrameBundle without complaint, because
// protobuf treats the fields it cannot place as unknown. A version string
// could not have prevented this.
func TestLegacyProbeCannotTellObservationPayloadsApart(t *testing.T) {
	payload, err := proto.Marshal(&pb.FrameRecord{Sequence: 42, SensorFrameId: "f42", CaptureStartUnixNanos: 7})
	if err != nil {
		t.Fatal(err)
	}
	frame, err := deserializeFrameProto(payload)
	if err != nil || frame.FrameID != 42 || !looksLikeFrameBundle(frame) {
		t.Fatalf("expected the legacy decoder to misread the payload as frame 42, got %+v, %v", frame, err)
	}
}
