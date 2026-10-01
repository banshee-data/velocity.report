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
	// ReReferences counts rows whose reference is the body centre where the
	// same track's previous row was referenced to the medoid: the first fix
	// after a seed or a lapse.
	ReReferences int `json:"re_references"`
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
	// FaceStableStrata splits the face-stable residuals by the centre frame's
	// heading rate and range, so a tail that face stability does not explain
	// can be attributed. Both come from the solid body on that frame, for the
	// point estimate's windows as well, so the two are split by the same
	// frames.
	FaceStableStrata []AnchorStratum `json:"face_stable_strata,omitempty"`
	// SteadyTransitions attributes the steady runs' body tail to what changed
	// inside each window, so the difference between the steady and
	// face-stable figures is named rather than inferred.
	SteadyTransitions    *TransitionAnatomy `json:"steady_run_transitions,omitempty"`
	RetainedSamplePoints int                `json:"retained_sample_points,omitempty"`
}

// TransitionAnatomy is the solid bodies' five-point windows on steady runs,
// grouped by what changed between consecutive rows inside each window. A
// window can carry several changes, so the groups overlap; a window with none
// is stable. The tail is the windows at or above the steady runs' p95.
type TransitionAnatomy struct {
	Windows         int               `json:"windows"`
	P99Metres       float64           `json:"p99_m"`
	TailMetres      float64           `json:"tail_threshold_m"`
	StableWindows   int               `json:"stable_windows"`
	StableP99Metres float64           `json:"stable_p99_m"`
	Transitions     []TransitionShare `json:"transitions"`
}

// TransitionShare is one kind of change: the windows that carry it, those of
// them in the tail, and the p99 of every window that does not carry it, which
// is what the steady p99 would be if this change cost nothing.
type TransitionShare struct {
	Transition       string  `json:"transition"`
	Windows          int     `json:"windows"`
	TailWindows      int     `json:"tail_windows"`
	P99WithoutMetres float64 `json:"p99_without_m"`
}

// The changes a window is grouped by. A face enters when it is in a fix and
// was not in the previous row's, and swaps when the face opposite it was; it
// leaves when it was in the previous fix and neither it nor its opposite is
// in the current fix.
// Faceless is a row without a fix beside one with. Revised is a change in
// the believed dimension the half-extents come from.
const (
	TransitionFaceless               = "faceless"
	TransitionLateralFaceEnters      = "lateral_face_enters"
	TransitionLongitudinalFaceEnters = "longitudinal_face_enters"
	TransitionFaceLeaves             = "face_leaves"
	TransitionFaceSwaps              = "face_swaps"
	TransitionWidthRevised           = "width_revised"
	TransitionLengthRevised          = "length_revised"
)

var transitionOrder = []string{
	TransitionFaceless, TransitionLateralFaceEnters, TransitionLongitudinalFaceEnters,
	TransitionFaceLeaves, TransitionFaceSwaps, TransitionWidthRevised, TransitionLengthRevised,
}

// transitionRow is what the anatomy needs to know about one body-centre row.
type transitionRow struct {
	fix           bool
	faces         l5tracks.VisibleFaces
	width, length float32
}

// oppositeFace is the face on the other side of the same axis.
func oppositeFace(f l5tracks.BodyFace) l5tracks.BodyFace {
	switch f {
	case l5tracks.FaceFront:
		return l5tracks.FaceRear
	case l5tracks.FaceRear:
		return l5tracks.FaceFront
	case l5tracks.FaceLeft:
		return l5tracks.FaceRight
	default:
		return l5tracks.FaceLeft
	}
}

// rowTransitions is the set of changes between two consecutive rows.
func rowTransitions(a, b transitionRow, into map[string]bool) {
	if a.fix != b.fix {
		into[TransitionFaceless] = true
	}
	var before, after l5tracks.VisibleFaces
	if a.fix {
		before = a.faces
	}
	if b.fix {
		after = b.faces
	}
	for _, f := range []l5tracks.BodyFace{l5tracks.FaceFront, l5tracks.FaceRear, l5tracks.FaceLeft, l5tracks.FaceRight} {
		switch {
		case after.Has(f) && !before.Has(f):
			switch {
			case before.Has(oppositeFace(f)):
				into[TransitionFaceSwaps] = true
			case f.IsLongitudinal():
				into[TransitionLongitudinalFaceEnters] = true
			default:
				into[TransitionLateralFaceEnters] = true
			}
		case before.Has(f) && !after.Has(f) && !after.Has(oppositeFace(f)):
			into[TransitionFaceLeaves] = true
		}
	}
	if a.width != b.width {
		into[TransitionWidthRevised] = true
	}
	if a.length != b.length {
		into[TransitionLengthRevised] = true
	}
}

