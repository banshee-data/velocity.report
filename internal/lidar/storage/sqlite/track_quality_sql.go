package sqlite

import (
	"database/sql"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// trackQualityColumns lists lidar_tracks' 6 quality columns. They are written
// from the lifetime counters the tracker keeps on a TrackedObject, which the
// visualiser, the recorder and the classifier read too, not from
// ComputeQualityMetrics, which recounts them from the capped trail.
const trackQualityColumns = `track_length_meters, track_duration_secs,
	occlusion_count, max_occlusion_frames, spatial_coverage, noise_point_ratio`

// trackQualityUpsertSet extends trackMeasurementUpsertSet with the quality
// columns.
const trackQualityUpsertSet = `,
			track_length_meters = excluded.track_length_meters,
			track_duration_secs = excluded.track_duration_secs,
			occlusion_count = excluded.occlusion_count,
			max_occlusion_frames = excluded.max_occlusion_frames,
			spatial_coverage = excluded.spatial_coverage,
			noise_point_ratio = excluded.noise_point_ratio`

// trackQualityUpdateSet extends trackMeasurementUpdateSet with the quality
// columns.
const trackQualityUpdateSet = `,
		track_length_meters = ?,
		track_duration_secs = ?,
		occlusion_count = ?,
		max_occlusion_frames = ?,
		spatial_coverage = ?,
		noise_point_ratio = ?`

// trackSpan is a track's duration, first to last observation, and its spatial
// coverage over that span, each with whether it is defined: neither is
// without elapsed time, and coverage is not without an observation.
type trackSpan struct {
	durationSecs, coverage   float32
	hasDuration, hasCoverage bool
}

func spanOf(track *TrackedObject) trackSpan {
	var s trackSpan
	if track.EndUnixNanos > track.StartUnixNanos {
		s.durationSecs, s.hasDuration = float32(track.EndUnixNanos-track.StartUnixNanos)/1e9, true
		s.coverage, s.hasCoverage = l5tracks.SpatialCoverage(track.ObservationCount, s.durationSecs)
	}
	return s
}

// trackQualityArgs returns the 6 quality values. Duration and spatial
// coverage are NULL while undefined, and noise_point_ratio is always NULL:
// nothing computes a track's noise ratio until clustering counts noise points.
func trackQualityArgs(track *TrackedObject) []any {
	span := spanOf(track)
	var duration, coverage any
	if span.hasDuration {
		duration = span.durationSecs
	}
	if span.hasCoverage {
		coverage = span.coverage
	}
	return []any{track.TrackLengthMeters, duration, track.OcclusionCount, track.MaxOcclusionFrames, coverage, nil}
}

// scanTrackQualityDests returns scan destinations for the 6 quality columns
// and a function to call after a successful Scan. NULL reads as 0, which is
// what a row written before these columns were populated holds.
func scanTrackQualityDests(track *TrackedObject) (dests []any, apply func()) {
	var length, duration, coverage, noise sql.NullFloat64
	var occlusions, maxOcclusion sql.NullInt64
	dests = []any{&length, &duration, &occlusions, &maxOcclusion, &coverage, &noise}
	apply = func() {
		track.TrackLengthMeters = float32(length.Float64)
		track.TrackDurationSecs = float32(duration.Float64)
		track.OcclusionCount = int(occlusions.Int64)
		track.MaxOcclusionFrames = int(maxOcclusion.Int64)
		track.SpatialCoverage = float32(coverage.Float64)
		track.NoisePointRatio = float32(noise.Float64)
	}
	return dests, apply
}
