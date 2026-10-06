package l4bobserve

import (
	"fmt"
	"math"
)

// DiffFrames returns nil when a and b are the same frame value for value,
// and otherwise an error naming the first difference. It is the equality
// FrameDigest hashes, made legible for a person comparing two extractions:
// float bits (so -0 differs from +0 and NaN payloads count), presence, order
// and identity. As in the digest, a nil and an empty column are equal, and an
// unknown Count's stray Value is not compared.
func DiffFrames(a, b FrameRecord) error {
	if err := diffFrameHeader(a, b); err != nil {
		return err
	}
	if len(a.Stages) != len(b.Stages) {
		return fmt.Errorf("stages: %d vs %d", len(a.Stages), len(b.Stages))
	}
	for i := range a.Stages {
		sa, sb := a.Stages[i], b.Stages[i]
		if sa.Stage != sb.Stage || !sameCount(sa.Input, sb.Input) || !sameCount(sa.Output, sb.Output) || !sameCount(sa.Rejected, sb.Rejected) {
			return fmt.Errorf("stage %d: %+v vs %+v", i, sa, sb)
		}
	}
	if err := diffPoints(a.Points, b.Points); err != nil {
		return fmt.Errorf("points: %w", err)
	}
	if len(a.Clusters) != len(b.Clusters) {
		return fmt.Errorf("clusters: %d vs %d", len(a.Clusters), len(b.Clusters))
	}
	for i := range a.Clusters {
		if err := diffCluster(a.Clusters[i], b.Clusters[i]); err != nil {
			return fmt.Errorf("cluster %d: %w", i, err)
		}
	}
	if err := diffSlice("unassigned", a.Unassigned, b.Unassigned); err != nil {
		return err
	}
	return diffSlice("unassigned reasons", a.UnassignedReasons, b.UnassignedReasons)
}

func diffFrameHeader(a, b FrameRecord) error {
	switch {
	case a.Sequence != b.Sequence:
		return fmt.Errorf("sequence: %d vs %d", a.Sequence, b.Sequence)
	case a.SensorFrameID != b.SensorFrameID:
		return fmt.Errorf("sensor frame id: %q vs %q", a.SensorFrameID, b.SensorFrameID)
	case a.FrameUnixNanos != b.FrameUnixNanos:
		return fmt.Errorf("frame time: %d vs %d", a.FrameUnixNanos, b.FrameUnixNanos)
	case a.CaptureStartUnixNanos != b.CaptureStartUnixNanos || a.CaptureEndUnixNanos != b.CaptureEndUnixNanos:
		return fmt.Errorf("capture interval: [%d, %d] vs [%d, %d]", a.CaptureStartUnixNanos, a.CaptureEndUnixNanos, b.CaptureStartUnixNanos, b.CaptureEndUnixNanos)
	case a.Completeness.State != b.Completeness.State || !sameCount(a.Completeness.LostPackets, b.Completeness.LostPackets):
		return fmt.Errorf("completeness: %s/%s vs %s/%s", a.Completeness.State, a.Completeness.LostPackets, b.Completeness.State, b.Completeness.LostPackets)
	case a.Background != b.Background:
		return fmt.Errorf("background: %s vs %s", a.Background, b.Background)
	case a.Disposition != b.Disposition:
		return fmt.Errorf("disposition: %+v vs %+v", a.Disposition, b.Disposition)
	case a.Payload != b.Payload:
		return fmt.Errorf("payload: %#x vs %#x", uint8(a.Payload), uint8(b.Payload))
	}
	return nil
}

