package l5tracks

import (
	"math/rand"
	"testing"
)

// Seeing a side plane does not establish that the cluster contains both
// longitudinal endpoints: occlusion and fragment membership can cut the face
// short. Its span must remain a lower bound, including on repeated frames.
func TestASideFaceDoesNotTurnAPartialSpanIntoALengthMeasurement(t *testing.T) {
	cfg := solidBodyConfig()
	cfg.SolidBody.ExtentState = true
	cfg.SolidBody.SpanBoundsBelief = true
	tracker := NewTracker(cfg)
	track := boxPassBody(-10, 5, 5)
	sb := &track.solidBody
	sb.ext = solidBodyExtent{on: true, h: [2]float32{2.25, 0.95},
		phh: [4]float32{0.5625, 0, 0, 0.04}}
	prior := dimensionPriorFor(MotionRigidVehicle)
	// A cluster containing only 1.2 m of a longer body's near side. The
	// side-plane observation measures width and position, not length.
	cluster := passingBoxCluster(rand.New(rand.NewSource(37)), -10, 5, 1.2, 1.9)
	face := EdgeMeasurement{Face: FaceRight, NormalY: -1, PlaneOffsetMetres: -4.05,
		HalfExtentMetres: 0.95, HalfExtentProvenance: ProvenanceAccumulated, SupportPoints: 20}
	length, variance := sb.ext.h[0], sb.ext.phh[0]
	for frame := 0; frame < 50; frame++ {
		f := tracker.measureNearEdgeFrame(sb, cluster, prior, 100+frame, false)
		if f.fallback != "" || !(f.spanBound[0] > 0) || f.spanBound[0] >= 2*length {
			t.Fatalf("frame %d: want a partial length bound, got %.3f m with fallback %q",
				frame, f.spanBound[0], f.fallback)
		}
		// Isolate the observed side plane; other faces need not be visible
		// when membership or an occluder removes the longitudinal ends.
		f.edges = EdgeMeasurementSet{Edges: []EdgeMeasurement{face}, Rank: 1}
		if _, outcome, _ := tracker.stepNearEdge(sb, cluster, prior, f); outcome != nearEdgeFix {
			t.Fatalf("frame %d: outcome %v, want a side-face fix", frame, outcome)
		}
		if sb.ext.h[0] != length || sb.ext.phh[0] != variance {
			t.Fatalf("frame %d: a partial side span changed half-length %.6f / variance %.6f to %.6f / %.6f",
				frame, length, variance, sb.ext.h[0], sb.ext.phh[0])
		}
	}
}
