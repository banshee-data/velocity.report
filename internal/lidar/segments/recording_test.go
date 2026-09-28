package segments

import (
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

func recordedTrack(id string, x float32, state l9endpoints.TrackState, misses int) l9endpoints.Track {
	return l9endpoints.Track{TrackID: id, State: state, Misses: misses, X: x, VX: 10, MaxSpeedMps: 10}
}

// writeRecording records the frames as a replay would and returns the path.
func writeRecording(t *testing.T, frames [][]l9endpoints.Track) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "vrlog")
	rec, err := recorder.NewRecorder(dir, "segments-test")
	if err != nil {
		t.Fatal(err)
	}
	for i, tracks := range frames {
		ts := testBase + int64(i)*100_000_000
		bundle := &l9endpoints.FrameBundle{FrameID: uint64(i), TimestampNanos: ts, SensorID: "segments-test"}
		if tracks != nil {
			bundle.Tracks = &l9endpoints.TrackSet{FrameID: uint64(i), TimestampNanos: ts, Tracks: tracks}
		}
		if err := rec.Record(bundle); err != nil {
			t.Fatal(err)
		}
	}
	if err := rec.Close(); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRecordingKeepsConfirmedMeasuredTracks(t *testing.T) {
	confirmed, tentative, deleted := l9endpoints.TrackStateConfirmed, l9endpoints.TrackStateTentative, l9endpoints.TrackStateDeleted
	frames := [][]l9endpoints.Track{}
	for i := 0; i < 5; i++ {
		x := float32(i)
		follower := recordedTrack("follower", x, confirmed, 0)
		leader := recordedTrack("leader", x+10, confirmed, 0)
		// The leader's fastest moment comes late. Every one of its points
		// must carry it, or the speed gate would differ from frame to frame.
		if i == 4 {
			leader.MaxSpeedMps = 18
		}
		frames = append(frames, []l9endpoints.Track{
			follower, leader,
			recordedTrack("coasting", x+20, confirmed, 1),
			recordedTrack("unconfirmed", x+20, tentative, 0),
			recordedTrack("fading", x+20, deleted, 0),
			recordedTrack("", x+20, confirmed, 0),
		})
	}
	// A frame that carries no tracks is a frame, not a fault.
	frames = append(frames, nil)
	points, err := LoadRecording(writeRecording(t, frames), map[string]bool{"leader": true})
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 10 {
		t.Fatalf("want the five frames of the two measured tracks, got %d: %+v", len(points), points)
	}
	for _, p := range points {
		switch p.Track {
		case "follower":
			if p.SplitFlag || p.MaxSpeed != 10 {
				t.Fatalf("follower: %+v", p)
			}
		case "leader":
			if !p.SplitFlag || p.MaxSpeed != 18 {
				t.Fatalf("leader lost its mark or its lifetime speed: %+v", p)
			}
		default:
			t.Fatalf("a track that was not a confirmed measurement was kept: %+v", p)
		}
		if p.VX != 10 || p.TimeNs < testBase || p.TimeNs > testBase+400_000_000 {
			t.Fatalf("point lost its velocity or frame time: %+v", p)
		}
	}
	// The series ranks exactly as stored observations of the same tracks do.
	windows, err := Find(points, "following", "run", "tuning", DefaultParams(), nil)
	if err != nil || len(windows) != 1 || windows[0].PairFrames != 5 || windows[0].ClosestGapM != 10 {
		t.Fatalf("recording did not rank: %+v %v", windows, err)
	}
	flagged, err := Find(points, "split_flags", "run", "tuning", DefaultParams(), nil)
	if err != nil || len(flagged) != 1 || flagged[0].Events != 5 {
		t.Fatalf("marks were not carried into the ranking: %+v %v", flagged, err)
	}
}

type faultyFrames struct {
	frames []*l9endpoints.FrameBundle
	fault  error
	closed bool
}

func (f *faultyFrames) ReadFrame() (*l9endpoints.FrameBundle, error) {
	if len(f.frames) == 0 {
		return nil, f.fault
	}
	frame := f.frames[0]
	f.frames = f.frames[1:]
	return frame, nil
}

func (f *faultyFrames) Close() error {
	f.closed = true
	return nil
}

