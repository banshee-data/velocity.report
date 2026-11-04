// Package lanefieldrun builds a stored run's lane field
// (docs/plans/lidar-lane-proposals-from-tracks-plan.md §1) from what the run
// left behind: the store's track observations when it holds them, else the
// run's VRLOG. An analysis replay, which is how archive captures and scene
// runs are made, never writes track observations
// (TrackingPipelineConfig.DisableTrackPersistence), so its recording is the
// only record of where its tracks were.
//
// Both sources give the same instants: a VRLOG frame's track with no miss is
// an instant a cluster was associated, which is exactly when the store would
// have written an observation, and the frame's box is the published box when
// the track had one, else the cluster's oriented box, as the store's columns
// are read. The tracks counted are the run's own (lidar_run_tracks, which
// holds the tracks it confirmed), with their final L6 class.
//
// The HTTP route and `velocity lidar lane-field` both call Build, so the two
// write the same bytes for the same run.
package lanefieldrun

import (
	"errors"
	"fmt"
	"io"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Source names where a field's instants were read from.
const (
	SourceObservations = "observations"
	SourceVRLOG        = "vrlog"
	SourceNone         = "none"
)

// ResolveVRLOG maps a run's stored VRLOG path to one that may be read, or
// fails; a server holds it to its VRLOG directory, the CLI may point it at a
// copy.
type ResolveVRLOG func(stored string) (string, error)

// ErrNoRecording is returned when a run has no observations and its
// recording cannot be read.
var ErrNoRecording = errors.New("the run stored no track observations and its recording cannot be read")

// Build returns one part of the run's lane field and the source its
// instants came from. It returns sqlite.ErrNotFound for a missing run. A run
// with neither observations nor a recording gives an empty field with its
// header; one whose recording is named but unreadable gives ErrNoRecording.
func Build(store *sqlite.RunStore, runID string, part l8analytics.LaneFieldPart, resolve ResolveVRLOG) (l8analytics.LaneField, string, error) {
	run, err := store.GetRun(runID)
	if err != nil {
		return l8analytics.LaneField{}, "", err
	}
	tracks, source, err := Tracks(store, run, resolve)
	if err != nil {
		return l8analytics.LaneField{}, source, err
	}
	f, err := l8analytics.BuildLaneField(tracks, sqlite.LaneFieldMeta(run), part, l8analytics.DefaultLaneFieldOptions())
	return f, source, err
}

// Tracks reads the run's tracks: the store's observations when there are
// any, else the run's VRLOG.
func Tracks(store *sqlite.RunStore, run *sqlite.Run, resolve ResolveVRLOG) ([]l8analytics.LaneFieldTrack, string, error) {
	tracks, err := store.LaneFieldTracks(run)
	if err != nil {
		return nil, "", err
	}
	if len(tracks) > 0 {
		return tracks, SourceObservations, nil
	}
	if run.VRLogPath == "" {
		return nil, SourceNone, nil
	}
	if resolve == nil {
		return nil, SourceVRLOG, ErrNoRecording
	}
	path, err := resolve(run.VRLogPath)
	if err != nil {
		// Wrap both, so a caller can still read the resolver's own error
		// (the server's HTTP status) with errors.As.
		return nil, SourceVRLOG, fmt.Errorf("%w: %w", ErrNoRecording, err)
	}
	classes, err := store.RunTrackClasses(run.RunID)
	if err != nil {
		return nil, SourceVRLOG, err
	}
	tracks, err = FromVRLOG(path, classes)
	if err != nil {
		return nil, SourceVRLOG, fmt.Errorf("%w: %v", ErrNoRecording, err)
	}
	return tracks, SourceVRLOG, nil
}

// FromVRLOG reads the observed instants of the given tracks (id to L6
// class) from a recording: each frame's tracks with no miss.
func FromVRLOG(path string, classes map[string]string) ([]l8analytics.LaneFieldTrack, error) {
	rep, err := recorder.NewReplayer(path)
	if err != nil {
		return nil, err
	}
	defer rep.Close()
	byID := map[string]*l8analytics.LaneFieldTrack{}
	for {
		frame, err := rep.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if frame == nil || frame.Tracks == nil {
			continue
		}
		ts := frame.Tracks.TimestampNanos
		if ts == 0 {
			ts = frame.TimestampNanos
		}
		for _, t := range frame.Tracks.Tracks {
			class, ok := classes[t.TrackID]
			if !ok || t.Misses != 0 {
				continue
			}
			tr := byID[t.TrackID]
			if tr == nil {
				tr = &l8analytics.LaneFieldTrack{TrackID: t.TrackID, Class: class}
				byID[t.TrackID] = tr
			}
			in := l8analytics.LaneFieldInstant{
				TimeNs: ts, X: float64(t.X), Y: float64(t.Y),
				VX: float64(t.VX), VY: float64(t.VY), SpeedMps: float64(t.SpeedMps),
				BoxX: float64(t.X), BoxY: float64(t.Y), BoxHeadingRad: float64(t.BBoxHeadingRad),
				BoxLength: float64(t.BBoxLength), BoxWidth: float64(t.BBoxWidth),
			}
			if t.BBoxCenterValid {
				in.BoxX, in.BoxY = float64(t.BBoxCenterX), float64(t.BBoxCenterY)
			}
			tr.Instants = append(tr.Instants, in)
		}
	}
	ids := make([]string, 0, len(byID))
	for id := range byID {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]l8analytics.LaneFieldTrack, 0, len(ids))
	for _, id := range ids {
		out = append(out, *byID[id])
	}
	return out, nil
}
