//go:build pcap && !race

package replayeval

import (
	"bytes"
	"path/filepath"
	"testing"
)

// TestReplayIsInvariantToWallClockPacing pins the estimator time-domain
// boundary on real data: the same kirk0 window, replayed unpaced and paced
// against the wall clock at two speeds, must produce the same tracks, frame
// for frame, and the same schema-2 tracking baseline, byte for byte.
//
// Pacing changes nothing about capture time and everything about wall time:
// how far apart packets reach the parser, how often the frame builder sees
// them, whether the reader yields to a lagging pipeline. So any path from the
// host clock into L2-L6 decisions (a time.Now() in a coast rule, frame
// assembly keyed on arrival time, a wall-clock warm-up) shows up here as a
// divergence. The static half of the same guarantee is the l5tracks source
// scan, TestL5TracksDoesNotReadTheWallClock.
//
// The window carries vehicles above 5 m/s, so a wrong prediction interval
// would change associations rather than hide in a stationary scene. The paced
// speeds sit either side of the pipeline's throughput on a loaded development
// machine: the real-time arm keeps up, and the faster arm can fall behind and
// take the reader's backoff yields. Both stay far inside the 30 s deficit at
// which the paced reader forgives lateness; that path drops the packet it
// forgives on (see docs/lidar/architecture/time-domain-model.md), a wall-clock
// dependence of paced L1 replay rather than of the estimator, and the reason
// this test is excluded from race builds: the race detector slows the pipeline
// enough to reach it.
func TestReplayIsInvariantToWallClockPacing(t *testing.T) {
	dir := t.TempDir()
	type arm struct {
		name  string
		speed float64
	}
	arms := []arm{{"unpaced", 0}, {"paced-1x", 1}, {"paced-2x", 2}}

	var (
		wantFrames   []string
		wantBaseline []byte
		wantRead     int
	)
	for i, a := range arms {
		cfg := kirk0MovingWindow(t, filepath.Join(dir, a.name))
		runtime := defaultRuntime()
		runtime.pacedSpeed = a.speed
		res, err := run(cfg, runtime)
		if err != nil {
			t.Fatalf("%s: %v", a.name, err)
		}
		frames, trackFrames := trackFingerprint(t, cfg.OutDir)
		baseline := readBaseline(t, cfg.OutDir)
		t.Logf("%s: frames_read=%d recorded=%d track_frames=%d elapsed=%s", a.name,
			res.FramesRead, res.FramesRecorded, trackFrames, res.Elapsed)

		// kirk0's frame timestamps are strictly increasing. The tracker's
		// non-monotonic guard therefore never engages on it, which is part of
		// why that guard cannot have moved a committed baseline.
		if td := res.TimeDomain; td.BackwardTimestamps != 0 || td.DuplicateTimestamps != 0 {
			t.Fatalf("%s: kirk0 produced non-monotonic frame timestamps: %+v", a.name, td)
		}
		if i == 0 {
			if trackFrames == 0 {
				t.Fatal("the window recorded no tracks; it no longer exercises the tracker")
			}
			wantFrames, wantBaseline, wantRead = frames, baseline, res.FramesRead
			continue
		}
		if res.FramesRead != wantRead {
			t.Fatalf("%s: read %d frames, unpaced read %d", a.name, res.FramesRead, wantRead)
		}
		requireSameDecisions(t, a.name, wantFrames, frames)
		if !bytes.Equal(wantBaseline, baseline) {
			t.Fatalf("%s: tracking baseline differs from unpaced:\n%s\n%s", a.name, wantBaseline, baseline)
		}
	}
}