// transitionAnatomy groups the moving tracks' windows of the body series by
// the changes inside each, reading rows, the series' own rows in order.
func transitionAnatomy(bodies []l8analytics.LateralFitTrack, rows [][]transitionRow) *TransitionAnatomy {
	type scored struct {
		residual float64
		changes  map[string]bool
	}
	var windows []scored
	for i, tr := range bodies {
		if tr.MaxSpeedMps < l8analytics.LateralFitMovingMinSpeedMps {
			continue
		}
		for _, w := range l8analytics.LateralFitWindows(tr.Points) {
			// A scored window is five consecutive rows about its centre.
			changes := map[string]bool{}
			for j := w.Centre - 2; j < w.Centre+2; j++ {
				rowTransitions(rows[i][j], rows[i][j+1], changes)
			}
			windows = append(windows, scored{w.Residual, changes})
		}
	}
	if len(windows) == 0 {
		return nil
	}
	all := make([]float64, len(windows))
	var stable []float64
	for i, w := range windows {
		all[i] = w.residual
		if len(w.changes) == 0 {
			stable = append(stable, w.residual)
		}
	}
	overall := l8analytics.SummariseLateralResiduals(append([]float64(nil), all...))
	a := &TransitionAnatomy{
		Windows: overall.Windows, P99Metres: overall.P99Metres, TailMetres: overall.P95Metres,
		StableWindows: len(stable), StableP99Metres: l8analytics.SummariseLateralResiduals(stable).P99Metres,
	}
	for _, name := range transitionOrder {
		share := TransitionShare{Transition: name}
		var without []float64
		for _, w := range windows {
			if !w.changes[name] {
				without = append(without, w.residual)
				continue
			}
			share.Windows++
			if w.residual >= a.TailMetres {
				share.TailWindows++
			}
		}
		share.P99WithoutMetres = l8analytics.SummariseLateralResiduals(without).P99Metres
		a.Transitions = append(a.Transitions, share)
	}
	return a
}

// AnchorStratum is one bin of an anchor comparison: its axis ("heading_rate"
// or "range"), the bin, and the two estimates' residuals within it.
type AnchorStratum struct {
	Axis   string                             `json:"axis"`
	Bin    string                             `json:"bin"`
	Points l8analytics.LateralResidualSummary `json:"point_estimates"`
	Bodies l8analytics.LateralResidualSummary `json:"solid_bodies"`
}

// Strata bins. Heading rate is the solid body's orientation change across the
// five-point window, folded as an axis, in degrees per second: the tracked
// heading's lag on a turn is what tilts a face normal (the state plan's
// invalidating condition b). Range is the body centre's distance from the
// sensor, which is the replay frame's origin.
const (
	StratumHeadingRate = "heading_rate"
	StratumRange       = "range"
)

var (
	headingRateBins = []string{"lt_5_deg_s", "5_to_15_deg_s", "ge_15_deg_s", "unknown"}
	rangeBins       = []string{"lt_20_m", "20_to_40_m", "ge_40_m"}
)

func headingRateBin(degPerSec float64, known bool) string {
	switch {
	case !known:
		return "unknown"
	case degPerSec < 5:
		return "lt_5_deg_s"
	case degPerSec < 15:
		return "5_to_15_deg_s"
	default:
		return "ge_15_deg_s"
	}
}

func rangeBin(metres float64) string {
	switch {
	case metres < 20:
		return "lt_20_m"
	case metres < 40:
		return "20_to_40_m"
	default:
		return "ge_40_m"
	}
}

// stratumSample is what a stratum needs to know about one frame.
type stratumSample struct {
	psiRad    float32
	headingOK bool
	rangeM    float64
}

