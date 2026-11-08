package annotation

import (
	"fmt"
	"math"

	pb "github.com/banshee-data/velocity.report/internal/lidar/recordingpb"
)

// featureSegmentRelation derives a horizontal infinite line from explicit
// direction-defining returns. The changing visible midpoint is not a body
// point. The conservative offset bound leaves tangent position unconstrained.
// Every return must lie within returnBound of the endpoint line in plan view.
func featureSegmentRelation(points Points, indices []uint32, start, end uint32, centre PlanarBound, yaw AngleBound, returnBound float64) (*pb.FeatureLineConstraint, float64, error) {
	if start == end || !finiteFeature(returnBound) || returnBound <= 0 || !finiteFeature(centre.XM) || !finiteFeature(centre.YM) || !finiteFeature(centre.BoundM) || centre.BoundM < 0 || !finiteFeature(yaw.Rad) || !finiteFeature(yaw.BoundRad) || yaw.BoundRad < 0 {
		return nil, 0, fmt.Errorf("invalid segment inputs")
	}
	unique := map[[3]float32]bool{}
	foundA, foundB := false, false
	for _, index := range indices {
		if int(index) >= len(points.X) || int(index) >= len(points.Y) || int(index) >= len(points.Z) {
			return nil, 0, fmt.Errorf("segment return outside source sample")
		}
		point := [3]float32{points.X[index], points.Y[index], points.Z[index]}
		for _, value := range point {
			if !finiteFeature(float64(value)) {
				return nil, 0, fmt.Errorf("non-finite segment return")
			}
		}
		unique[point] = true
		foundA = foundA || index == start
		foundB = foundB || index == end
	}
	if !foundA || !foundB || len(unique) < 3 {
		return nil, 0, fmt.Errorf("segment needs three distinct returns and two direction-defining support indices")
	}
	ax, ay := float64(points.X[start]), float64(points.Y[start])
	vx, vy := float64(points.X[end])-ax, float64(points.Y[end])-ay
	span := math.Hypot(vx, vy)
	if span < 0.1 || returnBound*2 >= span {
		return nil, 0, fmt.Errorf("insufficient horizontal segment span for the return bound")
	}
	// Straightness is tested in plan view only. A constant-elevation ring on a
	// flat vertical face is straight in (x, y) but sags in z across the face
	// (0.13-0.22 m over 4.5 m at 5 m on steep rings), so a 3D distance would
	// refuse a flat panel. The relation is horizontal, so z carries no part
	// of it and is not tested.
	for _, index := range indices {
		dx, dy := float64(points.X[index])-ax, float64(points.Y[index])-ay
		if math.Abs(dx*vy-dy*vx)/span > returnBound {
			return nil, 0, fmt.Errorf("selected support is not straight in plan view within the return bound")
		}
	}
	c, s := math.Cos(yaw.Rad), math.Sin(yaw.Rad)
	bx, by := c*vx+s*vy, -s*vx+c*vy
	nx, ny := -by/span, bx/span
	major := nx
	if math.Abs(ny) > math.Abs(nx) {
		major = ny
	}
	if major < 0 {
		nx, ny = -nx, -ny
	}
	angular := yaw.BoundRad + math.Asin(2*returnBound/span)
	if !finiteFeature(angular) || angular >= math.Pi/2 {
		return nil, 0, fmt.Errorf("segment orientation uncertainty is too broad")
	}
	dx, dy := ax-centre.XM, ay-centre.YM
	mx, my := c*dx+s*dy+bx/2, -s*dx+c*dy+by/2
	bound := centre.BoundM + returnBound + (math.Hypot(mx, my)+centre.BoundM+returnBound)*2*math.Sin(angular/2)
	if !finiteFeature(bound) {
		return nil, 0, fmt.Errorf("segment bound overflow")
	}
	return &pb.FeatureLineConstraint{NormalX: nx, NormalY: ny, OffsetM: nx*mx + ny*my, NormalBoundRad: angular, SourceStartIndex: &start, SourceEndIndex: &end}, bound, nil
}

func validateFeatureSegment(points Points, source *pb.FeatureObservation, anchor *pb.FeatureAnchor, centre PlanarBound, yaw AngleBound) error {
	line := anchor.Line
	if line == nil || len(line.ProtoReflect().GetUnknown()) != 0 || line.SourceStartIndex == nil || line.SourceEndIndex == nil || anchor.SourcePointIndex != nil || anchor.XM != 0 || anchor.YM != 0 || !finiteFeature(line.NormalX) || !finiteFeature(line.NormalY) || !finiteFeature(line.OffsetM) || !finiteFeature(line.NormalBoundRad) {
		return fmt.Errorf("invalid weak-direction segment registration")
	}
	expected, bound, err := featureSegmentRelation(points, source.PointIndices, *line.SourceStartIndex, *line.SourceEndIndex, centre, yaw, anchor.ReturnBoundM)
	if err != nil {
		return err
	}
	if math.Abs(expected.NormalX-line.NormalX) > 1e-5 || math.Abs(expected.NormalY-line.NormalY) > 1e-5 || math.Abs(expected.OffsetM-line.OffsetM) > 1e-5 || line.NormalBoundRad+1e-9 < expected.NormalBoundRad || line.NormalBoundRad >= math.Pi/2 || anchor.BoundM+1e-9 < bound {
		return fmt.Errorf("segment relation or bounds disagree with pinned evidence")
	}
	// A deliberately widened angular bound also widens the offset bound.
	mx := (float64(points.X[*line.SourceStartIndex])+float64(points.X[*line.SourceEndIndex]))/2 - centre.XM
	my := (float64(points.Y[*line.SourceStartIndex])+float64(points.Y[*line.SourceEndIndex]))/2 - centre.YM
	minimum := centre.BoundM + anchor.ReturnBoundM + (math.Hypot(mx, my)+centre.BoundM+anchor.ReturnBoundM)*2*math.Sin(line.NormalBoundRad/2)
	if anchor.BoundM+1e-9 < minimum {
		return fmt.Errorf("segment offset bound does not cover its stated angular uncertainty")
	}
	return nil
}
