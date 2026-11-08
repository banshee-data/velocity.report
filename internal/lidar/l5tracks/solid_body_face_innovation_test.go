package l5tracks

import (
	"math"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// A two-face fix's record is the joint innovation, worked by hand: each
// face's z less the prior along its normal, S = n P nᵀ + r on the diagonal
// and n₀ P n₁ off it, and the joint NIS equals the sequential update's.
func TestFaceInnovationsAreTheJointInnovationByHand(t *testing.T) {
	tracker := NewTracker(solidBodyConfig())
	r := float64(tracker.Config.MeasurementNoise)
	state := [4]float32{0.1, -0.2, 5, 0.3}
	p := [16]float32{0.2, 0.05, 0.1, 0.02, 0.05, 0.12, 0.01, 0.06, 0.1, 0.01, 1, 0.1, 0.02, 0.06, 0.1, 0.8}
	// A front face along +x and a left face along +y.
	front := EdgeMeasurement{Face: FaceFront, NormalX: 1, PlaneOffsetMetres: 2.6, HalfExtentMetres: 2.25,
		HalfExtentProvenance: ProvenanceAccumulated}
	left := EdgeMeasurement{Face: FaceLeft, NormalY: 1, PlaneOffsetMetres: 0.75, HalfExtentMetres: 0.9,
		HalfExtentProvenance: ProvenanceAccumulated}
	edges := []EdgeMeasurement{front, left}

	var m SolidBodyMeasurement
	flat := flatJoint(state, p)
	tracker.recordFaceInnovations(&m, &flat, edges, false, nil, nil)
	if m.FaceInnovationCount != 2 {
		t.Fatalf("count %d, want 2", m.FaceInnovationCount)
	}
	// By hand: y = (2.6 − 2.25) − 0.1 = 0.25 and (0.75 − 0.9) − (−0.2) = 0.05;
	// S = [[0.2 + r, 0.05], [0.05, 0.12 + r]].
	want := []struct{ y, s float64 }{{0.25, 0.2 + r}, {0.05, 0.12 + r}}
	for k, w := range want {
		f := m.FaceInnovations[k]
		if math.Abs(float64(f.Innovation)-w.y) > 1e-6 || math.Abs(float64(f.Variance)-w.s) > 1e-6 ||
			math.Abs(float64(f.NoiseVariance)-r) > 1e-9 || f.ExtentVariance != 0 {
			t.Errorf("face %d: %+v, want y %.4f s %.4f r %.4f", k, f, w.y, w.s, r)
		}
	}
	if math.Abs(float64(m.FaceInnovationCross)-0.05) > 1e-7 {
		t.Errorf("S01 %.6f, want 0.05", m.FaceInnovationCross)
	}
	if m.PriorX != state[0] || m.PriorY != state[1] || m.PriorVX != state[2] || m.PriorVY != state[3] {
		t.Errorf("prior %v %v %v %v, want %v", m.PriorX, m.PriorY, m.PriorVX, m.PriorVY, state)
	}
	s00, s11, s01 := 0.2+r, 0.12+r, 0.05
	det := s00*s11 - s01*s01
	handNIS := (s11*0.25*0.25 - 2*s01*0.25*0.05 + s00*0.05*0.05) / det
	joint, ok := m.JointFaceNIS()
	if !ok || math.Abs(joint-handNIS) > 1e-5 {
		t.Errorf("joint NIS %.6f (%v), want %.6f by hand", joint, ok, handNIS)
	}
	_, _, sequential, _, ok := tracker.applyEdgeMeasurements(state, p, edges, nil, nil)
	if !ok || math.Abs(float64(sequential)-handNIS) > 1e-5 {
		t.Errorf("sequential NIS %.6f, want the joint %.6f", sequential, handNIS)
	}

	// One face alone: rank one, no cross term, y²/S.
	tracker.recordFaceInnovations(&m, &flat, edges[:1], false, nil, nil)
	if joint, ok := m.JointFaceNIS(); m.FaceInnovationCount != 1 || m.FaceInnovationCross != 0 || !ok ||
		math.Abs(joint-0.25*0.25/(0.2+r)) > 1e-6 {
		t.Errorf("rank one: count %d cross %v NIS %.6f", m.FaceInnovationCount, m.FaceInnovationCross, joint)
	}
}

// Under ExtentState a face measures n·c + h: the innovation is the plane less
// that prediction, and S carries the half-extent's variance and its
// covariance with the position.
func TestFaceInnovationsUnderExtentStateCarryTheHalfExtent(t *testing.T) {
	cfg := solidBodyConfig()
	cfg.SolidBody.ExtentState = true
	tracker := NewTracker(cfg)
	r := float64(tracker.Config.MeasurementNoise)
	var sb solidBodyTrack
	sb.ext = solidBodyExtent{on: true, h: [2]float32{2.0, 0.9}}
	sb.ext.phh = [4]float32{0.3, 0, 0, 0.04}
	sb.ext.pxh = [8]float32{0.02, 0, 0, 0, 0, 0, 0, 0} // cov(x, hL)
	state := [4]float32{0.1, -0.2, 5, 0.3}
	p := [16]float32{0.2, 0.05, 0.1, 0.02, 0.05, 0.12, 0.01, 0.06, 0.1, 0.01, 1, 0.1, 0.02, 0.06, 0.1, 0.8}
	front := EdgeMeasurement{Face: FaceFront, NormalX: 1, PlaneOffsetMetres: 2.6, HalfExtentMetres: 2.0}
	js := sb.jointOf(state, p)
	var m SolidBodyMeasurement
	tracker.recordFaceInnovations(&m, &js, []EdgeMeasurement{front}, true, nil, nil)
	f := m.FaceInnovations[0]
	// y = 2.6 − (0.1 + 2.0); S = P_xx + 2 cov(x, hL) + P_hLhL + r.
	if math.Abs(float64(f.Innovation)-0.5) > 1e-6 {
		t.Errorf("innovation %.6f, want 0.5", f.Innovation)
	}
	if want := 0.2 + 2*0.02 + 0.3 + r; math.Abs(float64(f.Variance)-want) > 1e-6 {
		t.Errorf("variance %.6f, want %.6f", f.Variance, want)
	}
	if math.Abs(float64(f.ExtentVariance)-0.3) > 1e-7 {
		t.Errorf("extent variance %.6f, want 0.3", f.ExtentVariance)
	}
	nis, _, ok := tracker.applyEdgeMeasurementsJoint(&js, []EdgeMeasurement{front}, nil)
	if joint, jok := m.JointFaceNIS(); !ok || !jok || math.Abs(joint-float64(nis)) > 1e-5 {
		t.Errorf("joint NIS %.6f, update's %.6f", joint, nis)
	}
}

// On synthetic passes, shipped and face-state, every near-edge fix records
// one innovation per applied face whose joint NIS is the NIS the update
// reported, and nothing else records any.
func TestEveryFixRecordsItsFaceInnovations(t *testing.T) {
	faceState := func() TrackerConfig {
		cfg := solidBodyConfig()
		cfg.SolidBody.CourseAlignedFaces = true
		cfg.SolidBody.ExtentState = true
		cfg.SolidBody.CensoredExtent = true
		cfg.SolidBody.SpanBoundsBelief = true
		cfg.SolidBody.HeadingFaceNoise = true
		cfg.SolidBody.AnisotropicReferenceBias = true
		return cfg
	}
	for _, c := range []struct {
		name string
		pass l4perception.SyntheticPass
		cfg  TrackerConfig
	}{
		{"shipped_default_pass", l4perception.DefaultSyntheticPass(), solidBodyConfig()},
		{"face_state_end_on_truck", endOnTruckPass(), faceState()},
		{"face_state_default_pass", l4perception.DefaultSyntheticPass(), faceState()},
	} {
		tracker := NewTracker(c.cfg)
		fixes, byRank := 0, map[int]int{}
		for _, f := range syntheticPassFrames(t, c.pass) {
			tracker.Update(f.clusters, f.at)
			for _, tr := range tracker.GetActiveTracks() {
				reading, ok := tr.SolidBody()
				if !ok {
					continue
				}
				m := reading.Measurement
				if m.Source != MeasurementNearEdgeCandidateV1 {
					if m.FaceInnovationCount != 0 {
						t.Fatalf("%s: a %q update recorded %d face innovations", c.name, m.Source, m.FaceInnovationCount)
					}
					continue
				}
				if m.FaceInnovationCount != m.Rank {
					t.Fatalf("%s: rank %d fix recorded %d innovations", c.name, m.Rank, m.FaceInnovationCount)
				}
				joint, ok := m.JointFaceNIS()
				if !ok || math.Abs(joint-float64(m.NIS)) > 1e-3*math.Max(1, float64(m.NIS)) {
					t.Fatalf("%s: joint NIS %.6f, update's %.6f", c.name, joint, m.NIS)
				}
				fixes++
				byRank[m.Rank]++
			}
		}
		if fixes == 0 {
			t.Fatalf("%s: no near-edge fix to check", c.name)
		}
		t.Logf("%s: %d fixes checked, by rank %v", c.name, fixes, byRank)
	}
}
