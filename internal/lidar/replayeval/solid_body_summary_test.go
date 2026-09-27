package replayeval

import (
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// summaryRows is two tracks of one online version at 10 Hz. Track 1 moves
// along +x at 10 m/s: its point estimate wobbles 0.4 m across its course on
// alternate frames, as a medoid does, and its solid body is on the body
// centre from frame 3 with no wobble. Track 2 is parked and never leaves its
// medoid seed. A point estimate of another version is filed beside them.
func summaryRows() ([]observationsqlite.TrackEstimate, []observationsqlite.TrackSolidBody) {
	const base, period = int64(1_750_000_000_000_000_000), int64(100_000_000)
	var points []observationsqlite.TrackEstimate
	var bodies []observationsqlite.TrackSolidBody
	width := func(frames int) l5tracks.DimensionBelief {
		return l5tracks.DimensionBelief{Metres: 1.8, SigmaMetres: 0.2, AdmissibleFrames: frames, Provenance: l5tracks.ProvenanceAccumulated}
	}
	for seq := int64(1); seq <= 2; seq++ {
		for f := int64(0); f < 20; f++ {
			frame := base + f*period
			x, y, vx := float32(f), float32(0), float32(10)
			if seq == 2 {
				x, y, vx = 30, 5, 0
			}
			wobble := float32(0)
			if seq == 1 && f%2 == 1 {
				wobble = 0.4
			}
			points = append(points, observationsqlite.TrackEstimate{
				TrackID: "trk", CreationSequence: seq, FrameUnixNanos: frame, EstimatorID: "cv_kf_v1",
				ParamHash: "p", Stage: "online", X: x, Y: y + wobble, VX: vx,
			})
			e := l5tracks.SolidBodyEstimate{X: x, Y: y, Reference: l5tracks.ReferenceClusterMedoid, Width: width(int(f))}
			m := l5tracks.SolidBodyMeasurement{Source: l5tracks.MeasurementMedoidV0, Rank: 2, FallbackReason: "initialisation_window"}
			if seq == 1 && f >= 3 {
				e.Reference = l5tracks.ReferenceBodyCentre
				m = l5tracks.SolidBodyMeasurement{Source: l5tracks.MeasurementNearEdgeCandidateV1, Rank: 1,
					Faces: mustVisibleFaces("right")}
				if f == 10 {
					e.Reference = l5tracks.ReferenceClusterMedoid
					e.Y = wobble
					m = l5tracks.SolidBodyMeasurement{Source: l5tracks.MeasurementMedoidV0, Rank: 2, FallbackReason: "body_centre_lapsed"}
				}
			}
			if seq == 2 {
				e.Width = l5tracks.DimensionBelief{Metres: 1.9, SigmaMetres: 1, Provenance: l5tracks.ProvenanceClassPrior}
			}
			bodies = append(bodies, observationsqlite.TrackSolidBody{
				CreationSequence: seq, FrameUnixNanos: frame, EstimatorID: "cv_kf_v1", ParamHash: "p", Stage: "online",
				Reading: l5tracks.SolidBodyReading{Estimate: e, Measurement: m},
			})
		}
	}
	points = append(points, observationsqlite.TrackEstimate{CreationSequence: 1, FrameUnixNanos: base, EstimatorID: "cv_kf_v1",
		ParamHash: "other", Stage: "online", X: 99})
	return points, bodies
}

func mustVisibleFaces(s string) l5tracks.VisibleFaces {
	v, err := l5tracks.ParseVisibleFaces(s)
	if err != nil {
		panic(err)
	}
	return v
}

func TestSummariseSolidBodies(t *testing.T) {
	points, bodies := summaryRows()
	s, err := SummariseSolidBodies(points, bodies, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	// Frames 3 to 19 of track 1, but for the lapse at frame 10.
	if s.PointEstimates != 40 || s.SolidBodies != 40 || s.NearEdgeFixes != 16 || s.FixShare != 16.0/40 || s.Lapses != 1 {
		t.Fatalf("counts %+v", s)
	}
	if s.Sources["near_edge_candidate_v1"] != 16 || s.Sources["medoid_v0"] != 24 || s.References["body_centre"] != 16 ||
		s.Ranks["1"] != 16 || s.Ranks["2"] != 24 || s.Faces["right"] != 16 ||
		s.Fallbacks["initialisation_window"] != 23 || s.Fallbacks["body_centre_lapsed"] != 1 {
		t.Fatalf("breakdown %+v", s)
	}
	if s.Tracks != 2 || s.TracksWithWidth != 1 || s.TracksWidthConverged != 1 || s.TracksEstablished != 0 ||
		s.WidthMedianMetres != float64(float32(1.8)) {
		t.Fatalf("extents %+v", s)
	}
	// The parked track is not moving, so only track 1 is scored; the body
	// is steadier than the point estimate on the same frames.
	if s.AnchorPointsAll.MovingTracks != 1 || s.AnchorPointsCentred.Windows == 0 ||
		s.AnchorPointsCentred.Windows != s.AnchorBodiesCentred.Windows ||
		!(s.AnchorBodiesCentred.P99Metres < s.AnchorPointsCentred.P99Metres) || s.AnchorBodiesCentred.MaxMetres > 1e-6 {
		t.Fatalf("anchors: all %+v, points %+v, bodies %+v", s.AnchorPointsAll, s.AnchorPointsCentred, s.AnchorBodiesCentred)
	}
	// The lapse at frame 10 splits track 1's body-centre rows into two
	// steady runs, frames 3 to 9 and 11 to 19.
	if s.SteadyRuns != 2 || s.AnchorBodiesSteady.MovingTracks != 2 || s.AnchorBodiesSteady.Windows == 0 ||
		s.AnchorBodiesSteady.Windows >= s.AnchorBodiesCentred.Windows {
		t.Fatalf("steady runs %d: bodies %+v against centred %+v", s.SteadyRuns, s.AnchorBodiesSteady, s.AnchorBodiesCentred)
	}
	// Every fix uses the right face, so each steady run is one face-stable run.
	if s.FaceStableRuns != 2 || s.AnchorBodiesFaceStable != s.AnchorBodiesSteady {
		t.Fatalf("face-stable runs %d: %+v", s.FaceStableRuns, s.AnchorBodiesFaceStable)
	}
}

// A face appearing mid-run starts a new face-stable run without ending the
// steady one.
func TestSummariseSolidBodiesSplitsAtAFaceChange(t *testing.T) {
	points, bodies := summaryRows()
	for i := range bodies {
		if bodies[i].CreationSequence == 1 && bodies[i].FrameUnixNanos >= bodies[15].FrameUnixNanos &&
			bodies[i].Reading.Measurement.Source == l5tracks.MeasurementNearEdgeCandidateV1 {
			bodies[i].Reading.Measurement.Faces = mustVisibleFaces("rear,right")
			bodies[i].Reading.Measurement.Rank = 2
		}
	}
	s, err := SummariseSolidBodies(points, bodies, l5tracks.DefaultConvergenceBounds())
	if err != nil {
		t.Fatal(err)
	}
	if s.SteadyRuns != 2 || s.FaceStableRuns != 3 || s.Faces["rear,right"] != 5 {
		t.Fatalf("steady %d, face-stable %d, faces %v", s.SteadyRuns, s.FaceStableRuns, s.Faces)
	}
}

func TestSummariseSolidBodiesRefusesMixedVersions(t *testing.T) {
	points, bodies := summaryRows()
	bodies[5].ParamHash = "other"
	if _, err := SummariseSolidBodies(points, bodies, l5tracks.DefaultConvergenceBounds()); err == nil ||
		!strings.Contains(err.Error(), "mix versions") {
		t.Fatalf("mixed versions: %v", err)
	}
	s, err := SummariseSolidBodies(points, nil, l5tracks.DefaultConvergenceBounds())
	if err != nil || s.SolidBodies != 0 || s.FixShare != 0 {
		t.Fatalf("no solid bodies: %+v, %v", s, err)
	}
}
