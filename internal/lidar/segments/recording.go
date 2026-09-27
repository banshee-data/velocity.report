package segments

import (
	"fmt"
	"io"
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// frameReader is the part of a VRLOG replayer the series reader needs.
type frameReader interface {
	ReadFrame() (*l9endpoints.FrameBundle, error)
	Close() error
}

// LoadRecording reads a run's series from its VRLOG recording.
//
// An analysis replay does not write lidar_track_observations: that table is
// the live track store, and a replay must not add to it. The recording is
// then the only place a replayed run keeps where its tracks were, frame by
// frame, so a run with no stored observations is read from here.
//
// It keeps what the observation table would have held: confirmed tracks that
// were matched to a cluster in the frame. A coasting track's position is a
// prediction, a tentative track may be noise, and a deleted one is drawn only
// so that it can fade.
func LoadRecording(path string, flagged map[string]bool) ([]Point, error) {
	if path == "" {
		return nil, fmt.Errorf("recording path is required")
	}
	replayer, err := recorder.NewReplayer(path)
	if err != nil {
		return nil, fmt.Errorf("open recording: %w", err)
	}
	return readRecording(replayer, flagged)
}

func readRecording(frames frameReader, flagged map[string]bool) ([]Point, error) {
	defer frames.Close()
	points := []Point{}
	// The finder's speed gate is a track's lifetime maximum, which a frame
	// can only report up to its own time, so it is settled after the read.
	fastest := map[string]float64{}
	for ordinal := 0; ; ordinal++ {
		frame, err := frames.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read recording frame %d: %w", ordinal, err)
		}
		if frame == nil || frame.Tracks == nil {
			continue
		}
		for _, track := range frame.Tracks.Tracks {
			if track.TrackID == "" || track.State != l9endpoints.TrackStateConfirmed || track.Misses != 0 {
				continue
			}
			speed := math.Max(float64(track.MaxSpeedMps), math.Hypot(float64(track.VX), float64(track.VY)))
			if finite(speed) && speed > fastest[track.TrackID] {
				fastest[track.TrackID] = speed
			}
			points = append(points, Point{
				Track: track.TrackID, TimeNs: frame.TimestampNanos,
				X: float64(track.X), Y: float64(track.Y), VX: float64(track.VX), VY: float64(track.VY),
				SplitFlag: flagged[track.TrackID],
			})
		}
	}
	for i := range points {
		points[i].MaxSpeed = fastest[points[i].Track]
	}
	return points, nil
}

// LoadRunRecording reads a replayed run's series from its recording, carrying
// the split and merge marks from the run's track summaries.
func LoadRunRecording(db *sqlite.SQLDB, runID, path string) ([]Point, error) {
	flagged, err := RunFlags(db, runID)
	if err != nil {
		return nil, err
	}
	return LoadRecording(path, flagged)
}

// RunFlags names the run's tracks that are marked as split or merge
// candidates. The marks live on the run's track summaries, which a replay
// does write, so they are read from the database even when the series is not.
func RunFlags(db *sqlite.SQLDB, runID string) (map[string]bool, error) {
	if runID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	rows, err := db.Query(`SELECT track_id FROM lidar_run_tracks WHERE run_id=? AND (COALESCE(is_split_candidate,0)!=0 OR COALESCE(is_merge_candidate,0)!=0) ORDER BY track_id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	flagged := map[string]bool{}
	for rows.Next() {
		var track string
		if err = rows.Scan(&track); err != nil {
			return nil, err
		}
		flagged[track] = true
	}
	return flagged, rows.Err()
}
