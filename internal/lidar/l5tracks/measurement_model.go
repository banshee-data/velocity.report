package l5tracks

// The near-edge measurement model, per Sections 9.1 and 3.3 of
// docs/plans/lidar-state-estimation-plan.md.
//
// The idea is one sentence long: measure only the surface that was actually
// observed, and get the rest from a prior. The near face of a vehicle is
// densely and reliably sampled; the far face is not observed at all. A model
// that fits a box to a partial view and reports its centre inherits the error
// in the unseen half, which is what makes the OBB centre hop by half a metre
// while nothing physical moves.
//
// The property that makes this robust to occlusion is that **the number of
// measurement dimensions varies with what was actually observed**. A frame
// showing only the near lateral face constrains lateral position and nothing
// else, and this model says so by producing a rank-one measurement rather than
// a rank-two one with a fudged covariance.

import (
	"math"

	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
)

// BodyFace names one of the four vertical faces of the body, in the body's own
// frame rather than the site frame.
type BodyFace uint8

const (
	// FaceFront is the face whose outward normal points along +heading.
	FaceFront BodyFace = iota
	// FaceRear points along -heading.
	FaceRear
	// FaceLeft and FaceRight point along the perpendicular axis.
	FaceLeft
	FaceRight
)

// String names a face for diagnostics and residual records.
func (f BodyFace) String() string {
	switch f {
	case FaceFront:
		return "front"
	case FaceRear:
		return "rear"
	case FaceLeft:
		return "left"
	default:
		return "right"
	}
}

// IsLongitudinal reports whether the face's normal lies along the body's length
// axis. The two longitudinal faces constrain along-track position; the two
// lateral faces constrain across-track position.
func (f BodyFace) IsLongitudinal() bool {
	return f == FaceFront || f == FaceRear
}

// EdgeMeasurement is one observed face: where its plane was measured, and the
// half-extent that separates it from the body centre.
//
// It is deliberately not a point measurement. A face constrains the body along
// one axis — its own normal — and says nothing about the perpendicular one.
type EdgeMeasurement struct {
	Face BodyFace
	// NormalX, NormalY is the face's outward unit normal in the site frame,
	// pointing away from the body centre.
	NormalX, NormalY float32
	// PlaneOffsetMetres is the measured position of the face plane along its
	// own normal, as a percentile of the supporting returns rather than their
	// extreme, so one stray return cannot define the surface.
	PlaneOffsetMetres float32
	// HalfExtentMetres is half the body's dimension along this normal, and
	// HalfExtentProvenance says whether that came from evidence or a prior.
	// This is the term that does the work the sensor cannot: the far face was
	// not observed, so its position is inferred from the body's believed size.
	HalfExtentMetres     float32
	HalfExtentProvenance Provenance
	// SupportPoints is how many returns were attributed to this face.
	SupportPoints int
}

// ImpliedCentreOffset is the body centre's position along this face's normal,
// implied by the face plane and the half-extent. The centre lies inboard of
// the face, hence the subtraction.
func (e EdgeMeasurement) ImpliedCentreOffset() float32 {
	return e.PlaneOffsetMetres - e.HalfExtentMetres
}

// EdgeMeasurementSet is a variable-rank measurement of the body centre.
//
// Rank is the number of independent directions the observed faces constrain:
// two perpendicular faces pin the centre in the plane, one face pins it along a
// line and leaves the perpendicular direction entirely to the prediction. A
// caller must respect the rank rather than treating the reconstructed centre as
// a full two-dimensional fix.
type EdgeMeasurementSet struct {
	Edges []EdgeMeasurement
	// CentreX, CentreY is the reconstructed body centre. Along an unconstrained
	// direction it carries the prediction that was supplied, so it is only a
	// complete answer when Rank is 2.
	CentreX, CentreY float32
	// Rank is 1 or 2. Zero means no usable measurement.
	Rank int
	// ConstrainedX, ConstrainedY is the unit direction the rank-one
	// measurement constrains. Meaningless when Rank is 2.
	ConstrainedX, ConstrainedY float32
	// Source labels the geometry that produced this, for the residual record.
	Source MeasurementSource
	// FallbackReason is set when no measurement could be produced.
	FallbackReason string
}

