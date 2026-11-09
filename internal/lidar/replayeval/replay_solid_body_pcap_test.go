//go:build pcap && !race

package replayeval

// kirk0 evidence for the solid_body experiment. Label-free: it proves the
// wiring end to end and records row counts, extent convergence and anchor
// stability on this one capture. It is not Gate G-GEO-1, which needs the
// held-out geometry and manoeuvre labels a corpus run supplies; the command
// for that run is in the state-estimation plan's checklist.
//
// Excluded under -race: two full kirk0 replays with evidence storage are
// minutes of work there, and the race detector adds nothing to what the
// l5tracks and storage unit tests already cover.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// solidBodyArm is one replay of the kirk0 scoring window with evidence
// storage, and what it wrote.
type solidBodyArm struct {
	cfg      Config
	result   *Result
	points   []observationsqlite.TrackEstimate
	bodies   []observationsqlite.TrackSolidBody
	baseline []byte
	params   string
}

func runSolidBodyArm(t *testing.T, dir, name string, experiments []string) solidBodyArm {
	t.Helper()
	return runSolidBodyArmTapped(t, dir, name, experiments, nil)
}

// runSolidBodyArmTapped is runSolidBodyArm with a tracker tap.
func runSolidBodyArmTapped(t *testing.T, dir, name string, experiments []string, tap func(TrackerTapFrame)) solidBodyArm {
	t.Helper()
	cfg := Config{
		TrackerTap: tap,
		PCAPFile:   requireKirk0(t), OutDir: filepath.Join(dir, name), SensorID: "test-replay", UDPPort: 2369,
		// The whole capture after the settled 20 s prefix.
		StartSeconds: 20, WarmupSeconds: 20, RequireSettled: true,
		ObservationDBPath: filepath.Join(dir, name+".db"), ClipID: "kirk0-solid-body",
		// The corpus runner's retained-point cap, so these figures describe
		// the evidence a G-GEO-1 corpus run will read.
		ObservationMaxSamplePoints: 256,
		Experiments:                experiments,
	}
	cfg.ObservationCalibration = identityReplayCalibration(cfg.SensorID)
	result, err := Run(cfg)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	arm := solidBodyArm{cfg: cfg, result: result, baseline: readBaseline(t, cfg.OutDir)}
	var manifest struct {
		Params string `json:"params_sha256"`
	}
	b, err := os.ReadFile(filepath.Join(cfg.OutDir, "replay_manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &manifest); err != nil {
		t.Fatal(err)
	}
	arm.params = manifest.Params

	database, err := db.NewDB(cfg.ObservationDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	store := observationsqlite.NewStateEstimateStore(database)
	if arm.points, err = store.ListBySource(result.ObservationSourceID); err != nil {
		t.Fatal(err)
	}
	if arm.bodies, err = store.ListSolidBodiesBySource(result.ObservationSourceID); err != nil {
		t.Fatal(err)
	}
	return arm
}

// pointContent is a point estimate without its random identities.
func pointContent(e observationsqlite.TrackEstimate) string {
	return fmt.Sprintf("%d@%d %s %s %v %v %v %v %v", e.CreationSequence, e.FrameUnixNanos, e.MeasurementSource,
		e.ObservationModelID, e.X, e.Y, e.VX, e.VY, e.Covariance)
}

func TestSolidBodyExperimentOnKirk0(t *testing.T) {
	dir := t.TempDir()
	plain := runSolidBodyArm(t, dir, "default", nil)
	solid := runSolidBodyArm(t, dir, "solid_body", []string{ExperimentSolidBody})

	// Wiring: the solid body never changes what the tracker concluded.
	if !bytes.Equal(plain.baseline, solid.baseline) {
		t.Fatalf("the solid_body arm changed the tracking baseline:\n%s\n%s", plain.baseline, solid.baseline)
	}
	plainFrames, trackFrames := trackFingerprint(t, plain.cfg.OutDir)
	solidFrames, _ := trackFingerprint(t, solid.cfg.OutDir)
	requireSameDecisions(t, "solid_body", plainFrames, solidFrames)
	if plain.params == solid.params {
		t.Error("the solid_body arm shares the default's parameter hash")
	}
	if len(plain.points) != len(solid.points) {
		t.Fatalf("point estimates: %d default, %d solid_body", len(plain.points), len(solid.points))
	}
	for i := range plain.points {
		if a, b := pointContent(plain.points[i]), pointContent(solid.points[i]); a != b {
			t.Fatalf("point estimate %d differs:\n%s\n%s", i, a, b)
		}
	}
	if len(solid.bodies) != len(solid.points) {
		t.Fatalf("%d solid bodies for %d point estimates; want one beside each", len(solid.bodies), len(solid.points))
	}

	t.Logf("kirk0: %d recorded frames, %d track-frames; %d point estimates, %d solid bodies",
		solid.result.FramesRecorded, trackFrames, len(solid.points), len(solid.bodies))
	// The corpus runner reports the same summary per case.
	summary, err := SummariseSolidBodies(solid.points, solid.bodies, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("solid-body summary:\n%s", b)
	if summary.NearEdgeFixes == 0 {
		t.Fatal("no near-edge fix on kirk0: the solid body never left its medoid seed")
	}
	if summary.SolidBodies != len(solid.bodies) || summary.PointEstimates != len(solid.points) {
		t.Fatalf("summary read %d solid bodies and %d point estimates of %d and %d",
			summary.SolidBodies, summary.PointEstimates, len(solid.bodies), len(solid.points))
	}

	logExtentConvergence(t, solid.bodies)
}

// logExtentConvergence reports, per track, the last width and length belief
// and how many rows it took the width to reach the established bound.
func logExtentConvergence(t *testing.T, bodies []observationsqlite.TrackSolidBody) {
	t.Helper()
	bounds := l5tracks.DefaultConvergenceBounds()
	type trackExtent struct {
		rows, rowsToWidth int
		last              l5tracks.SolidBodyEstimate
		established       bool
	}
	tracks := map[int64]*trackExtent{}
	var order []int64
	for _, sb := range bodies {
		te := tracks[sb.CreationSequence]
		if te == nil {
			te = &trackExtent{}
			tracks[sb.CreationSequence] = te
			order = append(order, sb.CreationSequence)
		}
		te.rows++
		e := sb.Reading.Estimate
		if te.rowsToWidth == 0 && e.Width.IsConverged(bounds.MaxDimensionSigmaMetres, bounds.MinAdmissibleFrames) {
			te.rowsToWidth = te.rows
		}
		if e.Estimation == l5tracks.EstimationEstablished {
			te.established = true
		}
		te.last = e
	}
	var withWidth, converged, established int
	var widths, lengths, rowsToWidth []float64
	var b strings.Builder
	for _, seq := range order {
		te := tracks[seq]
		e := te.last
		if e.Width.Provenance == l5tracks.ProvenanceAccumulated {
			withWidth++
			widths = append(widths, float64(e.Width.Metres))
		}
		if e.Length.Provenance == l5tracks.ProvenanceAccumulated {
			lengths = append(lengths, float64(e.Length.Metres))
		}
		if te.rowsToWidth > 0 {
			converged++
			rowsToWidth = append(rowsToWidth, float64(te.rowsToWidth))
		}
		if te.established {
			established++
		}
		fmt.Fprintf(&b, "\n  seq %4d rows %4d class %-13s W %.3f±%.3f[%s,%d] L %.3f±%.3f[%s,%d] width converged at row %d, established %v",
			seq, te.rows, e.Motion.Class, e.Width.Metres, e.Width.SigmaMetres, e.Width.Provenance, e.Width.AdmissibleFrames,
			e.Length.Metres, e.Length.SigmaMetres, e.Length.Provenance, e.Length.AdmissibleFrames, te.rowsToWidth, te.established)
	}
	t.Logf("extent convergence: %d tracks, %d with accumulated width, %d with width converged (sigma <= %.2f m, >= %d frames), %d ever established; "+
		"final width median %.3f m (p10 %.3f, p90 %.3f), final length median %.3f m, rows to width convergence median %.0f%s",
		len(order), withWidth, converged, bounds.MaxDimensionSigmaMetres, bounds.MinAdmissibleFrames, established,
		evidenceMedian(widths), evidenceQuantile(widths, 0.1), evidenceQuantile(widths, 0.9), evidenceMedian(lengths), evidenceMedian(rowsToWidth), b.String())
}

func evidenceMedian(v []float64) float64 { return evidenceQuantile(v, 0.5) }

func evidenceQuantile(v []float64, q float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[int(q*float64(len(s)-1))]
}

// TestSolidBodyFaceRemediesOnKirk0 runs the face-transition remedies of the
// near-edge tracked-state plan's S2.1, and the solid body measured from full
// cluster members, beside the plain solid body. Each must leave the tracks
// alone, as the solid body does; their effect on the anchor is logged for the
// plan, not asserted, because one capture does not choose a remedy.
func TestSolidBodyFaceRemediesOnKirk0(t *testing.T) {
	dir := t.TempDir()
	arms := []struct {
		name        string
		experiments []string
	}{
		{"solid_body", []string{ExperimentSolidBody}},
		{"t1_hysteresis", []string{ExperimentSolidBody, ExperimentSolidBodyFaceHysteresis}},
		{"t2_consider", []string{ExperimentSolidBody, ExperimentSolidBodyFaceConsider}},
		{"t1_t2", []string{ExperimentSolidBody, ExperimentSolidBodyFaceHysteresis, ExperimentSolidBodyFaceConsider}},
		{"full_members", []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers}},
		{"t3_course", []string{ExperimentSolidBody, ExperimentSolidBodyCourseFaces}},
		{"t1_t3", []string{ExperimentSolidBody, ExperimentSolidBodyFaceHysteresis, ExperimentSolidBodyCourseFaces}},
		{"t1_t3_t5", []string{ExperimentSolidBody, ExperimentSolidBodyFaceHysteresis, ExperimentSolidBodyCourseFaces,
			ExperimentSolidBodyRankOneMedoid}},
		// The face-state arm is a shadow change too: the extents in the
		// state, the censored extent, the span bounds, the heading term and
		// the anisotropic reference bias leave every track alone.
		{"face_state", []string{ExperimentSolidBody, ExperimentSolidBodyFaceState}},
	}
	var plain solidBodyArm
	var table strings.Builder
	fmt.Fprintf(&table, "\n  %-14s %6s %9s %6s %8s  %-17s  %-17s  %s",
		"arm", "fixes", "face runs", "held", "faceless", "all p95/p99/max", "face p95/p99/max", "point all/face p99")
	for i, a := range arms {
		arm := runSolidBodyArm(t, dir, a.name, a.experiments)
		if i == 0 {
			plain = arm
		} else {
			if !bytes.Equal(plain.baseline, arm.baseline) {
				t.Fatalf("%s changed the tracking baseline", a.name)
			}
			if arm.params == plain.params {
				t.Errorf("%s shares the plain solid body's parameter hash", a.name)
			}
			if len(arm.points) != len(plain.points) {
				t.Fatalf("%s: %d point estimates, %d without the remedy", a.name, len(arm.points), len(plain.points))
			}
			for j := range plain.points {
				if x, y := pointContent(plain.points[j]), pointContent(arm.points[j]); x != y {
					t.Fatalf("%s: point estimate %d differs:\n%s\n%s", a.name, j, x, y)
				}
			}
		}
		s, err := SummariseSolidBodies(arm.points, arm.bodies, l5tracks.DefaultConvergenceBounds())
		if err != nil {
			t.Fatal(err)
		}
		all, face := s.AnchorBodiesCentred, s.AnchorBodiesFaceStable
		fmt.Fprintf(&table, "\n  %-14s %6d %9d %6d %8d  %.3f/%.3f/%.3f  %.3f/%.3f/%.3f  %.3f/%.3f",
			a.name, s.NearEdgeFixes, s.FaceStableRuns, s.Fallbacks["face_hysteresis"], s.Fallbacks["no_face_reached_minimum_support"],
			all.P95Metres, all.P99Metres, all.MaxMetres, face.P95Metres, face.P99Metres, face.MaxMetres,
			s.AnchorPointsCentred.P99Metres, s.AnchorPointsFaceStable.P99Metres)
		if a.name == "t1_t2" {
			b, err := json.MarshalIndent(s.FaceStableStrata, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("t1_t2 face-stable strata:\n%s", b)
		}
	}
	t.Logf("kirk0 face remedies (solid body lateral residual over body-centre frames, metres):%s", table.String())
}

// TestNearEdgeTrackOnKirk0 is S2.2's exit on kirk0: the near_edge_track arm
// (the solid body's state machine on the tracked filter, A2 association, full
// members, remedies T1 and T3) runs, and runs identically twice. Unlike the
// shadow it changes the tracks, so the comparison with the default is logged
// for the plan rather than required to be equal; the default arm itself is
// the default replay, which the other kirk0 tests pin.
func TestNearEdgeTrackOnKirk0(t *testing.T) {
	runNearEdgeTrackOnKirk0(t, "near_edge_track", []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers,
		ExperimentSolidBodyFaceHysteresis, ExperimentSolidBodyCourseFaces, ExperimentNearEdgeTrack})
}

// TestNearEdgeTrackExtentStateOnKirk0 is the same exit with the extent state
// on the tracked filter: the arm runs, runs identically twice, and every
// online solid body carries and persists the 6x6 joint covariance. How the
// tracked arm moves against near_edge_track is logged, not asserted.
func TestNearEdgeTrackExtentStateOnKirk0(t *testing.T) {
	first := runNearEdgeTrackOnKirk0(t, "near_edge_track_extent_state", []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers,
		ExperimentSolidBodyFaceHysteresis, ExperimentSolidBodyCourseFaces, ExperimentNearEdgeTrack, ExperimentSolidBodyExtentState})
	var joint, jointFront, independentFront int
	var frontJoint, frontIndependent []float64
	for _, sb := range first.bodies {
		r := sb.Reading
		if !r.ExtentState || !r.Estimate.ExtentState {
			t.Fatalf("solid body %s read back without the extent state", sb.EstimateID)
		}
		if _, ok := r.JointCovariance(); !ok {
			t.Fatalf("solid body %s has no joint covariance", sb.EstimateID)
		}
		joint++
		if p, ok := r.Estimate.ProjectSurface(l5tracks.SurfaceFront); ok {
			if !p.JointBound {
				t.Fatalf("solid body %s: a front projection that is not joint under the extent state", sb.EstimateID)
			}
			jointFront++
			frontJoint = append(frontJoint, float64(p.SigmaMetres))
			apart := r.Estimate
			apart.ExtentState = false
			if q, ok := apart.ProjectSurface(l5tracks.SurfaceFront); ok {
				independentFront++
				frontIndependent = append(frontIndependent, float64(q.SigmaMetres))
			}
		}
	}
	t.Logf("kirk0 near_edge_track_extent_state: %d solid bodies with the joint covariance; %d front projections, "+
		"joint sigma median %.3f m (p90 %.3f), independent-sum sigma median %.3f m (p90 %.3f) over %d of them",
		joint, jointFront, evidenceMedian(frontJoint), evidenceQuantile(frontJoint, 0.9),
		evidenceMedian(frontIndependent), evidenceQuantile(frontIndependent, 0.9), independentFront)
	logExtentConvergence(t, first.bodies)
}

// runNearEdgeTrackOnKirk0 runs a tracked arm twice beside the default, holds
// it to determinism and to one solid body beside each point estimate at the
// same position, logs its summary, and returns the first run.
func runNearEdgeTrackOnKirk0(t *testing.T, name string, experiments []string) solidBodyArm {
	t.Helper()
	dir := t.TempDir()
	plain := runSolidBodyArm(t, dir, "default", nil)
	first := runSolidBodyArm(t, dir, name, experiments)
	repeat := runSolidBodyArm(t, dir, name+"_repeat", experiments)

	if !bytes.Equal(first.baseline, repeat.baseline) {
		t.Fatalf("%s's tracking baseline differs between two replays:\n%s\n%s", name, first.baseline, repeat.baseline)
	}
	firstFrames, trackFrames := trackFingerprint(t, first.cfg.OutDir)
	repeatFrames, _ := trackFingerprint(t, repeat.cfg.OutDir)
	requireSameDecisions(t, name+" repeat", firstFrames, repeatFrames)
	if len(first.points) != len(repeat.points) || len(first.bodies) != len(repeat.bodies) {
		t.Fatalf("repeat wrote %d/%d rows, first %d/%d", len(repeat.points), len(repeat.bodies), len(first.points), len(first.bodies))
	}
	for i := range first.points {
		if a, b := pointContent(first.points[i]), pointContent(repeat.points[i]); a != b {
			t.Fatalf("point estimate %d differs between replays:\n%s\n%s", i, a, b)
		}
	}
	for i := range first.bodies {
		if a, b := solidBodyContent(first.bodies[i]), solidBodyContent(repeat.bodies[i]); a != b {
			t.Fatalf("solid body %d differs between replays:\n%s\n%s", i, a, b)
		}
	}
	if first.params == plain.params {
		t.Errorf("%s shares the default's parameter hash", name)
	}
	if len(first.bodies) != len(first.points) {
		t.Fatalf("%d solid bodies for %d point estimates; want one beside each", len(first.bodies), len(first.points))
	}

	// Under near_edge_track the solid body is the tracked state, so every
	// row pair describes one position.
	type key struct{ seq, frame int64 }
	bodyAt := map[key]observationsqlite.TrackSolidBody{}
	for _, sb := range first.bodies {
		bodyAt[key{sb.CreationSequence, sb.FrameUnixNanos}] = sb
	}
	nearEdge := 0
	for _, p := range first.points {
		sb, ok := bodyAt[key{p.CreationSequence, p.FrameUnixNanos}]
		if !ok {
			t.Fatalf("no solid body beside point estimate %d@%d", p.CreationSequence, p.FrameUnixNanos)
		}
		if sb.Reading.Estimate.X != p.X || sb.Reading.Estimate.Y != p.Y || sb.Reading.VX != p.VX || sb.Reading.VY != p.VY {
			t.Fatalf("track %d at %d: solid body (%v, %v) is not the tracked estimate (%v, %v)",
				p.CreationSequence, p.FrameUnixNanos, sb.Reading.Estimate.X, sb.Reading.Estimate.Y, p.X, p.Y)
		}
		if p.MeasurementSource == string(l5tracks.MeasurementNearEdgeCandidateV1) {
			nearEdge++
		}
	}
	if nearEdge == 0 {
		t.Fatal("no tracked estimate was updated by a near-edge fix")
	}

	summary, err := SummariseSolidBodies(first.points, first.bodies, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	if summary.ReReferences == 0 {
		t.Error("no re-reference counted on kirk0")
	}
	plainFrames, plainTrackFrames := trackFingerprint(t, plain.cfg.OutDir)
	all := summary.AnchorBodiesCentred
	t.Logf("kirk0 %s: %d recorded frames (default %d), %d track-frames (default %d); "+
		"%d fixes of %d rows, %d near-edge point estimates, %d re-references, %d lapses; "+
		"body-centre lateral p95/p99/max %.3f/%.3f/%.3f m, steady p99 %.3f m, face-stable p99 %.3f m",
		name, len(firstFrames), len(plainFrames), trackFrames, plainTrackFrames,
		summary.NearEdgeFixes, summary.SolidBodies, nearEdge, summary.ReReferences, summary.Lapses,
		all.P95Metres, all.P99Metres, all.MaxMetres, summary.AnchorBodiesSteady.P99Metres, summary.AnchorBodiesFaceStable.P99Metres)
	return first
}

// solidBodyContent is a solid body without its random identities: the
// reading, joint covariance included.
func solidBodyContent(sb observationsqlite.TrackSolidBody) string {
	joint, _ := sb.Reading.JointCovariance()
	return fmt.Sprintf("%d@%d %s %+v %v", sb.CreationSequence, sb.FrameUnixNanos, sb.Reading.CovarianceStateModel(), sb.Reading, joint)
}

// boxArmSummary is what one kirk0 arm drew as its published box, over the
// scored frames: per track, the longest box and the last of its counters.
type boxArmSummary struct {
	trackFrames int
	maxLength   map[int64]float64
	maxWidth    map[int64]float64
	stats       map[int64]l5tracks.BoxStats
}

func newBoxArmSummary() *boxArmSummary {
	return &boxArmSummary{maxLength: map[int64]float64{}, maxWidth: map[int64]float64{}, stats: map[int64]l5tracks.BoxStats{}}
}

func (s *boxArmSummary) tap(f TrackerTapFrame) {
	if !f.Scored {
		return
	}
	for _, tr := range f.Tracks {
		if tr.BoxLength <= 0 {
			continue
		}
		s.trackFrames++
		seq := tr.CreationSequence
		if l := float64(tr.BoxLength); l > s.maxLength[seq] {
			s.maxLength[seq] = l
			s.maxWidth[seq] = float64(tr.BoxWidth)
		}
		if st, ok := tr.BoxModelStats(); ok {
			s.stats[seq] = st
		}
	}
}

// totals sums the per-track counters.
func (s *boxArmSummary) totals() l5tracks.BoxStats {
	var sum l5tracks.BoxStats
	for _, st := range s.stats {
		sum.BodyExtentFrames += st.BodyExtentFrames
		sum.BodyExtentUnknown += st.BodyExtentUnknown
		sum.BodyExtentAxisRefusals += st.BodyExtentAxisRefusals
		sum.BodyExtentUnsettled += st.BodyExtentUnsettled
		sum.BodyExtentWidthFrames += st.BodyExtentWidthFrames
		sum.BodyExtentOneSided += st.BodyExtentOneSided
	}
	return sum
}

// TestBoxExtentMeasuredLengthOnKirk0 is box_extent_measured_length's wiring
// check against box_extent_solid_body on the face-state solid body: #984's
// arm takes the extent state's width on some frames (the width that widened
// car 8's box in l5id0019), and the measured-only arm takes it on fewer,
// keeps the box's own width on frames the state settled from one side, and
// still takes the length. Label-free: geom0017 scores the boxes against
// geom0015's reviewed poses.
func TestBoxExtentMeasuredLengthOnKirk0(t *testing.T) {
	dir := t.TempDir()
	arm := []string{ExperimentSolidBody, ExperimentSolidBodyFaceState, ExperimentBoxExtentSolidBody}
	allBox, measuredBox := newBoxArmSummary(), newBoxArmSummary()
	all := runSolidBodyArmTapped(t, dir, "box_extent_solid_body", arm, allBox.tap)
	measured := runSolidBodyArmTapped(t, dir, "box_extent_measured_length",
		append(append([]string{}, arm...), ExperimentBoxExtentMeasuredLength), measuredBox.tap)
	if measured.params == all.params {
		t.Error("box_extent_measured_length shares box_extent_solid_body's parameter hash")
	}
	a, m := allBox.totals(), measuredBox.totals()
	vehicleWidths := func(s *boxArmSummary) []float64 {
		var w []float64
		for seq, l := range s.maxLength {
			if l >= 3.5 && s.maxWidth[seq] >= 1.5 {
				w = append(w, s.maxWidth[seq])
			}
		}
		return w
	}
	aw, mw := vehicleWidths(allBox), vehicleWidths(measuredBox)
	t.Logf("kirk0 box_extent_solid_body: %d took the state (%d the width), %d none to read, %d axis refusals, %d unsettled; "+
		"vehicle tracks %d, width at the longest box median %.2f m, p90 %.2f m",
		a.BodyExtentFrames, a.BodyExtentWidthFrames, a.BodyExtentUnknown, a.BodyExtentAxisRefusals, a.BodyExtentUnsettled,
		len(aw), evidenceMedian(aw), evidenceQuantile(aw, 0.9))
	t.Logf("kirk0 box_extent_measured_length: %d took the state (%d the width), %d kept a one-sided dimension, %d none to read, "+
		"%d axis refusals, %d unsettled; vehicle tracks %d, width at the longest box median %.2f m, p90 %.2f m; tracking baseline equal %v",
		m.BodyExtentFrames, m.BodyExtentWidthFrames, m.BodyExtentOneSided, m.BodyExtentUnknown, m.BodyExtentAxisRefusals, m.BodyExtentUnsettled,
		len(mw), evidenceMedian(mw), evidenceQuantile(mw, 0.9), bytes.Equal(all.baseline, measured.baseline))
	if a.BodyExtentOneSided != 0 {
		t.Errorf("box_extent_solid_body counted %d one-sided frames without the qualifier", a.BodyExtentOneSided)
	}
	if a.BodyExtentWidthFrames == 0 {
		t.Fatal("box_extent_solid_body took no width from the state on kirk0: the comparison has nothing to show")
	}
	if m.BodyExtentWidthFrames >= a.BodyExtentWidthFrames || m.BodyExtentOneSided == 0 {
		t.Errorf("measured only took the width on %d frames against %d, with %d one-sided: want fewer, and one-sided frames kept",
			m.BodyExtentWidthFrames, a.BodyExtentWidthFrames, m.BodyExtentOneSided)
	}
	if m.BodyExtentFrames == 0 {
		t.Error("measured only took no length from the state on kirk0")
	}
}

// TestBoxExtentFromSolidBodyOnKirk0 is box_extent_solid_body's wiring check:
// beside the face-state solid body, the published box takes its length and
// width from the solid body's extent state on some frames and falls back on
// the rest, and the run says how often each. Label-free: it shows the option
// reaches the box on a real capture and how much it moves it, not whether
// the moved box is nearer the truth, which needs geom0015's reviewed body
// centres.
func TestBoxExtentFromSolidBodyOnKirk0(t *testing.T) {
	dir := t.TempDir()
	faceState := []string{ExperimentSolidBody, ExperimentSolidBodyFaceState}
	plainBox, onBox := newBoxArmSummary(), newBoxArmSummary()
	plain := runSolidBodyArmTapped(t, dir, "face_state", faceState, plainBox.tap)
	on := runSolidBodyArmTapped(t, dir, "box_extent_solid_body", append(append([]string{}, faceState...), ExperimentBoxExtentSolidBody), onBox.tap)
	if on.params == plain.params {
		t.Error("box_extent_solid_body shares the face-state arm's parameter hash")
	}
	sameTracks := bytes.Equal(plain.baseline, on.baseline)
	if !sameTracks {
		// L3's track guard holds the returns inside each confirmed track's
		// published box as foreground (pipeline/track_guard.go), so a longer
		// box changes the next frames' clusters and the tracks move with it.
		t.Logf("box_extent_solid_body changed the tracking baseline, through the track guard's published boxes")
	}
	total := onBox.totals()
	t.Logf("kirk0 box_extent_solid_body: %d box track-frames (face state %d), tracking baseline equal %v; "+
		"associated frames: %d took the solid body's extent, %d had none to read, %d refused on axis, %d not yet settled",
		onBox.trackFrames, plainBox.trackFrames, sameTracks,
		total.BodyExtentFrames, total.BodyExtentUnknown, total.BodyExtentAxisRefusals, total.BodyExtentUnsettled)
	if total.BodyExtentFrames == 0 {
		t.Fatal("no frame on kirk0 took the solid body's extent: the option never reached the box")
	}
	if pt := plainBox.totals(); pt != (l5tracks.BoxStats{}) {
		t.Errorf("the face-state arm counted body-extent frames without the option: %+v", pt)
	}
	logBoxLengthShift(t, plainBox, onBox)
}

// logBoxLengthShift reports, over the tracks both arms drew, how the longest
// box each drew moved, and the longest box among vehicle-wide tracks. Tracks
// are paired by creation sequence, which names the same object in both arms
// only while their tracks agree; where the track guard has moved them it is
// an approximate pairing, and the vehicle-wide medians are the firmer read.
func logBoxLengthShift(t *testing.T, plain, on *boxArmSummary) {
	t.Helper()
	var deltas, plainVehicle, onVehicle []float64
	longer, shorter, used := 0, 0, 0
	for seq, l := range on.maxLength {
		p, ok := plain.maxLength[seq]
		if !ok {
			continue
		}
		d := l - p
		deltas = append(deltas, d)
		switch {
		case d > 0.5:
			longer++
		case d < -0.5:
			shorter++
		}
		if on.stats[seq].BodyExtentFrames > 0 {
			used++
		}
		if on.maxWidth[seq] >= 1.5 && plain.maxWidth[seq] >= 1.5 {
			plainVehicle = append(plainVehicle, p)
			onVehicle = append(onVehicle, l)
		}
	}
	t.Logf("longest box per track over %d tracks in both arms (%d took the state on some frame): "+
		"change median %+.3f m, p10 %+.3f, p90 %+.3f; %d longer by over 0.5 m, %d shorter; "+
		"vehicle-wide tracks (%d): median longest %.2f m without, %.2f m with, p90 %.2f and %.2f m",
		len(deltas), used, evidenceMedian(deltas), evidenceQuantile(deltas, 0.1), evidenceQuantile(deltas, 0.9), longer, shorter,
		len(onVehicle), evidenceMedian(plainVehicle), evidenceMedian(onVehicle), evidenceQuantile(plainVehicle, 0.9), evidenceQuantile(onVehicle, 0.9))
}

// headingArmSummary is, over one kirk0 arm's scored frames, how the solid
// body's orientation sat against the published box's heading, and the
// shared belief's calibration diagnostic.
type headingArmSummary struct {
	bodyFrames, disagree int       // body-centre frames, and those over 10° from the box
	nis                  []float64 // accepted axis innovations' r²/S
	gated                int
	// atScale holds, per scale in sharedNISScales, the accepted innovations
	// read against that AxisSigmaScale (HeadingNISAtScale); priorShare is
	// each accepted innovation's share of S that is the predicted P.
	atScale    [][]float64
	priorShare []float64
}

// sharedNISScales are the AxisSigmaScale values the kirk0 calibration reads
// the shared heading's innovations against, the tuning default first. The
// tuning key stays at 1.5: these are readings of that filter's innovations.
var sharedNISScales = []float64{1.5, 1.0, 0.75, 0.5}

func (s *headingArmSummary) tap(f TrackerTapFrame) {
	if !f.Scored {
		return
	}
	for _, tr := range f.Tracks {
		r, ok := tr.SolidBody()
		if !ok {
			continue
		}
		m := r.Measurement
		if m.HeadingNISKnown {
			if m.HeadingNISGated {
				s.gated++
			} else {
				s.nis = append(s.nis, float64(m.HeadingNIS))
				if s.atScale == nil {
					s.atScale = make([][]float64, len(sharedNISScales))
				}
				for i, k := range sharedNISScales {
					if v, ok := m.HeadingNISAtScale(k); ok {
						s.atScale[i] = append(s.atScale[i], v)
					}
				}
				if total := float64(m.HeadingPriorVariance) + sharedNISScales[0]*sharedNISScales[0]*float64(m.HeadingFitVariance); total > 0 {
					s.priorShare = append(s.priorShare, float64(m.HeadingPriorVariance)/total)
				}
			}
		}
		e := r.Estimate
		if e.Reference != l5tracks.ReferenceBodyCentre || tr.BoxLength <= 0 || e.Orientation.Provenance == l5tracks.ProvenanceNone {
			continue
		}
		s.bodyFrames++
		if l5tracks.FoldAxisAngleDeg(float64(e.Orientation.PsiRad)-float64(tr.BoxHeadingRad)) > 10 {
			s.disagree++
		}
	}
}

// TestSolidBodySharedOrientationOnKirk0 is solid_body_shared_orientation's
// wiring check beside the face-state solid body. The shared belief is a
// shadow change, so its tracks are the face-state arm's exactly; the solid
// body's axis then agrees with the box's on the body-centre frames it is
// shared on, and the axis innovation's mean NIS says how the box filter's
// variance stands against its errors on a real capture, and against smaller
// axis_sigma_scale values on the same innovations. With
// box_extent_solid_body as well, it reports the refusals on axis against the
// box extent alone, with the agreement test on the body's own axis (the
// default) and on the shared heading (box_extent_shared_axis). Label-free,
// like the other kirk0 arms (shared-orientation-duplicates-2026-11).
func TestSolidBodySharedOrientationOnKirk0(t *testing.T) {
	dir := t.TempDir()
	faceState := []string{ExperimentSolidBody, ExperimentSolidBodyFaceState}
	with := func(names ...string) []string { return append(append([]string{}, faceState...), names...) }
	plainHeading, sharedHeading := &headingArmSummary{}, &headingArmSummary{}
	plain := runSolidBodyArmTapped(t, dir, "face_state", faceState, plainHeading.tap)
	shared := runSolidBodyArmTapped(t, dir, "shared_orientation", with(ExperimentSolidBodySharedOrientation), sharedHeading.tap)
	aloneBox, bothBox, sharedAxisBox := newBoxArmSummary(), newBoxArmSummary(), newBoxArmSummary()
	alone := runSolidBodyArmTapped(t, dir, "box_extent", with(ExperimentBoxExtentSolidBody), aloneBox.tap)
	both := runSolidBodyArmTapped(t, dir, "shared_orientation_box_extent",
		with(ExperimentSolidBodySharedOrientation, ExperimentBoxExtentSolidBody), bothBox.tap)
	sharedAxis := runSolidBodyArmTapped(t, dir, "shared_orientation_box_extent_shared_axis",
		with(ExperimentSolidBodySharedOrientation, ExperimentBoxExtentSolidBody, ExperimentBoxExtentSharedAxis), sharedAxisBox.tap)

	if !bytes.Equal(plain.baseline, shared.baseline) {
		t.Fatal("solid_body_shared_orientation changed the tracking baseline: the shadow body's orientation reached the tracks")
	}
	if shared.params == plain.params {
		t.Error("solid_body_shared_orientation shares the face-state arm's parameter hash")
	}
	nis := sharedHeading.nis
	if len(nis) == 0 {
		t.Fatal("no heading innovation reached the solid body on kirk0: the shared orientation never applied")
	}
	t.Logf("kirk0 solid body against the box's heading over body-centre frames: face state %d of %d over 10°, shared %d of %d",
		plainHeading.disagree, plainHeading.bodyFrames, sharedHeading.disagree, sharedHeading.bodyFrames)
	t.Logf("kirk0 shared heading calibration: %d axis innovations accepted, %d gated (%.1f %%); mean NIS %.3f, median %.3f, p90 %.3f "+
		"(a calibrated filter's accepted innovations average about one)",
		len(nis), sharedHeading.gated, 100*float64(sharedHeading.gated)/float64(sharedHeading.gated+len(nis)),
		meanFloat(nis), evidenceMedian(nis), evidenceQuantile(nis, 0.9))
	if sharedHeading.disagree >= plainHeading.disagree {
		t.Errorf("sharing the heading left %d body-centre frames over 10° from the box, against %d without",
			sharedHeading.disagree, plainHeading.disagree)
	}
	for i, k := range sharedNISScales {
		v := sharedHeading.atScale[i]
		t.Logf("kirk0 shared heading NIS read at axis_sigma_scale %.2f: mean %.3f, median %.3f, p90 %.3f, share over the 3-sigma gate %.1f %% "+
			"(a calibrated chi-square(1) has median 0.455 and p90 2.71)",
			k, meanFloat(v), evidenceMedian(v), evidenceQuantile(v, 0.9), 100*shareOver(v, 9))
	}
	t.Logf("kirk0 shared heading: predicted P_psi-psi's share of S at 1.5, median %.3f, p10 %.3f, p90 %.3f",
		evidenceMedian(sharedHeading.priorShare), evidenceQuantile(sharedHeading.priorShare, 0.1), evidenceQuantile(sharedHeading.priorShare, 0.9))
	if got, want := evidenceMedian(sharedHeading.atScale[0]), evidenceMedian(nis); math.Abs(got-want) > 1e-3*math.Max(1, want) {
		t.Errorf("the reading at the filter's own scale has median %.4f, HeadingNIS %.4f", got, want)
	}

	// Beside box_extent_solid_body the box draws the solid body's extent, so
	// the shared orientation reaches the box. Read against the body's own
	// axis the agreement test refuses about as box_extent_solid_body alone
	// does; read against the shared heading it can refuse only on unshared
	// frames.
	logArm := func(name string, arm solidBodyArm, s *boxArmSummary) l5tracks.BoxStats {
		total := s.totals()
		t.Logf("kirk0 %s: tracking baseline equal to box_extent_solid_body alone %v; associated frames: "+
			"%d took the solid body's extent, %d had none to read, %d refused on axis, %d not yet settled",
			name, bytes.Equal(alone.baseline, arm.baseline), total.BodyExtentFrames, total.BodyExtentUnknown, total.BodyExtentAxisRefusals, total.BodyExtentUnsettled)
		return total
	}
	aloneTotal := logArm("box_extent_solid_body alone", alone, aloneBox)
	ownTotal := logArm("shared orientation with box_extent_solid_body (own axis)", both, bothBox)
	sharedTotal := logArm("shared orientation with box_extent_solid_body and box_extent_shared_axis", sharedAxis, sharedAxisBox)
	logBoxLengthShift(t, aloneBox, bothBox)
	logBoxLengthShift(t, aloneBox, sharedAxisBox)
	if sharedTotal.BodyExtentAxisRefusals >= ownTotal.BodyExtentAxisRefusals {
		t.Errorf("against the shared heading the test refused %d frames, against the own axis %d; want fewer",
			sharedTotal.BodyExtentAxisRefusals, ownTotal.BodyExtentAxisRefusals)
	}
	if aloneTotal.BodyExtentAxisRefusals == 0 || ownTotal.BodyExtentAxisRefusals == 0 {
		t.Errorf("no axis refusal with the own axis (%d) or with the box extent alone (%d)",
			ownTotal.BodyExtentAxisRefusals, aloneTotal.BodyExtentAxisRefusals)
	}
}

// shareOver is the share of v above x.
func shareOver(v []float64, x float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	n := 0
	for _, e := range v {
		if e > x {
			n++
		}
	}
	return float64(n) / float64(len(v))
}

func meanFloat(v []float64) float64 {
	if len(v) == 0 {
		return math.NaN()
	}
	s := 0.0
	for _, x := range v {
		s += x
	}
	return s / float64(len(v))
}
