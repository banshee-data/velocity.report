package recorder

import (
	"path/filepath"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
)

// A replay records background snapshots stamped with the wall clock, far
// after the capture's own times. FrameAt must name rotation frames by the
// capture's clock and never land on such a snapshot.
func TestFrameAtSkipsSnapshotsStampedOutsideTheSpan(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.vrlog")
	rec, err := NewRecorder(path, "hesai-pandar40p")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	const start = int64(1_788_290_733_000_000_000)
	const wall = int64(1_794_073_654_000_000_000)
	snapshot := func(id uint64, ts int64) *l9endpoints.FrameBundle {
		return &l9endpoints.FrameBundle{FrameID: id, TimestampNanos: ts, SensorID: "s",
			FrameType: l9endpoints.FrameTypeBackground, Background: &l9endpoints.BackgroundSnapshot{SequenceNumber: id}}
	}
	// index 0: a snapshot sharing the first rotation's time; 1..5 rotations at
	// start+0..400 ms; 6: a snapshot on the wall clock; 7..8 rotations.
	frames := []*l9endpoints.FrameBundle{snapshot(100, start)}
	for i := range 5 {
		frames = append(frames, testFrameBundle(uint64(i), start+int64(i)*100_000_000))
	}
	frames = append(frames, snapshot(101, wall))
	for i := 5; i < 7; i++ {
		frames = append(frames, testFrameBundle(uint64(i), start+int64(i)*100_000_000))
	}
	for _, f := range frames {
		if err := rec.Record(f); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	rep, err := NewReplayer(path)
	if err != nil {
		t.Fatalf("NewReplayer: %v", err)
	}
	defer rep.Close()

	cases := []struct {
		name string
		at   int64
		want uint64
	}{
		{"the first rotation, not the snapshot stamped with its time", start, 1},
		{"an exact rotation", start + 200_000_000, 3},
		{"between rotations: the next", start + 150_000_000, 3},
		{"after the snapshot inside the record order", start + 500_000_000, 7},
		{"the last rotation, not a later-stamped snapshot", start + 600_000_000, 8},
		{"past the end: the last rotation", wall + 1_000, 8},
		{"before the start: the first rotation", start - 1_000_000_000, 1},
	}
	for _, c := range cases {
		got, ok := rep.FrameAt(c.at)
		if !ok || got != c.want {
			t.Errorf("%s: FrameAt = %d, %v; want %d", c.name, got, ok, c.want)
		}
	}
	if rep.CurrentFrame() != 0 {
		t.Errorf("FrameAt moved the read position to %d", rep.CurrentFrame())
	}
}
