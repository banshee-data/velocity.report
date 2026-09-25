package vrlog

import (
	"errors"
	"fmt"
	"io"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
)

const testFrameStart = int64(1_700_000_000_000_000_000)

// testManifest is a manifest whose identities recompute: a replayed source
// and an identity calibration.
func testManifest(t testing.TB) Manifest {
	t.Helper()
	calibration := l4bobserve.Calibration{SensorID: "hesai-test", FromFrame: "sensor", ToFrame: "site",
		Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
	calibrationID, err := l4bobserve.CalibrationID(calibration)
	if err != nil {
		t.Fatal(err)
	}
	files := []CaptureFile{{Path: "/captures/test.pcapng", SHA256: strings.Repeat("ab", 32)}}
	sourceID, err := l4bobserve.SourceID(l4bobserve.CaptureSource{ReplayCaseID: "synthetic-case",
		CapturePaths: []string{files[0].Path}, CaptureSHA256s: []string{files[0].SHA256}, ExtractorID: "l4.test/v1"})
	if err != nil {
		t.Fatal(err)
	}
	return Manifest{
		Capture:     CaptureIdentity{SensorID: "hesai-test", SourceType: "synthetic"},
		Extraction:  ExtractionIdentity{SourceID: sourceID, CalibrationID: calibrationID, CoordinateFrame: "site/hesai-test", ReplayCaseID: "synthetic-case", CaptureFiles: files, ExtractorID: "l4.test/v1"},
		Calibration: calibration,
		Provenance:  Provenance{Writer: "vrlog-test", ParamsHash: "sha256:test"},
		Metadata:    []MetadataObject{NewMetadataObject("tuning", "application/json", []byte(`{"l4":{}}`))},
	}
}

// synthFrame builds a foreground-complete frame of n points. Points 0 and 1
// are coincident (identical coordinates, distinct returns); the first cluster
// holds both boundary indices 0 and n-1; coordinates include -0, a subnormal
// and values that do not round-trip through float32.
func synthFrame(seq uint64, n int) l4bobserve.FrameRecord {
	start := testFrameStart + int64(seq)*100_000_000
	f := l4bobserve.FrameRecord{
		Sequence: seq, SensorFrameID: fmt.Sprintf("hesai-test-%d", seq),
		FrameUnixNanos: start, CaptureStartUnixNanos: start, CaptureEndUnixNanos: start + 99_000_000,
		Completeness: l4bobserve.Completeness{State: l4bobserve.CompletenessComplete, LostPackets: l4bobserve.KnownCount(0)},
		Background:   l4bobserve.BackgroundSettled,
		Disposition:  l4bobserve.Disposition{Kind: l4bobserve.DispositionObserved},
		Payload:      l4bobserve.PayloadPoints | l4bobserve.PayloadMembership,
		Stages: []l4bobserve.StageCount{
			{Stage: l4bobserve.StageL2Frame, Output: l4bobserve.KnownCount(n + 7)},
			{Stage: l4bobserve.StageL3Foreground, Input: l4bobserve.KnownCount(n + 7), Output: l4bobserve.KnownCount(n), Rejected: l4bobserve.KnownCount(7)},
		},
	}
	p := l4bobserve.RetainedPoints{Fields: l4bobserve.FieldAcquisitionTime | l4bobserve.FieldIntensity | l4bobserve.FieldChannel |
		l4bobserve.FieldSourceOrdinal | l4bobserve.FieldPacketSequence | l4bobserve.FieldBlockIndex}
	for i := range n {
		x := 10 + 0.1*float64(i) + 1e-12*float64(seq)
		y, z := -3+math.Sqrt(float64(i)), 0.1*float64(i%7)
		switch i {
		case 1:
			x, y, z = p.X[0], p.Y[0], p.Z[0]
		case 2:
			x, z = math.Copysign(0, -1), math.SmallestNonzeroFloat64
		}
		p.X, p.Y, p.Z = append(p.X, x), append(p.Y, y), append(p.Z, z)
		p.TimeOffsetNanos = append(p.TimeOffsetNanos, int64(i)*1000)
		p.Intensity = append(p.Intensity, uint8(i*37))
		p.Channel = append(p.Channel, uint16(i%40))
		p.SourceOrdinal = append(p.SourceOrdinal, uint32(3*i+1))
		p.PacketSequence = append(p.PacketSequence, uint32(1<<31+i/10))
		p.BlockIndex = append(p.BlockIndex, uint16(i%10))
	}
	f.Points = p
	if n == 0 {
		f.Stages = append(f.Stages, l4bobserve.StageCount{Stage: l4bobserve.StageL4Cluster, Input: l4bobserve.KnownCount(0), Output: l4bobserve.KnownCount(0), Rejected: l4bobserve.KnownCount(0)})
		return f
	}
	var a, b []uint32
	for i := range n {
		switch {
		case i == 0 || i == n-1 || (i%3 == 0 && i < n/2):
			a = append(a, uint32(i))
		case i%3 == 1 && i > n/2:
			b = append(b, uint32(i))
		default:
			f.Unassigned = append(f.Unassigned, uint32(i))
			f.UnassignedReasons = append(f.UnassignedReasons, l4bobserve.RejectionReason(i%8))
		}
	}
	obb := &l4bobserve.OrientedBox{CenterX: 10.5, CenterY: -2.25, Length: 4.5, Width: 1.8, Height: 1.5, HeadingRad: float32(math.Pi)}
	f.Clusters = append(f.Clusters, l4bobserve.ClusterRecord{ClusterID: 1, Members: a, Summary: l4bobserve.ClusterSummary{
		FirstMemberUnixNanos: start, CentroidX: float32(p.X[0]), CentroidY: float32(p.Y[0]), CentroidZ: float32(p.Z[0]),
		BoundingBoxLength: 4.5, BoundingBoxWidth: 1.8, BoundingBoxHeight: 1.5, HeightP95: 1.4,
		IntensityMean: math.Float32frombits(0x7fc00123), // a NaN payload the codec must keep
		PointsCount:   len(a), GroundClipped: true, OBB: obb}})
	if len(b) > 0 {
		f.Clusters = append(f.Clusters, l4bobserve.ClusterRecord{ClusterID: 7, Members: b, Summary: l4bobserve.ClusterSummary{
			FirstMemberUnixNanos: start + 5, CentroidX: 1, PointsCount: len(b)}})
	}
	members := len(a) + len(b)
	f.Stages = append(f.Stages, l4bobserve.StageCount{Stage: l4bobserve.StageL4Cluster, Input: l4bobserve.KnownCount(n),
		Output: l4bobserve.KnownCount(members), Rejected: l4bobserve.KnownCount(n - members)})
	return f
}

// dispositionFrames are one frame of each processing outcome, all valid
// for the foreground-complete profile, starting at seq.
func dispositionFrames(seq uint64) []l4bobserve.FrameRecord {
	empty := synthFrame(seq, 0)

	unsettled := synthFrame(seq+1, 12)
	unsettled.Background = l4bobserve.BackgroundSettling
	unsettled.Disposition = l4bobserve.Disposition{Kind: l4bobserve.DispositionUnsettled}
	unsettled.Completeness = l4bobserve.Completeness{State: l4bobserve.CompletenessPartial, LostPackets: l4bobserve.KnownCount(3)}

	suppressed := synthFrame(seq+2, 9)
	suppressed.Disposition = l4bobserve.Disposition{Kind: l4bobserve.DispositionSuppressed, Stage: l4bobserve.StageL4Transform, Reason: "pipeline profile stops at L3"}
	suppressed.Payload = l4bobserve.PayloadPoints
	suppressed.Clusters, suppressed.Unassigned, suppressed.UnassignedReasons = nil, nil, nil
	suppressed.Stages = suppressed.Stages[:2]
	suppressed.Completeness = l4bobserve.Completeness{State: l4bobserve.CompletenessUnknown}

	failed := synthFrame(seq+3, 0)
	failed.Disposition = l4bobserve.Disposition{Kind: l4bobserve.DispositionFailed, Stage: l4bobserve.StageL3Foreground, Reason: "foreground mask unavailable"}
	failed.Payload = 0
	failed.Points = l4bobserve.RetainedPoints{}
	failed.Stages = failed.Stages[:1]
	return []l4bobserve.FrameRecord{empty, unsettled, suppressed, failed}
}

// createTest returns a writer in a fresh temporary directory.
func createTest(t testing.TB, m Manifest) (*Writer, string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "capture.vrlog")
	w, err := Create(dir, m)
	if err != nil {
		t.Fatal(err)
	}
	// Stop the committer of a writer a test left open; a no-op otherwise.
	t.Cleanup(func() { _ = w.Abandon() })
	return w, dir
}

// readAll opens dir and returns every record in order.
func readAll(t testing.TB, dir string, opts Options) []Record {
	t.Helper()
	r, err := Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	var out []Record
	for {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, rec)
	}
}
