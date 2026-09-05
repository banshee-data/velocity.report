package recorder

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
)

// Frame accounting must let a reader recover the sensor-rotation count.
// TotalFrames counts every record, including background snapshots, because it
// indexes index.bin. Only RotationFrames is comparable with an analysis run's
// frame count or a source PCAP's rotation count.

func recordMixed(t *testing.T, types []l9endpoints.FrameType) LogHeader {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "acct.vrlog")
	rec, err := NewRecorder(dir, "hesai-pandar40p")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	base := int64(1765057342040018330)
	for i, ft := range types {
		fb := &l9endpoints.FrameBundle{
			FrameID:        uint64(i),
			TimestampNanos: base + int64(i)*100_000_000,
			SensorID:       "hesai-pandar40p",
			FrameType:      ft,
		}
		if err := rec.Record(fb); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "header.json"))
	if err != nil {
		t.Fatalf("read header: %v", err)
	}
	var h LogHeader
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatalf("unmarshal header: %v", err)
	}
	return h
}

func TestHeaderSeparatesRotationsFromBackgroundSnapshots(t *testing.T) {
	types := []l9endpoints.FrameType{
		l9endpoints.FrameTypeBackground,
		l9endpoints.FrameTypeForeground,
		l9endpoints.FrameTypeForeground,
		l9endpoints.FrameTypeEmpty,
		l9endpoints.FrameTypeBackground,
		l9endpoints.FrameTypeFull,
	}
	h := recordMixed(t, types)

	if h.TotalFrames != 6 {
		t.Errorf("TotalFrames = %d, want 6 (every record, matching index.bin)", h.TotalFrames)
	}
	// Foreground x2, empty, full — the empty placeholder is a real rotation
	// over an empty street and must be counted as one.
	if h.RotationFrames != 4 {
		t.Errorf("RotationFrames = %d, want 4 (background snapshots excluded)", h.RotationFrames)
	}
	if h.BackgroundFrames != 2 {
		t.Errorf("BackgroundFrames = %d, want 2", h.BackgroundFrames)
	}
	if h.TotalFrames != h.RotationFrames+h.BackgroundFrames {
		t.Errorf("TotalFrames (%d) must equal RotationFrames (%d) + BackgroundFrames (%d)",
			h.TotalFrames, h.RotationFrames, h.BackgroundFrames)
	}
}

func TestRotationCountIgnoresBackgroundVolume(t *testing.T) {
	// However many snapshots the background model emits, the rotation count
	// must reflect only what the sensor actually swept.
	types := []l9endpoints.FrameType{}
	for i := 0; i < 10; i++ {
		types = append(types, l9endpoints.FrameTypeBackground)
	}
	for i := 0; i < 3; i++ {
		types = append(types, l9endpoints.FrameTypeForeground)
	}
	h := recordMixed(t, types)

	if h.RotationFrames != 3 {
		t.Errorf("RotationFrames = %d, want 3 despite 10 snapshots", h.RotationFrames)
	}
	if h.TotalFrames != 13 {
		t.Errorf("TotalFrames = %d, want 13", h.TotalFrames)
	}
}

func TestBackgroundFramesDoNotSetTimeRange(t *testing.T) {
	// A snapshot stamped outside the rotation range must not widen start/end.
	dir := filepath.Join(t.TempDir(), "range.vrlog")
	rec, err := NewRecorder(dir, "hesai-pandar40p")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	base := int64(1765057342040018330)
	frames := []struct {
		ft l9endpoints.FrameType
		ts int64
	}{
		// A snapshot carrying a timestamp from the recording's future, which
		// is what a stale lastForegroundTimestamp used to produce.
		{l9endpoints.FrameTypeBackground, base + 999_000_000_000},
		{l9endpoints.FrameTypeForeground, base},
		{l9endpoints.FrameTypeForeground, base + 100_000_000},
		{l9endpoints.FrameTypeForeground, base + 200_000_000},
	}
	for i, f := range frames {
		if err := rec.Record(&l9endpoints.FrameBundle{
			FrameID: uint64(i), TimestampNanos: f.ts,
			SensorID: "hesai-pandar40p", FrameType: f.ft,
		}); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	b, _ := os.ReadFile(filepath.Join(dir, "header.json"))
	var h LogHeader
	if err := json.Unmarshal(b, &h); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if h.StartNs != base {
		t.Errorf("StartNs = %d, want %d (the first rotation, not the snapshot)", h.StartNs, base)
	}
	if h.EndNs != base+200_000_000 {
		t.Errorf("EndNs = %d, want %d (the last rotation)", h.EndNs, base+200_000_000)
	}
	if span := h.EndNs - h.StartNs; span <= 0 {
		t.Errorf("span %d must be positive", span)
	}
}