// NearEdgeInput is everything the model needs. The sensor origin and the body
// heading must be in the same site frame as the cluster.
type NearEdgeInput struct {
	Cluster l4perception.WorldCluster
	// Points are the cluster's member returns, passed explicitly rather than
	// read off the cluster because WorldCluster.RetainedPoints is populated
	// only when MaxSamplePoints is set, and is a uniform subsample capped at
	// 1024 when it is. A percentile tolerates uniform subsampling — the
	// distribution is preserved — but the support counts shrink with it, so
	// MinFaceSupport must be read against however many points were actually
	// supplied rather than against the cluster's full PointsCount.
	Points []l4perception.WorldPoint
	// SensorX, SensorY is the calibrated sensor origin. Without it there is no
	// way to know which face is the near one, and the model refuses rather
	// than guessing.
	SensorX, SensorY float32
	// HeadingRad is the believed body orientation. A rank-two measurement is
	// only meaningful if the body's axes are known, so this must come from a
	// resolved orientation belief.
	HeadingRad float32
	// HalfLength and HalfWidth are half the believed body dimensions, with the
	// provenance of each. These supply the unobserved half.
	HalfLength, HalfWidth             float32
	LengthProvenance, WidthProvenance Provenance
	// PredictedX, PredictedY fills the direction a rank-one measurement leaves
	// unconstrained.
	PredictedX, PredictedY float32
	// FaceToleranceMetres is how close to a face plane a return must lie to be
	// attributed to it.
	FaceToleranceMetres float32
	// MinFaceSupport is the fewest returns that may define a face.
	MinFaceSupport int
}

// DefaultFaceToleranceMetres is how near a face plane a return must sit to
// count as lying on it. It is a few centimetres: wide enough to absorb the
// sensor's specified range accuracy, narrow enough that returns from the roof
// or the opposite face are not swept in.
const DefaultFaceToleranceMetres = 0.15

// DefaultMinFaceSupport is the fewest returns that may define a face plane.
// Below this the percentile is not meaningful and the face is not claimed.
const DefaultMinFaceSupport = 8