func TestRecordingFailuresAreReported(t *testing.T) {
	if _, err := LoadRecording("", nil); err == nil {
		t.Fatal("accepted an unnamed recording")
	}
	if _, err := LoadRecording(filepath.Join(t.TempDir(), "absent"), nil); err == nil || !strings.Contains(err.Error(), "open recording") {
		t.Fatalf("missing recording: %v", err)
	}
	frame := &l9endpoints.FrameBundle{TimestampNanos: testBase, Tracks: &l9endpoints.TrackSet{Tracks: []l9endpoints.Track{recordedTrack("a", 0, l9endpoints.TrackStateConfirmed, 0)}}}
	broken := &faultyFrames{frames: []*l9endpoints.FrameBundle{frame}, fault: errors.New("chunk is damaged")}
	// Half a run ranks differently from the whole: a damaged recording is
	// refused, not read as far as it goes.
	if points, err := readRecording(broken, nil); err == nil || !strings.Contains(err.Error(), "frame 1") || points != nil {
		t.Fatalf("damaged recording: %+v %v", points, err)
	}
	if !broken.closed {
		t.Fatal("recording was left open after a failure")
	}
	sparse := &faultyFrames{frames: []*l9endpoints.FrameBundle{nil, {TimestampNanos: testBase}, frame}, fault: io.EOF}
	points, err := readRecording(sparse, nil)
	if err != nil || len(points) != 1 || points[0].Track != "a" || !sparse.closed {
		t.Fatalf("frames without tracks: %+v %v", points, err)
	}
	// A track that reports no lifetime maximum still has its own speed.
	slow := recordedTrack("b", 0, l9endpoints.TrackStateConfirmed, 0)
	slow.MaxSpeedMps, slow.VX, slow.VY = 0, 3, 4
	one := &faultyFrames{frames: []*l9endpoints.FrameBundle{{TimestampNanos: testBase, Tracks: &l9endpoints.TrackSet{Tracks: []l9endpoints.Track{slow}}}}, fault: io.EOF}
	if points, err = readRecording(one, nil); err != nil || len(points) != 1 || points[0].MaxSpeed != 5 {
		t.Fatalf("speed from velocity: %+v %v", points, err)
	}
}

func TestRunFlagsAreScopedToTheRun(t *testing.T) {
	db := fixtureDB(t, `CREATE TABLE lidar_run_tracks(run_id TEXT,track_id TEXT,is_split_candidate INTEGER,is_merge_candidate INTEGER);
INSERT INTO lidar_run_tracks VALUES('run','split',1,0),('run','merge',0,1),('run','plain',0,0),('run','unset',NULL,NULL),('other','elsewhere',1,1);`)
	flagged, err := RunFlags(db, "run")
	if err != nil {
		t.Fatal(err)
	}
	if len(flagged) != 2 || !flagged["split"] || !flagged["merge"] {
		t.Fatalf("flags: %+v", flagged)
	}
	if _, err := RunFlags(db, ""); err == nil {
		t.Fatal("accepted an unnamed run")
	}
	recording := writeRecording(t, [][]l9endpoints.Track{{
		recordedTrack("split", 0, l9endpoints.TrackStateConfirmed, 0),
		recordedTrack("plain", 10, l9endpoints.TrackStateConfirmed, 0),
	}})
	points, err := LoadRunRecording(db, "run", recording)
	if err != nil || len(points) != 2 {
		t.Fatalf("run recording: %+v %v", points, err)
	}
	for _, p := range points {
		if p.SplitFlag != (p.Track == "split") {
			t.Fatalf("mark was not carried from the run's summaries: %+v", p)
		}
	}
	if _, err := db.Exec(`DROP TABLE lidar_run_tracks`); err != nil {
		t.Fatal(err)
	}
	if _, err := RunFlags(db, "run"); err == nil {
		t.Fatal("missing run tracks went unnoticed")
	}
	// Without the marks the split finder would rank nothing and say nothing.
	if _, err := LoadRunRecording(db, "run", recording); err == nil {
		t.Fatal("recording was read although the run's marks could not be")
	}
	// A track identity that cannot be read as text stops the read.
	blob := fixtureDB(t, `CREATE TABLE lidar_run_tracks(run_id TEXT,track_id,is_split_candidate INTEGER,is_merge_candidate INTEGER);
INSERT INTO lidar_run_tracks VALUES('run',NULL,1,0);`)
	if _, err := RunFlags(blob, "run"); err == nil {
		t.Fatal("unreadable track identity was accepted")
	}
}