// anchorStrata bins the residuals of paired point and body series, whose
// samples are the same frames, by each window's centre frame.
func anchorStrata(points, bodies []l8analytics.LateralFitTrack, samples [][]stratumSample) []AnchorStratum {
	type binKey struct{ axis, bin string }
	collect := func(tracks []l8analytics.LateralFitTrack) map[binKey][]float64 {
		out := map[binKey][]float64{}
		for i, tr := range tracks {
			if tr.MaxSpeedMps < l8analytics.LateralFitMovingMinSpeedMps {
				continue
			}
			for _, w := range l8analytics.LateralFitWindows(tr.Points) {
				// A scored window is five consecutive samples, so its ends
				// are two either side of the centre.
				first, last := samples[i][w.Centre-2], samples[i][w.Centre+2]
				dt := float64(tr.Points[w.Centre+2].TimestampNanos-tr.Points[w.Centre-2].TimestampNanos) / 1e9
				known := first.headingOK && last.headingOK && dt > 0
				var rate float64
				if known {
					rate = l5tracks.FoldAxisAngleDeg(float64(last.psiRad)-float64(first.psiRad)) / dt
				}
				hk := binKey{StratumHeadingRate, headingRateBin(rate, known)}
				rk := binKey{StratumRange, rangeBin(samples[i][w.Centre].rangeM)}
				out[hk] = append(out[hk], w.Residual)
				out[rk] = append(out[rk], w.Residual)
			}
		}
		return out
	}
	p, b := collect(points), collect(bodies)
	var out []AnchorStratum
	for _, axis := range []struct {
		name string
		bins []string
	}{{StratumHeadingRate, headingRateBins}, {StratumRange, rangeBins}} {
		for _, bin := range axis.bins {
			k := binKey{axis.name, bin}
			out = append(out, AnchorStratum{
				Axis: axis.name, Bin: bin,
				Points: l8analytics.SummariseLateralResiduals(p[k]),
				Bodies: l8analytics.SummariseLateralResiduals(b[k]),
			})
		}
	}
	return out
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
		transition   transitionRow
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
	// lastReference is each track's previous row's reference.
	lastReference := map[int64]l5tracks.ReferencePoint{}
	for _, sb := range bodies {
		e, m := sb.Reading.Estimate, sb.Reading.Measurement
		if previous, ok := lastReference[sb.CreationSequence]; ok &&
			previous == l5tracks.ReferenceClusterMedoid && e.Reference == l5tracks.ReferenceBodyCentre {
			s.ReReferences++
		}
		lastReference[sb.CreationSequence] = e.Reference
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
				transition: transitionRow{
					fix:   m.Source == l5tracks.MeasurementNearEdgeCandidateV1,
					faces: m.Faces, width: e.Width.Metres, length: e.Length.Metres,
				},
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
	var faceSamples [][]stratumSample
	var steadyRows [][]transitionRow
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
			steadyRows = append(steadyRows, nil)
		}
		steadyRows[k] = append(steadyRows[k], row.transition)
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
			faceSamples = append(faceSamples, nil)
		}
		faceSamples[f] = append(faceSamples[f], stratumSample{
			psiRad:    e.Orientation.PsiRad,
			headingOK: e.Orientation.Provenance != l5tracks.ProvenanceNone,
			rangeM:    math.Hypot(float64(e.X), float64(e.Y)),
		})
		pointsFace[f].Points = append(pointsFace[f].Points, l8analytics.SeriesPoint{TimestampNanos: p.FrameUnixNanos, X: p.X, Y: p.Y})
		bodiesFace[f].Points = append(bodiesFace[f].Points, l8analytics.SeriesPoint{TimestampNanos: p.FrameUnixNanos, X: e.X, Y: e.Y})
	}
	s.AnchorPointsAll = l8analytics.SummariseLateralFit(all)
	s.AnchorPointsCentred = l8analytics.SummariseLateralFit(pointsCentred)
	s.AnchorBodiesCentred = l8analytics.SummariseLateralFit(bodiesCentred)
	s.AnchorPointsSteady = l8analytics.SummariseLateralFit(pointsSteady)
	s.AnchorBodiesSteady = l8analytics.SummariseLateralFit(bodiesSteady)
	s.SteadyRuns = len(bodiesSteady)
	s.SteadyTransitions = transitionAnatomy(bodiesSteady, steadyRows)
	s.AnchorPointsFaceStable = l8analytics.SummariseLateralFit(pointsFace)
	s.AnchorBodiesFaceStable = l8analytics.SummariseLateralFit(bodiesFace)
	s.FaceStableRuns = len(bodiesFace)
	s.FaceStableStrata = anchorStrata(pointsFace, bodiesFace, faceSamples)
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