// MeasureNearEdge builds a variable-rank measurement from the faces the sensor
// actually saw.
//
// It refuses, rather than approximating, when the geometry needed to identify a
// face is missing: no calibrated sensor origin means the near face cannot be
// told from the far one, and no believed dimension along a normal means the
// unobserved half cannot be supplied. Both return a FallbackReason so the
// caller can record why no face was claimed.
func MeasureNearEdge(in NearEdgeInput) EdgeMeasurementSet {
	out := EdgeMeasurementSet{Source: MeasurementNearEdgeCandidateV1}

	if !finiteMeasurementCoordinate(in.SensorX) || !finiteMeasurementCoordinate(in.SensorY) {
		out.FallbackReason = "missing_calibrated_sensor_origin"
		return out
	}
	if !finiteMeasurementCoordinate(in.HeadingRad) {
		out.FallbackReason = "missing_heading"
		return out
	}
	points := in.Points
	if len(points) == 0 {
		out.FallbackReason = "no_cluster_points"
		return out
	}

	tolerance := in.FaceToleranceMetres
	if tolerance <= 0 {
		tolerance = DefaultFaceToleranceMetres
	}
	minSupport := in.MinFaceSupport
	if minSupport <= 0 {
		minSupport = DefaultMinFaceSupport
	}

	// The body's own axes in the site frame.
	cos := float32(math.Cos(float64(in.HeadingRad)))
	sin := float32(math.Sin(float64(in.HeadingRad)))
	axes := [2]struct {
		dirX, dirY            float32
		halfExtent            float32
		provenance            Provenance
		positiveFace, negFace BodyFace
	}{
		// Longitudinal: the length axis.
		{cos, sin, in.HalfLength, in.LengthProvenance, FaceFront, FaceRear},
		// Lateral: the width axis, perpendicular.
		{-sin, cos, in.HalfWidth, in.WidthProvenance, FaceLeft, FaceRight},
	}

	// A rough centre, needed only to decide which side of the body the sensor
	// is on. The OBB centre is biased, but the bias is far smaller than half a
	// body, so it is adequate for a sign decision and for nothing else.
	//
	// It falls back to the points' own mean when the cluster carries no
	// centroid, rather than leaving it at the origin: a rough centre that
	// coincides with the sensor makes every face equidistant, and the model
	// would then refuse a perfectly measurable frame for no reason the caller
	// could see.
	roughX, roughY := in.Cluster.CentroidX, in.Cluster.CentroidY
	if obb := in.Cluster.OBB; obb != nil &&
		finiteMeasurementCoordinate(obb.CenterX) && finiteMeasurementCoordinate(obb.CenterY) {
		roughX, roughY = obb.CenterX, obb.CenterY
	} else if roughX == 0 && roughY == 0 {
		var sumX, sumY float64
		for _, p := range points {
			sumX += p.X
			sumY += p.Y
		}
		roughX = float32(sumX / float64(len(points)))
		roughY = float32(sumY / float64(len(points)))
	}

	for _, axis := range axes {
		if axis.halfExtent <= 0 || axis.provenance == ProvenanceNone {
			// No believed dimension along this normal, so the unobserved half
			// cannot be supplied and this axis contributes nothing.
			continue
		}

		// Which way does the sensor lie along this axis? That face is the near
		// one, and the only one worth measuring.
		toSensor := (in.SensorX-roughX)*axis.dirX + (in.SensorY-roughY)*axis.dirY
		if toSensor == 0 {
			// Edge-on: neither face of this pair is nearer, so there is no
			// near face to claim.
			continue
		}
		sign := float32(1)
		face := axis.positiveFace
		if toSensor < 0 {
			sign = -1
			face = axis.negFace
		}
		// The outward normal of the near face points toward the sensor.
		normalX, normalY := sign*axis.dirX, sign*axis.dirY

		// The near face is the extreme of the point set along that normal, so
		// take a high percentile rather than the maximum: one stray return
		// must not define the surface.
		plane, support, ok := facePlaneOffset(points, normalX, normalY, tolerance)
		if !ok || support < minSupport {
			continue
		}

		out.Edges = append(out.Edges, EdgeMeasurement{
			Face:                 face,
			NormalX:              normalX,
			NormalY:              normalY,
			PlaneOffsetMetres:    plane,
			HalfExtentMetres:     axis.halfExtent,
			HalfExtentProvenance: axis.provenance,
			SupportPoints:        support,
		})
	}

	return assembleCentre(out, in)
}

// facePlaneOffset locates the face plane nearest the sensor along a normal, and
// counts the returns lying on it.
//
// The plane is placed at the (100 - NearEdgePercentile)th percentile of the
// projections, which is the near-edge percentile measured from the far end: the
// near face has the largest projection along its own outward normal.
func facePlaneOffset(points []l4perception.WorldPoint, normalX, normalY, tolerance float32) (float32, int, bool) {
	offset, ok := l4perception.LateralPercentile(
		points, float64(normalX), float64(normalY), 100-l4perception.NearEdgePercentile)
	if !ok {
		return 0, 0, false
	}
	plane := float32(offset)
	if !finiteMeasurementCoordinate(plane) {
		return 0, 0, false
	}

	// Count the support: returns within tolerance of the plane.
	var support int
	for _, p := range points {
		projection := float32(p.X*float64(normalX) + p.Y*float64(normalY))
		if projection >= plane-tolerance {
			support++
		}
	}
	return plane, support, true
}

