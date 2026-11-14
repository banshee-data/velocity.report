package l4bobserve

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestCaptureAndCalibrationIdentityBindEvidenceInputs(t *testing.T) {
	columbus := CaptureSource{ClipID: "columbus-broadway", CapturePaths: []string{
		"s2/s2_sf_3_20260902132037_00002.pcap",
		"s2/s2_sf_3_20260902132537_00003.pcap",
	}, CaptureSHA256s: []string{strings.Repeat("a", 64), strings.Repeat("b", 64)}, ExtractorID: "l4.dbscan_xy_v1:tuning/abc"}
	marina := CaptureSource{ClipID: "marina-webster-beach", CapturePaths: []string{
		"s2/s2_sf_2_20260902102827_00013.pcap",
		"s2/s2_sf_2_20260902103327_00014.pcap",
	}, CaptureSHA256s: []string{strings.Repeat("c", 64), strings.Repeat("d", 64)}, ExtractorID: "l4.dbscan_xy_v1:tuning/abc"}
	columbusID, err := SourceID(columbus)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := SourceID(columbus); err != nil || again != columbusID {
		t.Fatalf("source identity was not stable: %q, %v", again, err)
	}
	if marinaID, err := SourceID(marina); err != nil || marinaID == columbusID {
		t.Fatalf("distinct corpus cases shared a source identity: %q, %v", marinaID, err)
	}
	reordered := columbus
	reordered.CapturePaths = []string{columbus.CapturePaths[1], columbus.CapturePaths[0]}
	reordered.CaptureSHA256s = []string{columbus.CaptureSHA256s[1], columbus.CaptureSHA256s[0]}
	if reorderedID, err := SourceID(reordered); err != nil || reorderedID == columbusID {
		t.Fatalf("file order did not contribute to source identity: %q, %v", reorderedID, err)
	}

	calibration := Calibration{SensorID: "hesai-pandar40p", FromFrame: "sensor/hesai-pandar40p", ToFrame: "site/columbus-broadway", Transform: [16]float64{1, 0, 0, 1, 0, 1, 0, 2, 0, 0, 1, 3, 0, 0, 0, 1}}
	calibrationID, err := CalibrationID(calibration)
	if err != nil {
		t.Fatal(err)
	}
	calibration.Transform[3] += 0.01
	if changed, err := CalibrationID(calibration); err != nil || changed == calibrationID {
		t.Fatalf("transform revision did not change calibration identity: %q, %v", changed, err)
	}
	observationID, err := ObservationID(columbusID, calibrationID, 123, 4)
	if err != nil || !strings.HasPrefix(observationID, "observation/v1/") {
		t.Fatalf("observation identity = %q, %v", observationID, err)
	}
}

func TestIdentityRejectsIncompleteEvidence(t *testing.T) {
	if _, err := SourceID(CaptureSource{}); err == nil {
		t.Fatal("accepted source without a case or captures")
	}
	if _, err := SourceID(CaptureSource{ClipID: "case", CapturePaths: []string{"a.pcap"}, CaptureSHA256s: []string{strings.Repeat("x", 64)}, ExtractorID: "l4"}); err == nil {
		t.Fatal("accepted an invalid capture digest")
	}
	if _, err := CalibrationID(Calibration{}); err == nil {
		t.Fatal("accepted calibration without frame identities")
	}
	if _, err := ObservationID("", "calibration/v1/x", 0, 0); err == nil {
		t.Fatal("accepted observation without source identity")
	}
}

func TestReplayWindowIdentityVersionsAndBindsEveryBoundary(t *testing.T) {
	source := CaptureSource{ClipID: "clip", CapturePaths: []string{"capture"}, CaptureSHA256s: []string{strings.Repeat("a", 64)}, ExtractorID: "extractor"}
	legacy, err := SourceID(source)
	if err != nil || legacy != "source/v1/f0ed5c3fbd403403a463973bc261e713e80d39676333f908fa35b424b999aac4" {
		t.Fatalf("legacy: %s %v", legacy, err)
	}
	w := ReplayWindow{StartOffsetNanos: 20, WarmupNanos: 10, DurationNanos: 30}
	source.Window = &w
	id, err := SourceID(source)
	if err != nil || !strings.HasPrefix(id, "source/v2/") {
		t.Fatalf("window: %s %v", id, err)
	}
	if again, err := SourceID(source); err != nil || again != id {
		t.Fatalf("unstable: %s %v", again, err)
	}
	for _, changed := range []ReplayWindow{{21, 10, 30}, {20, 11, 30}, {20, 10, 31}, {20, 10, 0}, {}} {
		source.Window = &changed
		got, err := SourceID(source)
		if err != nil || got == id || got == legacy {
			t.Fatalf("window %+v: %s %v", changed, got, err)
		}
	}
	source.Window = &w
	encoded, _ := json.Marshal(source)
	var decoded CaptureSource
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	if got, err := SourceID(decoded); err != nil || got != id {
		t.Fatalf("roundtrip %s %v", got, err)
	}
	for _, bad := range []ReplayWindow{{-1, 0, 0}, {1, -1, 0}, {1, 2, 0}, {1, 0, -1}, {math.MaxInt64, 0, 1}} {
		source.Window = &bad
		if _, err := SourceID(source); err == nil {
			t.Fatalf("accepted %+v", bad)
		}
	}
}