func diffPoints(a, b RetainedPoints) error {
	if a.Fields != b.Fields {
		return fmt.Errorf("fields: %#x vs %#x", uint16(a.Fields), uint16(b.Fields))
	}
	for _, column := range []struct {
		name string
		a, b []float64
	}{{"x", a.X, b.X}, {"y", a.Y, b.Y}, {"z", a.Z, b.Z}} {
		if err := diffFloat64s(column.name, column.a, column.b); err != nil {
			return err
		}
	}
	if err := diffSlice("time offset", a.TimeOffsetNanos, b.TimeOffsetNanos); err != nil {
		return err
	}
	if err := diffSlice("intensity", a.Intensity, b.Intensity); err != nil {
		return err
	}
	if err := diffSlice("channel", a.Channel, b.Channel); err != nil {
		return err
	}
	if err := diffSlice("return index", a.ReturnIndex, b.ReturnIndex); err != nil {
		return err
	}
	if err := diffSlice("source ordinal", a.SourceOrdinal, b.SourceOrdinal); err != nil {
		return err
	}
	if err := diffSlice("packet sequence", a.PacketSequence, b.PacketSequence); err != nil {
		return err
	}
	return diffSlice("block index", a.BlockIndex, b.BlockIndex)
}

func diffCluster(a, b ClusterRecord) error {
	if a.ClusterID != b.ClusterID {
		return fmt.Errorf("id: %d vs %d", a.ClusterID, b.ClusterID)
	}
	if err := diffSlice("members", a.Members, b.Members); err != nil {
		return err
	}
	sa, sb := a.Summary, b.Summary
	if sa.FirstMemberUnixNanos != sb.FirstMemberUnixNanos || sa.PointsCount != sb.PointsCount || sa.GroundClipped != sb.GroundClipped {
		return fmt.Errorf("summary: %+v vs %+v", sa, sb)
	}
	fa := []float32{sa.CentroidX, sa.CentroidY, sa.CentroidZ, sa.BoundingBoxLength, sa.BoundingBoxWidth, sa.BoundingBoxHeight, sa.HeightP95, sa.IntensityMean}
	fb := []float32{sb.CentroidX, sb.CentroidY, sb.CentroidZ, sb.BoundingBoxLength, sb.BoundingBoxWidth, sb.BoundingBoxHeight, sb.HeightP95, sb.IntensityMean}
	if err := diffFloat32s("summary", fa, fb); err != nil {
		return err
	}
	if (sa.OBB == nil) != (sb.OBB == nil) {
		return fmt.Errorf("oriented box present: %v vs %v", sa.OBB != nil, sb.OBB != nil)
	}
	if sa.OBB == nil {
		return nil
	}
	oa, ob := *sa.OBB, *sb.OBB
	return diffFloat32s("oriented box",
		[]float32{oa.CenterX, oa.CenterY, oa.CenterZ, oa.Length, oa.Width, oa.Height, oa.HeadingRad},
		[]float32{ob.CenterX, ob.CenterY, ob.CenterZ, ob.Length, ob.Width, ob.Height, ob.HeadingRad})
}

// DiffGaps is DiffFrames for gap records.
func DiffGaps(a, b GapRecord) error {
	if GapDigest(a) == GapDigest(b) {
		return nil
	}
	return fmt.Errorf("gap: %+v vs %+v", a, b)
}

func sameCount(a, b Count) bool {
	return a.Known == b.Known && (!a.Known || a.Value == b.Value)
}

func diffSlice[T comparable](name string, a, b []T) error {
	if len(a) != len(b) {
		return fmt.Errorf("%s: %d values vs %d", name, len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			return fmt.Errorf("%s[%d]: %v vs %v", name, i, a[i], b[i])
		}
	}
	return nil
}

func diffFloat64s(name string, a, b []float64) error {
	if len(a) != len(b) {
		return fmt.Errorf("%s: %d values vs %d", name, len(a), len(b))
	}
	for i := range a {
		if math.Float64bits(a[i]) != math.Float64bits(b[i]) {
			return fmt.Errorf("%s[%d]: bits %#016x vs %#016x", name, i, math.Float64bits(a[i]), math.Float64bits(b[i]))
		}
	}
	return nil
}

func diffFloat32s(name string, a, b []float32) error {
	for i := range a {
		if math.Float32bits(a[i]) != math.Float32bits(b[i]) {
			return fmt.Errorf("%s value %d: bits %#08x vs %#08x", name, i, math.Float32bits(a[i]), math.Float32bits(b[i]))
		}
	}
	return nil
}
