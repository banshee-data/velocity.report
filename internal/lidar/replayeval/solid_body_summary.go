package replayeval

import (
	"fmt"
	"math"
	"sort"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// SolidBodySummary is what a solid_body replay wrote, read back from its
// evidence database, label-free: how often the near-edge measurement placed
// the body, and whether the body it placed is steadier than the point
// estimate beside it. It is the per-case form of the state-estimation plan's
// G-GEO-1 criteria 1 and 2 for the shadow estimator (Section 5.3, Option A),
// and what a corpus run reports before the near-edge measurement feeds the
// tracker (docs/plans/lidar-near-edge-tracked-state-plan.md, S2.0).
//
// The anchor comparisons score the same track-frames under both estimates, so
// the difference between them is the observation model and nothing else. Their
// window counts can still differ slightly: a window is scored only when its own
// fitted speed clears the fit's floor.
type SolidBodySummary struct {
	PointEstimates int `json:"point_estimates"`
	SolidBodies    int `json:"solid_bodies"`
	// NearEdgeFixes counts rows updated by a near-edge fix; FixShare is that
	// over SolidBodies.
	NearEdgeFixes int     `json:"near_edge_fixes"`
	FixShare      float64 `json:"fix_share"`
	// Rows by measurement source, reference point, rank, face set (near-edge
	// rows only) and fallback reason. Lapses are the body_centre_lapsed rows.
	Sources    map[string]int `json:"sources"`
	References map[string]int `json:"references"`
	Ranks      map[string]int `json:"ranks"`
	Faces      map[string]int `json:"faces"`
	Fallbacks  map[string]int `json:"fallbacks"`
	Lapses     int            `json:"lapses"`
	// Per track, from its last row: accumulated width, width converged under
	// the default bounds at any row, and ever established.
	Tracks               int     `json:"tracks"`
	TracksWithWidth      int     `json:"tracks_with_accumulated_width"`
	TracksWidthConverged int     `json:"tracks_width_converged"`
	TracksEstablished    int     `json:"tracks_established"`
	WidthMedianMetres    float64 `json:"final_width_median_m"`
	LengthMedianMetres   float64 `json:"final_length_median_m"`
	// Anchor stability by l8analytics' five-point lateral fit: every point
	// estimate; the point estimates on frames where the solid body is on the
	// body centre; and the solid bodies on those same frames.
	AnchorPointsAll     l8analytics.LateralFitSummary `json:"anchor_point_estimates_all"`
	AnchorPointsCentred l8analytics.LateralFitSummary `json:"anchor_point_estimates_body_centre_frames"`
	AnchorBodiesCentred l8analytics.LateralFitSummary `json:"anchor_solid_bodies_body_centre_frames"`
	// The same two comparisons over steady runs only: each track's
	// body-centre rows split wherever a row that is not on the body centre
	// (a lapse, or the seed before the first fix) intervenes, and scored as
	// separate series. A window that spans a re-reference is in the
	// body-centre-frame figures above and not in these, so the difference
	// between them is what reference changes cost. Moving and scored counts
	// here are runs, not tracks.
	AnchorPointsSteady l8analytics.LateralFitSummary `json:"anchor_point_estimates_steady_runs"`
	AnchorBodiesSteady l8analytics.LateralFitSummary `json:"anchor_solid_bodies_steady_runs"`
	SteadyRuns         int                           `json:"steady_runs"`
	// Face-stable runs split the steady runs again wherever the set of faces
	// the near-edge fix used changes (a row with no fix is its own set). A
	// tail that is in the steady runs and not here is what a face appearing or
	// disappearing costs.
	AnchorPointsFaceStable l8analytics.LateralFitSummary `json:"anchor_point_estimates_face_stable_runs"`
	AnchorBodiesFaceStable l8analytics.LateralFitSummary `json:"anchor_solid_bodies_face_stable_runs"`
	FaceStableRuns         int                           `json:"face_stable_runs"`
	RetainedSamplePoints   int                           `json:"retained_sample_points,omitempty"`
}

// SummariseSolidBodies reads one replay's online point estimates and the
// solid bodies filed beside them. The solid bodies must be one version; point
// estimates of other versions are ignored, so a database holding several
// arms is read as the solid bodies' arm alone.
func SummariseSolidBodies(points []observationsqlite.TrackEstimate, bodies []observationsqlite.TrackSolidBody,
	bounds l5tracks.ConvergenceBounds) (SolidBodySummary, error) {
	s := SolidBodySummary{
		Sources: map[string]int{}, References: map[string]int{}, Ranks: map[string]int{},
		Faces: map[string]int{}, Fallbacks: map[string]int{},
	}
	if len(bodies) == 0 {
		return s, nil
	}
	first := bodies[0]
	for _, sb := range bodies {
		if sb.EstimatorID != first.EstimatorID || sb.ParamHash != first.ParamHash || sb.Stage != first.Stage {
			return s, fmt.Errorf("solid bodies mix versions %s/%s/%s and %s/%s/%s",
				first.EstimatorID, first.ParamHash, first.Stage, sb.EstimatorID, sb.ParamHash, sb.Stage)
		}
	}
	var version []observationsqlite.TrackEstimate
	for _, p := range points {
		if p.EstimatorID == first.EstimatorID && p.ParamHash == first.ParamHash && p.Stage == first.Stage {
			version = append(version, p)
		}
	}
	s.PointEstimates, s.SolidBodies = len(version), len(bodies)

	type trackState struct {
		last           l5tracks.SolidBodyEstimate
		widthConverged bool
		established    bool
	}
	tracks := map[int64]*trackState{}
	var order []int64
	type key struct{ seq, frame int64 }
	type centredRow struct {
		estimate     l5tracks.SolidBodyEstimate
		run, faceRun int
	}
	centred := map[key]centredRow{}
	// run numbers each track's contiguous body-centre rows; a row that is
	// not on the body centre ends the run.
	run := map[int64]int{}
	inRun := map[int64]bool{}
	// faceRun numbers each track's face-stable runs; lastFaces is the face
	// set of its previous body-centre row, with "none" for a row without a
	// fix.
	faceRun := map[int64]int{}
	lastFaces := map[int64]string{}
	for _, sb := range bodies {
		e, m := sb.Reading.Estimate, sb.Reading.Measurement
		s.Sources[sourceName(m.Source)]++
		s.References[e.Reference.String()]++
		s.Ranks[strconv.Itoa(m.Rank)]++
		if m.FallbackReason != "" {
			s.Fallbacks[m.FallbackReason]++
		}
		if m.FallbackReason == "body_centre_lapsed" {
			s.Lapses++
		}
		if m.Source == l5tracks.MeasurementNearEdgeCandidateV1 {
			s.NearEdgeFixes++
			s.Faces[m.Faces.String()]++
		}
		if e.Reference == l5tracks.ReferenceBodyCentre {
			faces := "none"
			if m.Source == l5tracks.MeasurementNearEdgeCandidateV1 {
				faces = m.Faces.String()
			}
			if !inRun[sb.CreationSequence] {
				run[sb.CreationSequence]++
				inRun[sb.CreationSequence] = true
				faceRun[sb.CreationSequence]++
			} else if faces != lastFaces[sb.CreationSequence] {
				faceRun[sb.CreationSequence]++
			}
			lastFaces[sb.CreationSequence] = faces
			centred[key{sb.CreationSequence, sb.FrameUnixNanos}] = centredRow{
				estimate: e, run: run[sb.CreationSequence], faceRun: faceRun[sb.CreationSequence],
			}
		} else {
			inRun[sb.CreationSequence] = false
		}
		ts := tracks[sb.CreationSequence]
		if ts == nil {
			ts = &trackState{}
			tracks[sb.CreationSequence] = ts
			order = append(order, sb.CreationSequence)
		}
		ts.last = e
		if e.Width.IsConverged(bounds.MaxDimensionSigmaMetres, bounds.MinAdmissibleFrames) {
			ts.widthConverged = true
		}
		if e.Estimation == l5tracks.EstimationEstablished {
			ts.established = true
		}
	}
	s.FixShare = float64(s.NearEdgeFixes) / float64(len(bodies))

	var widths, lengths []float64
	for _, seq := range order {
		ts := tracks[seq]
		s.Tracks++
		if ts.last.Width.Provenance == l5tracks.ProvenanceAccumulated {
			s.TracksWithWidth++
			widths = append(widths, float64(ts.last.Width.Metres))
		}
		if ts.last.Length.Provenance == l5tracks.ProvenanceAccumulated {
			lengths = append(lengths, float64(ts.last.Length.Metres))
		}
		if ts.widthConverged {
			s.TracksWidthConverged++
		}
		if ts.established {
			s.TracksEstablished++
		}
	}
	s.WidthMedianMetres, s.LengthMedianMetres = median(widths), median(lengths)

	maxSpeed := map[int64]float64{}
	for _, p := range version {
		if v := math.Hypot(float64(p.VX), float64(p.VY)); v > maxSpeed[p.CreationSequence] {
			maxSpeed[p.CreationSequence] = v
		}
	}
	var all, pointsCentred, bodiesCentred, pointsSteady, bodiesSteady, pointsFace, bodiesFace []l8analytics.LateralFitTrack
	allIndex, centredIndex := map[int64]int{}, map[int64]int{}
	type runKey struct {
		seq int64
		run int
	}
	steadyIndex, faceIndex := map[runKey]int{}, map[runKey]int{}
	for _, p := range version {
		id := strconv.FormatInt(p.CreationSequence, 10)
		i, ok := allIndex[p.CreationSequence]
		if !ok {
			i = len(all)
			allIndex[p.CreationSequence] = i
			all = append(all, l8analytics.LateralFitTrack{ID: id, MaxSpeedMps: maxSpeed[p.CreationSequence]})
		}
		all[i].Points = append(all[i].Points, l8analytics.SeriesPoint{TimestampNanos: p.FrameUnixNanos, X: p.X, Y: p.Y})

		row, ok := centred[key{p.CreationSequence, p.FrameUnixNanos}]
		if !ok {
			continue
		}
		e := row.estimate
		j, ok := centredIndex[p.CreationSequence]
		if !ok {
			j = len(pointsCentred)
			centredIndex[p.CreationSequence] = j
			pointsCentred = append(pointsCentred, l8analytics.LateralFitTrack{ID: id, MaxSpeedMps: maxSpeed[p.CreationSequence]})
			bodiesCentred = append(bodiesCentred, l8analytics.LateralFitTrack{ID: id, MaxSpeedMps: maxSpeed[p.CreationSequence]})
		}
		pointsCentred[j].Points = append(pointsCentred[j].Points, l8analytics.SeriesPoint{TimestampNanos: p.FrameUnixNanos, X: p.X, Y: p.Y})
		bodiesCentred[j].Points = append(bodiesCentred[j].Points, l8analytics.SeriesPoint{TimestampNanos: p.FrameUnixNanos, X: e.X, Y: e.Y})

		rk := runKey{p.CreationSequence, row.run}
		k, ok := steadyIndex[rk]
		if !ok {
			k = len(pointsSteady)
			steadyIndex[rk] = k
			runID := id + "." + strconv.Itoa(row.run)
			pointsSteady = append(pointsSteady, l8analytics.LateralFitTrack{ID: runID, MaxSpeedMps: maxSpeed[p.CreationSequence]})
			bodiesSteady = append(bodiesSteady, l8analytics.LateralFitTrack{ID: runID, MaxSpeedMps: maxSpeed[p.CreationSequence]})
		}
		pointsSteady[k].Points = append(pointsSteady[k].Points, l8analytics.SeriesPoint{TimestampNanos: p.FrameUnixNanos, X: p.X, Y: p.Y})
		bodiesSteady[k].Points = append(bodiesSteady[k].Points, l8analytics.SeriesPoint{TimestampNanos: p.FrameUnixNanos, X: e.X, Y: e.Y})

		fk := runKey{p.CreationSequence, row.faceRun}
		f, ok := faceIndex[fk]
		if !ok {
			f = len(pointsFace)
			faceIndex[fk] = f
			runID := id + ".f" + strconv.Itoa(row.faceRun)
			pointsFace = append(pointsFace, l8analytics.LateralFitTrack{ID: runID, MaxSpeedMps: maxSpeed[p.CreationSequence]})
			bodiesFace = append(bodiesFace, l8analytics.LateralFitTrack{ID: runID, MaxSpeedMps: maxSpeed[p.CreationSequence]})
		}
		pointsFace[f].Points = append(pointsFace[f].Points, l8analytics.SeriesPoint{TimestampNanos: p.FrameUnixNanos, X: p.X, Y: p.Y})
		bodiesFace[f].Points = append(bodiesFace[f].Points, l8analytics.SeriesPoint{TimestampNanos: p.FrameUnixNanos, X: e.X, Y: e.Y})
	}
	s.AnchorPointsAll = l8analytics.SummariseLateralFit(all)
	s.AnchorPointsCentred = l8analytics.SummariseLateralFit(pointsCentred)
	s.AnchorBodiesCentred = l8analytics.SummariseLateralFit(bodiesCentred)
	s.AnchorPointsSteady = l8analytics.SummariseLateralFit(pointsSteady)
	s.AnchorBodiesSteady = l8analytics.SummariseLateralFit(bodiesSteady)
	s.SteadyRuns = len(bodiesSteady)
	s.AnchorPointsFaceStable = l8analytics.SummariseLateralFit(pointsFace)
	s.AnchorBodiesFaceStable = l8analytics.SummariseLateralFit(bodiesFace)
	s.FaceStableRuns = len(bodiesFace)
	return s, nil
}

// SummariseSolidBodyEvidence reads a replay's point estimates and solid
// bodies for one observation source from an evidence database.
func SummariseSolidBodyEvidence(database observationsqlite.DBClient, sourceID string) (SolidBodySummary, error) {
	store := observationsqlite.NewStateEstimateStore(database)
	points, err := store.ListBySource(sourceID)
	if err != nil {
		return SolidBodySummary{}, err
	}
	bodies, err := store.ListSolidBodiesBySource(sourceID)
	if err != nil {
		return SolidBodySummary{}, err
	}
	return SummariseSolidBodies(points, bodies, l5tracks.DefaultConvergenceBounds())
}

func sourceName(m l5tracks.MeasurementSource) string {
	if m == "" {
		return "none"
	}
	return string(m)
}

// median is the middle value, the lower of the two for an even count, and
// zero for none, so a summary never carries NaN into JSON.
func median(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[(len(s)-1)/2]
}