// assembleCentre turns the observed faces into a body centre and a rank.
func assembleCentre(out EdgeMeasurementSet, in NearEdgeInput) EdgeMeasurementSet {
	switch len(out.Edges) {
	case 0:
		if out.FallbackReason == "" {
			out.FallbackReason = "no_face_reached_minimum_support"
		}
		return out

	case 1:
		// One face: the centre is pinned along that normal and is otherwise
		// wherever the prediction says. Reporting this as a two-dimensional
		// fix is exactly the error that drags a partially occluded vehicle
		// sideways, so the rank records that only one direction was measured.
		e := out.Edges[0]
		constraint := e.ImpliedCentreOffset()
		predictedAlong := in.PredictedX*e.NormalX + in.PredictedY*e.NormalY
		correction := constraint - predictedAlong
		out.CentreX = in.PredictedX + correction*e.NormalX
		out.CentreY = in.PredictedY + correction*e.NormalY
		out.Rank = 1
		out.ConstrainedX, out.ConstrainedY = e.NormalX, e.NormalY
		return out

	default:
		// Two perpendicular faces pin the centre in the plane. Each supplies
		// its own axis; neither is asked about the other.
		a, b := out.Edges[0], out.Edges[1]
		oa, ob := a.ImpliedCentreOffset(), b.ImpliedCentreOffset()
		// Solve for the point whose projections onto the two normals are oa
		// and ob. The normals are perpendicular unit vectors, so the inverse
		// is the transpose.
		det := a.NormalX*b.NormalY - a.NormalY*b.NormalX
		if math.Abs(float64(det)) < 1e-6 {
			// Parallel faces cannot fix a plane position; keep the better
			// supported one as a rank-one measurement.
			if b.SupportPoints > a.SupportPoints {
				out.Edges = []EdgeMeasurement{b}
			} else {
				out.Edges = []EdgeMeasurement{a}
			}
			return assembleCentre(out, in)
		}
		out.CentreX = (oa*b.NormalY - ob*a.NormalY) / det
		out.CentreY = (ob*a.NormalX - oa*b.NormalX) / det
		out.Rank = 2
		return out
	}
}

// Covariance is the measurement covariance implied by the set's rank.
//
// A rank-one measurement gets a tight variance along the direction it
// constrains and an effectively unbounded one across it, which is how the
// filter is told to keep its prediction in the unmeasured direction rather than
// being pulled toward a fabricated position. That is the whole point of
// carrying a rank: the alternative is an isotropic covariance that quietly
// invents information.
//
// alongVariance is the variance to use along a constrained direction.
// unconstrainedVariance is what to use across it, and should be large enough
// that the Kalman gain in that direction is negligible.
func (s EdgeMeasurementSet) Covariance(alongVariance, unconstrainedVariance float32) MeasurementCovariance {
	switch s.Rank {
	case 2:
		return MeasurementCovariance{XX: alongVariance, YY: alongVariance}
	case 1:
		// Build u*along*u' + v*unconstrained*v' for the constrained direction
		// u and its perpendicular v.
		ux, uy := s.ConstrainedX, s.ConstrainedY
		vx, vy := -uy, ux
		return MeasurementCovariance{
			XX: alongVariance*ux*ux + unconstrainedVariance*vx*vx,
			XY: alongVariance*ux*uy + unconstrainedVariance*vx*vy,
			YY: alongVariance*uy*uy + unconstrainedVariance*vy*vy,
		}
	default:
		return MeasurementCovariance{XX: unconstrainedVariance, YY: unconstrainedVariance}
	}
}

// UsesInferredExtent reports whether any face's half-extent came from a class
// prior rather than evidence about this object. The measurement is still
// usable — the far face is never observed, so some prior is unavoidable — but a
// consumer should know the reconstruction leaned on an assumed size.
func (s EdgeMeasurementSet) UsesInferredExtent() bool {
	for _, e := range s.Edges {
		if e.HalfExtentProvenance == ProvenanceClassPrior {
			return true
		}
	}
	return false
}
