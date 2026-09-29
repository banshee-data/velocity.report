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
	cfg := Config{
		PCAPFile: requireKirk0(t), OutDir: filepath.Join(dir, name), SensorID: "test-replay", UDPPort: 2369,
		// The whole capture after the settled 20 s prefix.
		StartSeconds: 20, WarmupSeconds: 20, RequireSettled: true,
		ObservationDBPath: filepath.Join(dir, name+".db"), ReplayCaseID: "kirk0-solid-body",
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
		{"t4_half", []string{ExperimentSolidBody, ExperimentSolidBodyHalfExtents}},
		{"t1_t3_t4", []string{ExperimentSolidBody, ExperimentSolidBodyFaceHysteresis, ExperimentSolidBodyCourseFaces,
			ExperimentSolidBodyHalfExtents}},
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
		if a.name == "t1_t3" || a.name == "t1_t3_t4" {
			b, err := json.MarshalIndent(s.SteadyTransitions, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("%s steady-run transitions:\n%s", a.name, b)
		}
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
	dir := t.TempDir()
	experiments := []string{ExperimentSolidBody, ExperimentSolidBodyFullMembers,
		ExperimentSolidBodyFaceHysteresis, ExperimentSolidBodyCourseFaces, ExperimentNearEdgeTrack}
	plain := runSolidBodyArm(t, dir, "default", nil)
	first := runSolidBodyArm(t, dir, "near_edge_track", experiments)
	repeat := runSolidBodyArm(t, dir, "near_edge_track_repeat", experiments)

	if !bytes.Equal(first.baseline, repeat.baseline) {
		t.Fatalf("near_edge_track's tracking baseline differs between two replays:\n%s\n%s", first.baseline, repeat.baseline)
	}
	firstFrames, trackFrames := trackFingerprint(t, first.cfg.OutDir)
	repeatFrames, _ := trackFingerprint(t, repeat.cfg.OutDir)
	requireSameDecisions(t, "near_edge_track repeat", firstFrames, repeatFrames)
	if len(first.points) != len(repeat.points) || len(first.bodies) != len(repeat.bodies) {
		t.Fatalf("repeat wrote %d/%d rows, first %d/%d", len(repeat.points), len(repeat.bodies), len(first.points), len(first.bodies))
	}
	for i := range first.points {
		if a, b := pointContent(first.points[i]), pointContent(repeat.points[i]); a != b {
			t.Fatalf("point estimate %d differs between replays:\n%s\n%s", i, a, b)
		}
	}
	if first.params == plain.params {
		t.Error("near_edge_track shares the default's parameter hash")
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
	t.Logf("kirk0 near_edge_track: %d recorded frames (default %d), %d track-frames (default %d); "+
		"%d fixes of %d rows, %d near-edge point estimates, %d re-references, %d lapses; "+
		"body-centre lateral p95/p99/max %.3f/%.3f/%.3f m, steady p99 %.3f m, face-stable p99 %.3f m",
		len(firstFrames), len(plainFrames), trackFrames, plainTrackFrames,
		summary.NearEdgeFixes, summary.SolidBodies, nearEdge, summary.ReReferences, summary.Lapses,
		all.P95Metres, all.P99Metres, all.MaxMetres, summary.AnchorBodiesSteady.P99Metres, summary.AnchorBodiesFaceStable.P99Metres)
}
