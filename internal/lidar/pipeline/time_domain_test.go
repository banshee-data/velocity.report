package pipeline

import (
	"fmt"
	"math"
	"sort"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// timestampRecordingTracker records the timestamp each Update receives.
type timestampRecordingTracker struct {
	*l5tracks.Tracker
	received []time.Time
}

func (r *timestampRecordingTracker) Update(clusters []l5tracks.WorldCluster, timestamp time.Time) {
	r.received = append(r.received, timestamp)
	r.Tracker.Update(clusters, timestamp)
}

// timeDomainScene is a seeded background followed by a slowly approaching
// foreground object: enough to create, confirm and move a track.
func timeDomainScene(captureTimes []time.Time) []*l2frames.LiDARFrame {
	frames := make([]*l2frames.LiDARFrame, 0, len(captureTimes))
	for i, ts := range captureTimes {
		if i < 5 {
			frames = append(frames, makeStableFrame(fmt.Sprintf("seed-%d", i), ts, 20.0))
			continue
		}
		frames = append(frames, makeForegroundFrame(fmt.Sprintf("fg-%d", i), ts, 20.0, 5.0+0.1*float64(i-5)))
	}
	return frames
}

func runTimeDomainScene(t *testing.T, name string, frames []*l2frames.LiDARFrame) *timestampRecordingTracker {
	t.Helper()
	tracker := &timestampRecordingTracker{Tracker: l5tracks.NewTracker(l5tracks.DefaultTrackerConfig())}
	cfg := &TrackingPipelineConfig{
		SensorID:          "time-domain-" + name + "-" + t.Name(),
		BackgroundManager: makeTestBgManager(t, "time-domain-"+name+"-"+t.Name()),
		Tracker:           tracker,
		RemoveGround:      false,
	}
	cb := cfg.NewFrameCallback()
	for _, frame := range frames {
		cb(frame)
	}
	return tracker
}

// trackStates is every track's estimator state, in creation order, without
// the random public ID.
func trackStates(tk *l5tracks.Tracker) []string {
	tracks := tk.GetAllTracks()
	sort.Slice(tracks, func(i, j int) bool { return tracks[i].CreationSequence < tracks[j].CreationSequence })
	out := make([]string, 0, len(tracks))
	for _, tr := range tracks {
		out = append(out, fmt.Sprintf("%d %s hits=%d misses=%d x=%v y=%v vx=%v vy=%v P=%v start=%d end=%d state_t=%d observed_t=%d",
			tr.CreationSequence, tr.TrackState, tr.Hits, tr.Misses, tr.X, tr.Y, tr.VX, tr.VY, tr.P,
			tr.StartUnixNanos, tr.EndUnixNanos, tr.StateUnixNanos, tr.LastObservedUnixNanos))
	}
	return out
}

// Wall time jumps, capture time is regular. The frames carry host ingest
// times in StartWallTime/EndWallTime; here those leap an hour ahead, run
// backwards and stall, as they would across an NTP step or a paused replay.
// The tracker must receive exactly each frame's capture time and reach
// exactly the same tracks as when the wall clock ran evenly.
func TestTrackerIgnoresWallClockJumps(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	capture := make([]time.Time, 20)
	for i := range capture {
		capture[i] = base.Add(time.Duration(i) * 100 * time.Millisecond)
	}

	even := timeDomainScene(capture)
	jumpy := timeDomainScene(capture)
	wall := time.Unix(1_800_000_000, 0)
	for i := range even {
		even[i].StartWallTime = wall.Add(time.Duration(i) * 100 * time.Millisecond)
		even[i].EndWallTime = even[i].StartWallTime.Add(90 * time.Millisecond)
		var jump time.Duration
		switch {
		case i%3 == 0:
			jump = time.Hour
		case i%3 == 1:
			jump = -37 * time.Minute
		}
		jumpy[i].StartWallTime = wall.Add(jump)
		jumpy[i].EndWallTime = jumpy[i].StartWallTime
	}

	evenRun := runTimeDomainScene(t, "even", even)
	jumpyRun := runTimeDomainScene(t, "jumpy", jumpy)

	if len(evenRun.received) == 0 {
		t.Fatal("no frame reached the tracker; the scene no longer exercises tracking")
	}
	for _, run := range []*timestampRecordingTracker{evenRun, jumpyRun} {
		for i, ts := range run.received {
			if !ts.Equal(capture[len(capture)-len(run.received)+i]) {
				t.Fatalf("Update %d received %v, not the frame's capture time", i, ts)
			}
		}
	}
	want, got := trackStates(evenRun.Tracker), trackStates(jumpyRun.Tracker)
	if len(want) == 0 {
		t.Fatal("no tracks were created")
	}
	if fmt.Sprint(want) != fmt.Sprint(got) {
		t.Fatalf("wall-clock jumps changed the tracks:\n even %v\njumpy %v", want, got)
	}
}

// Capture time jumps, wall time is regular: the tracker must see the capture
// gap, whatever the host clock says. A 3 s hole in capture time is recorded
// unclamped and counted as clamped, although every frame arrived 100 ms of
// wall time after the last.
func TestTrackerSeesCaptureGapsUnderRegularWallTime(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	capture := make([]time.Time, 16)
	for i := range capture {
		capture[i] = base.Add(time.Duration(i) * 100 * time.Millisecond)
		if i >= 12 {
			capture[i] = capture[i].Add(3 * time.Second)
		}
	}
	frames := timeDomainScene(capture)
	wall := time.Unix(1_800_000_000, 0)
	for i := range frames {
		frames[i].StartWallTime = wall.Add(time.Duration(i) * 100 * time.Millisecond)
		frames[i].EndWallTime = frames[i].StartWallTime
	}

	run := runTimeDomainScene(t, "gap", frames)
	stats := run.Tracker.TimeDomainStats()
	if stats.ClampedGaps != 1 || math.Abs(stats.MaxGapSecs-3.1) > 1e-6 {
		t.Fatalf("the tracker did not see the 3.1 s capture gap: %+v", stats)
	}
}
