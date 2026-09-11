package scene

import (
	"fmt"
	"path/filepath"
	"sort"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// span is one source track's life: it is present from First to Last inclusive,
// in source frame indices.
type span struct {
	id          string
	first, last int
}

// writeVRLOGWithSpans records a VRLOG in which each span is present only for
// its own frames, so a reader can tell continuity from fragmentation.
func writeVRLOGWithSpans(t *testing.T, frames int, spans []span) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "spans.vrlog")
	rec, err := recorder.NewRecorder(dir, "hesai-pandar40p")
	if err != nil {
		t.Fatalf("NewRecorder: %v", err)
	}
	for i := 0; i < frames; i++ {
		ts := baseNs + int64(i)*100_000_000
		set := &l9endpoints.TrackSet{FrameID: uint64(i), TimestampNanos: ts}
		for _, s := range spans {
			if i < s.first || i > s.last {
				continue
			}
			set.Tracks = append(set.Tracks, l9endpoints.Track{
				TrackID:  s.id,
				SensorID: "hesai-pandar40p",
				State:    l9endpoints.TrackStateConfirmed,
				X:        float32(i), Y: 1, Z: 0,
				VX: 10, SpeedMps: 10, MaxSpeedMps: 10,
				BBoxLength: 4, BBoxWidth: 2, BBoxHeight: 1.5,
				ObjectClass: "car", ClassConfidence: 0.9,
			})
		}
		if err := rec.Record(&l9endpoints.FrameBundle{
			FrameID: uint64(i), TimestampNanos: ts, SensorID: "hesai-pandar40p",
			FrameType: l9endpoints.FrameTypeForeground, Tracks: set,
		}); err != nil {
			t.Fatalf("Record %d: %v", i, err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return dir
}

// appearances indexes an export by local track id, in export frame order.
func appearances(frames []Frame) map[string][]int {
	out := map[string][]int{}
	for i, f := range frames {
		for _, tr := range f.Tracks {
			out[tr.ID] = append(out[tr.ID], i)
		}
	}
	return out
}

// TestExportKeepsTrackTrailsUnbroken pins the property the viewer draws a trail
// from: a track present for a continuous run of source frames must come out as
// a continuous run of export frames under one identifier.
//
// This is the regression guard for scenes that played as objects flickering in
// and out with no trail. Both ways that can happen are covered: an identifier
// re-keyed part-way through a track's life, and an identifier recycled from a
// track that has already ended.
func TestExportKeepsTrackTrailsUnbroken(t *testing.T) {
	// 90 frames at 10 Hz is 9 s; 4 s chunks cut it twice, so a long track has
	// to survive two chunk boundaries. Lives are staggered so that ids are
	// retired mid-recording, which is when a recycling allocator goes wrong.
	const frames = 90
	spans := []span{
		{"long-runner", 0, 89},        // the whole recording, across both cuts
		{"crosses-first-cut", 30, 55}, // spans one boundary
		{"early", 5, 25},              // ends before the second chunk
		{"late", 60, 88},              // starts after "early" has gone
		{"blink", 44, 45},             // two frames, at a boundary
	}
	src := writeVRLOGWithSpans(t, frames, spans)
	out := t.TempDir()

	res, err := Export(Options{
		VRLOGPath: src, OutDir: out, Kind: KindTracks,
		Stride: 1, ChunkSeconds: 4,
	})
	if err != nil {
		t.Fatalf("Export: %v", err)
	}
	if res.Header.FrameCount != frames {
		t.Fatalf("exported %d frames, want %d", res.Header.FrameCount, frames)
	}
	if res.DroppedNonMonotonic != 0 {
		t.Fatalf("export dropped %d frames as non-monotonic; the fixture is evenly spaced", res.DroppedNonMonotonic)
	}

	got := appearances(readFrames(t, out))
	if len(got) != len(spans) {
		t.Fatalf("export carries %d track ids, want %d — an id was split or merged", len(got), len(spans))
	}

	// Each exported id must be one unbroken run. A hole means the viewer lifts
	// the pen: the trail breaks and the object reads as flicker.
	for id, at := range got {
		sort.Ints(at)
		for i := 1; i < len(at); i++ {
			if at[i] != at[i-1]+1 {
				t.Errorf("track %q is absent from frames %d..%d then returns: a trail cannot be drawn through a hole",
					id, at[i-1]+1, at[i]-1)
				break
			}
		}
	}

	// Every source span must be present in full under exactly one id, so a
	// track cannot be truncated or handed a second identifier part-way.
	lengths := map[int]int{}
	for _, at := range got {
		lengths[len(at)]++
	}
	for _, s := range spans {
		want := s.last - s.first + 1
		if lengths[want] == 0 {
			t.Errorf("no exported track lasts %d frames, so source track %q (frames %d..%d) did not survive the export intact",
				want, s.id, s.first, s.last)
			continue
		}
		lengths[want]--
	}
}

// TestExportNeverRecyclesATrackIdentifier pins the allocator directly: once an
// identifier has been handed to a source track it belongs to that track for the
// life of the export, even after the track ends and even across chunk files.
//
// A recycling allocator is invisible in aggregate counts and shows up only as
// two unrelated objects sharing a trail, so assert it rather than infer it.
func TestExportNeverRecyclesATrackIdentifier(t *testing.T) {
	// Forty tracks that never overlap: each lives two frames and dies. A
	// recycling allocator would reuse "0" for all of them.
	const frames = 80
	spans := make([]span, 0, 40)
	for i := 0; i < 40; i++ {
		spans = append(spans, span{fmt.Sprintf("source-%02d", i), i * 2, i*2 + 1})
	}
	src := writeVRLOGWithSpans(t, frames, spans)
	out := t.TempDir()

	if _, err := Export(Options{
		VRLOGPath: src, OutDir: out, Kind: KindTracks,
		Stride: 1, ChunkSeconds: 2,
	}); err != nil {
		t.Fatalf("Export: %v", err)
	}

	got := appearances(readFrames(t, out))
	if len(got) != len(spans) {
		t.Fatalf("export carries %d ids for %d distinct source tracks: identifiers are being recycled", len(got), len(spans))
	}
	for id, at := range got {
		if len(at) != 2 {
			t.Errorf("id %q appears in %d frames, want 2: it is shared by more than one source track", id, len(at))
		}
	}
}
