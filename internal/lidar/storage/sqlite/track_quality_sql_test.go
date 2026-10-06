package sqlite

import (
	"database/sql"
	"testing"
)

func qualityTrack(id string) *TrackedObject {
	const second = int64(1_000_000_000)
	start := int64(1_700_000_000) * second
	track := &TrackedObject{TrackID: id, TrackMeasurement: TrackMeasurement{
		SensorID: "sensor-q", TrackState: TrackConfirmed, StartUnixNanos: start, EndUnixNanos: start + 2*second,
		ObservationCount: 10,
	}}
	track.TrackLengthMeters = 42.5
	// 6 missed frames in gaps it was seen again after, the longest 4; the
	// live counters also hold the 14-frame coast it is leaving on.
	track.ClosedOcclusionCount, track.MaxClosedOcclusionFrames = 6, 4
	track.OcclusionCount, track.MaxOcclusionFrames = 20, 14
	return track
}

// The tracker's lifetime counters are stored and read back, occlusions as
// the closed ones, with duration and coverage from the span: 10 observations
// in 2 s at 10 Hz cover 0.5.
func TestTrackQualityColumnsRoundTrip(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	if err := InsertTrack(db, qualityTrack("q-1"), "site/main"); err != nil {
		t.Fatal(err)
	}
	tracks, err := GetActiveTracks(db, "sensor-q", "")
	if err != nil || len(tracks) != 1 {
		t.Fatalf("GetActiveTracks: %v, %d tracks", err, len(tracks))
	}
	got := tracks[0]
	if got.TrackLengthMeters != 42.5 || got.TrackDurationSecs != 2 || got.OcclusionCount != 6 ||
		got.MaxOcclusionFrames != 4 || got.SpatialCoverage != 0.5 {
		t.Fatalf("read back length %v, duration %v, occlusions %d/%d, coverage %v",
			got.TrackLengthMeters, got.TrackDurationSecs, got.OcclusionCount, got.MaxOcclusionFrames, got.SpatialCoverage)
	}

	// The next frame's upsert replaces them.
	next := qualityTrack("q-1")
	next.TrackLengthMeters, next.ClosedOcclusionCount = 60, 7
	if err := InsertTrack(db, next, "site/main"); err != nil {
		t.Fatal(err)
	}
	inRange, err := GetTracksInRange(db, "sensor-q", "", 0, next.EndUnixNanos, 10)
	if err != nil || len(inRange) != 1 {
		t.Fatalf("GetTracksInRange: %v, %d tracks", err, len(inRange))
	}
	if inRange[0].TrackLengthMeters != 60 || inRange[0].OcclusionCount != 7 {
		t.Fatalf("upsert left length %v, occlusions %d", inRange[0].TrackLengthMeters, inRange[0].OcclusionCount)
	}
}

// Duration and coverage are NULL until defined, and noise_point_ratio is
// NULL because nothing computes it: a stored 0 would read as a measurement.
func TestTrackQualityColumnsUndefinedAreNull(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	track := qualityTrack("q-new")
	track.EndUnixNanos = track.StartUnixNanos
	if err := InsertTrack(db, track, "site/main"); err != nil {
		t.Fatal(err)
	}
	var duration, coverage, noise sql.NullFloat64
	if err := db.QueryRow(`SELECT track_duration_secs, spatial_coverage, noise_point_ratio FROM lidar_tracks WHERE track_id = 'q-new'`).
		Scan(&duration, &coverage, &noise); err != nil {
		t.Fatal(err)
	}
	if duration.Valid || coverage.Valid || noise.Valid {
		t.Fatalf("duration %v, coverage %v, noise %v; want all NULL", duration, coverage, noise)
	}
}

// A track read from the database and updated keeps its quality values, so
// a classification edit does not reset them; rows written before the
// columns were populated read as 0.
func TestUpdateTrackKeepsQualityAndLegacyRowsRead(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	if err := InsertTrack(db, qualityTrack("q-edit"), "site/main"); err != nil {
		t.Fatal(err)
	}
	// A legacy row: the measurement columns written, the quality ones NULL.
	if err := InsertTrack(db, qualityTrack("q-legacy"), "site/main"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE lidar_tracks SET track_length_meters = NULL, track_duration_secs = NULL,
		occlusion_count = NULL, max_occlusion_frames = NULL, spatial_coverage = NULL WHERE track_id = 'q-legacy'`); err != nil {
		t.Fatal(err)
	}
	tracks, err := GetActiveTracks(db, "sensor-q", "")
	if err != nil || len(tracks) != 2 {
		t.Fatalf("GetActiveTracks: %v, %d tracks", err, len(tracks))
	}
	byID := map[string]*TrackedObject{}
	for _, tr := range tracks {
		byID[tr.TrackID] = tr
	}
	if legacy := byID["q-legacy"]; legacy.TrackLengthMeters != 0 || legacy.SpatialCoverage != 0 {
		t.Fatalf("legacy row read length %v, coverage %v", legacy.TrackLengthMeters, legacy.SpatialCoverage)
	}
	edit := byID["q-edit"]
	edit.ObjectClass = "car"
	if err := UpdateTrack(db, edit); err != nil {
		t.Fatal(err)
	}
	var length float64
	var occlusions int
	if err := db.QueryRow(`SELECT track_length_meters, occlusion_count FROM lidar_tracks WHERE track_id = 'q-edit'`).
		Scan(&length, &occlusions); err != nil {
		t.Fatal(err)
	}
	if length != 42.5 || occlusions != 6 {
		t.Fatalf("after update: length %v, occlusions %d; want 42.5, 6", length, occlusions)
	}
}
