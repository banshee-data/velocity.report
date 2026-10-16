package evalfixture

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
)

// The fate fixture: the scene's pack with returns that name real packets, the
// capture holding those packets, and a foreground-complete observation
// container over it, for the tests of the fate measure and its command.
//
// Packets. Each sample i has two sequence-enabled Pandar40P payloads: packet
// A (sequence 100+2i) carries obj_car_a's returns 0 and 1 in block 0,
// channels 1 and 2; packet B (sequence 101+2i) carries its returns 2 and 3
// in block 0, channels 1 and 2, and the other twelve returns of the sample in
// block 1, channels 1 to 12. The capture holds every packet in that order,
// except that packet A of sample 8 is missing and a decoy carrying packet B
// of sample 8's sequence follows it. The capture's first packet is 1 ms
// after sample 0, and the container's window starts 50 ms after that, so
// sample 0's frame is warm-up.
//
// The held-out split scores obj_car_a (the pedestrian is only proposed). Its
// returns 0 to 3 at each sample, in the default scenario:
//
//	sample 0  warm-up frame: not measured
//	sample 1  0, 1 in cluster 1; 2 in cluster 2 (two fragments); 3 not in the
//	          domain, though a decoy return with its coordinates and another
//	          identity is in cluster 1: l3_background, and the gap
//	sample 2  height band, surface below floor, surface above ceiling, voxel;
//	          nothing in the frame is clustered, so it is not comparable
//	sample 3  input cap, DBSCAN noise, cluster shape, unknown reason
//	sample 4  unsettled frame: not measured
//	sample 5  car a fully occluded: not scored
//	sample 6  no frame; a gap record before sample 7's: not measured
//	sample 7  0 in cluster 1; 1 only in sample 8's domain (other_frame);
//	          2 twice in the domain (unresolved); 3 in cluster 1 with its x
//	          moved 1 cm (matched by identity, coordinates disagree)
//	sample 8  0, 1 in a packet the capture lacks; 2, 3 in a packet whose
//	          sequence the decoy shares: four unresolved, though all four are
//	          clustered in the domain
//	sample 9  failed frame: not measured
const (
	FatePort          = 2369
	FateWindowStartNs = int64(50_000_000)
	// FateFrameOffsetNs is how far each frame's time is from its sample's:
	// within the default frame tolerance.
	FateFrameOffsetNs = int64(-200_000)
)

// FateFixture is where the fate fixture was written.
type FateFixture struct {
	Fixture
	CapturePath     string
	ObservationsDir string
}

// FatePacketSequence is the sequence number of sample i's packet j (0 is A,
// 1 is B).
func FatePacketSequence(i, j int) uint32 { return uint32(100 + 2*i + j) }

// fatePayload is sample i's packet j as a 1266-byte payload. A decoy shares
// its sequence and differs in content.
func fatePayload(i, j int, decoy bool) []byte {
	p := make([]byte, parse.PACKET_SIZE_SEQUENCE)
	for k := 0; k < len(p)-parse.SEQUENCE_SIZE; k++ {
		p[k] = byte(i*31 + j*7 + k)
	}
	if decoy {
		p[0] ^= 0xff
	}
	binary.LittleEndian.PutUint32(p[len(p)-parse.SEQUENCE_SIZE:], FatePacketSequence(i, j))
	return p
}

// fateReturns lays the scene's points out over two real packets per sample.
func fateReturns(samples []annotation.Sample) annotation.PackReturns {
	layout := annotation.ReturnLayout{Model: "hesai-pandar40p", Blocks: 10, Channels: 40}
	per := layout.PerPacket()
	r := annotation.PackReturns{Layout: layout, Returns: make([][]uint64, len(samples))}
	for i := range samples {
		a, b := uint64(len(r.Packets)), uint64(len(r.Packets)+1)
		r.Packets = append(r.Packets, parse.PacketKey(fatePayload(i, 0, false)), parse.PacketKey(fatePayload(i, 1, false)))
		returns := []uint64{a * per, a*per + 1, b * per, b*per + 1}
		for k := 0; k < samples[i].PointCount-4; k++ {
			returns = append(returns, b*per+uint64(layout.Channels)+uint64(k))
		}
		r.Returns[i] = returns
		samples[i].ReturnStart, samples[i].ReturnEnd = a*per, (b+1)*per
	}
	return r
}

// FateReturn is one return of a scenario frame's domain.
type FateReturn struct {
	Seq            uint32
	Block, Channel uint16
	X, Y, Z        float64
	// Cluster is the accepted cluster holding the return, or zero; Reason
	// says why an unclustered return is in none.
	Cluster int64
	Reason  l4bobserve.RejectionReason
}

// FateFrame is one scenario frame: its sample, outcome and domain.
type FateFrame struct {
	Sample      int
	Disposition l4bobserve.DispositionKind
	Domain      []FateReturn
	// GapBefore records a bounded gap between the previous frame and this one.
	GapBefore bool
}

// FateScenario is the container's frames, in order.
type FateScenario struct{ Frames []FateFrame }

// packReturn is sample i's pack point k as a domain return, clustered in
// cluster c: its packet identity and its coordinates exactly as the pack
// stores them.
func packReturn(pts annotation.Points, i, k int, c int64) FateReturn {
	r := FateReturn{X: float64(pts.X[k]), Y: float64(pts.Y[k]), Z: float64(pts.Z[k]), Cluster: c}
	switch {
	case k < 2:
		r.Seq, r.Block, r.Channel = FatePacketSequence(i, 0), 0, uint16(k+1)
	case k < 4:
		r.Seq, r.Block, r.Channel = FatePacketSequence(i, 1), 0, uint16(k-1)
	default:
		r.Seq, r.Block, r.Channel = FatePacketSequence(i, 1), 1, uint16(k-3)
	}
	return r
}

// DefaultFateScenario is the scenario the package comment describes.
func DefaultFateScenario(packDir string) (FateScenario, error) {
	pack, err := annotation.OpenPack(packDir)
	if err != nil {
		return FateScenario{}, err
	}
	var s FateScenario
	for i := 0; i < Samples; i++ {
		if i == 6 {
			continue
		}
		pts, err := pack.PointsAt(i)
		if err != nil {
			return FateScenario{}, err
		}
		f := FateFrame{Sample: i, Disposition: l4bobserve.DispositionObserved, GapBefore: i == 7}
		// Car a's four returns first, then the other objects' twelve, in
		// cluster 9 unless a case says otherwise.
		car := make([]FateReturn, 4)
		for k := range car {
			car[k] = packReturn(pts, i, k, 1)
		}
		others := make([]FateReturn, 0, 12)
		for k := 4; k < len(pts.X); k++ {
			others = append(others, packReturn(pts, i, k, 9))
		}
		switch i {
		case 1:
			car[2].Cluster = 2
			decoy := car[3]
			decoy.Block, decoy.Channel = 2, 1
			car[3] = decoy // the real return 3 is absent; its coordinates are not
		case 2:
			for k, reason := range []l4bobserve.RejectionReason{l4bobserve.RejectionHeightBand, l4bobserve.RejectionSurfaceBelowFloor,
				l4bobserve.RejectionSurfaceAboveCeiling, l4bobserve.RejectionVoxelReduced} {
				car[k].Cluster, car[k].Reason = 0, reason
			}
			for k := range others {
				others[k].Cluster, others[k].Reason = 0, l4bobserve.RejectionClusterNoise
			}
		case 3:
			for k, reason := range []l4bobserve.RejectionReason{l4bobserve.RejectionClusterInputCap, l4bobserve.RejectionClusterNoise,
				l4bobserve.RejectionClusterShape, l4bobserve.RejectionUnknown} {
				car[k].Cluster, car[k].Reason = 0, reason
			}
		case 4:
			f.Disposition = l4bobserve.DispositionUnsettled
		case 7:
			twin := car[2]
			twin.Cluster, twin.Reason = 0, l4bobserve.RejectionClusterNoise
			car[3].X += 0.01
			car = []FateReturn{car[0], car[2], twin, car[3]} // return 1 is absent here
		case 8:
			prevPts, err := pack.PointsAt(7)
			if err != nil {
				return FateScenario{}, err
			}
			others = append(others, packReturn(prevPts, 7, 1, 9)) // sample 7's return 1
		case 9:
			f.Disposition = l4bobserve.DispositionFailed
		}
		f.Domain = append(car, others...)
		s.Frames = append(s.Frames, f)
	}
	return s, nil
}

// WriteFate writes the pack, split manifest, capture and default container
// under dir.
func WriteFate(dir string) (*FateFixture, error) {
	f := &FateFixture{
		Fixture:     Fixture{PackDir: filepath.Join(dir, "pack"), SplitManifestPath: filepath.Join(dir, "split.json")},
		CapturePath: filepath.Join(dir, "capture.pcapng"), ObservationsDir: filepath.Join(dir, "observations.vrlog"),
	}
	pack, err := writePackWith(f.PackDir, fateReturns)
	if err != nil {
		return nil, err
	}
	f.PackDigest, f.DatasetID = pack.Manifest.PackDigest, pack.Manifest.DatasetID
	if err := WriteSplitManifest(f.SplitManifestPath, f.Manifest()); err != nil {
		return nil, err
	}
	var payloads [][]byte
	for i := 0; i < Samples; i++ {
		if i != 8 {
			payloads = append(payloads, fatePayload(i, 0, false))
		}
		payloads = append(payloads, fatePayload(i, 1, false))
		if i == 8 {
			payloads = append(payloads, fatePayload(i, 1, true))
		}
	}
	if err := WriteFateCapture(f.CapturePath, payloads); err != nil {
		return nil, err
	}
	s, err := DefaultFateScenario(f.PackDir)
	if err != nil {
		return nil, err
	}
	if err := WriteFateContainer(f.ObservationsDir, f.CapturePath, s); err != nil {
		return nil, err
	}
	return f, nil
}

// WriteFateCapture writes payloads as UDP datagrams to FatePort in a PCAPNG
// file, the first 1 ms after sample 0 and the rest 1 µs apart.
func WriteFateCapture(path string, payloads [][]byte) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	w, err := pcapgo.NewNgWriter(file, layers.LinkTypeEthernet)
	if err != nil {
		return err
	}
	start := time.Unix(0, SampleTime(0)+1_000_000)
	for k, p := range payloads {
		eth := &layers.Ethernet{SrcMAC: net.HardwareAddr{2, 0, 0, 0, 0, 1}, DstMAC: net.HardwareAddr{2, 0, 0, 0, 0, 2}, EthernetType: layers.EthernetTypeIPv4}
		ip := &layers.IPv4{Version: 4, TTL: 64, Protocol: layers.IPProtocolUDP, SrcIP: net.IP{192, 168, 1, 201}, DstIP: net.IP{192, 168, 1, 100}}
		udp := &layers.UDP{SrcPort: 10000, DstPort: layers.UDPPort(FatePort)}
		if err := udp.SetNetworkLayerForChecksum(ip); err != nil {
			return err
		}
		buf := gopacket.NewSerializeBuffer()
		if err := gopacket.SerializeLayers(buf, gopacket.SerializeOptions{FixLengths: true, ComputeChecksums: true}, eth, ip, udp, gopacket.Payload(p)); err != nil {
			return err
		}
		d := buf.Bytes()
		ci := gopacket.CaptureInfo{Timestamp: start.Add(time.Duration(k) * time.Microsecond), CaptureLength: len(d), Length: len(d)}
		if err := w.WritePacket(ci, d); err != nil {
			return err
		}
	}
	return w.Flush()
}

// WriteFateContainer writes the scenario as a closed foreground-complete
// container whose manifest names capturePath, by its digest, as its source,
// and a window starting FateWindowStartNs after its first packet. Frames are
// numbered in order.
func WriteFateContainer(dir, capturePath string, s FateScenario) error {
	raw, err := os.ReadFile(capturePath)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	files := []vrlog.CaptureFile{{Path: capturePath, SHA256: hex.EncodeToString(sum[:])}}
	calibration := l4bobserve.Calibration{SensorID: "fixture", FromFrame: "sensor", ToFrame: "site",
		Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}}
	calibrationID, err := l4bobserve.CalibrationID(calibration)
	if err != nil {
		return err
	}
	const extractor = "l4.fixture/v1"
	sourceID, err := l4bobserve.SourceID(l4bobserve.CaptureSource{ReplayCaseID: "fate-fixture",
		CapturePaths: []string{files[0].Path}, CaptureSHA256s: []string{files[0].SHA256}, ExtractorID: extractor})
	if err != nil {
		return err
	}
	w, err := vrlog.Create(dir, vrlog.Manifest{
		Capture: vrlog.CaptureIdentity{SensorID: "fixture", SourceType: "pcap"},
		Extraction: vrlog.ExtractionIdentity{SourceID: sourceID, CalibrationID: calibrationID, CoordinateFrame: "site/fixture",
			ReplayCaseID: "fate-fixture", CaptureFiles: files, ExtractorID: extractor,
			Window: vrlog.SourceWindow{StartOffsetNanos: FateWindowStartNs, WarmupNanos: FateWindowStartNs}},
		Calibration: calibration,
		Provenance:  vrlog.Provenance{Writer: "evalfixture", BuildVersion: "fixture", BuildGitSHA: "fixture", ParamsHash: "sha256:fixture"},
		Commit:      vrlog.CommitPolicy{CommitDeadline: time.Minute},
	})
	if err != nil {
		return err
	}
	for n, frame := range s.Frames {
		rec, err := fateRecord(uint64(n), frame)
		if err != nil {
			_ = w.Abandon()
			return err
		}
		if frame.GapBefore {
			t := SampleTime(frame.Sample-1) + FateFrameOffsetNs + 1
			if err := w.AppendGap(l4bobserve.GapRecord{Cause: "fixture: rotation lost", MissingFrames: l4bobserve.KnownCount(1),
				Time: l4bobserve.GapTimeBounded, StartUnixNanos: t, EndUnixNanos: t + SamplePeriodNs/2}); err != nil {
				_ = w.Abandon()
				return err
			}
		}
		if err := w.AppendFrame(rec); err != nil {
			_ = w.Abandon()
			return fmt.Errorf("frame %d (sample %d): %w", n, frame.Sample, err)
		}
	}
	_, err = w.Close()
	return err
}

// fateRecord builds the scenario frame's record: a valid foreground-complete
// frame whose domain is the frame's returns in order.
func fateRecord(seq uint64, f FateFrame) (l4bobserve.FrameRecord, error) {
	start := SampleTime(f.Sample) + FateFrameOffsetNs
	n := len(f.Domain)
	rec := l4bobserve.FrameRecord{
		Sequence: seq, SensorFrameID: fmt.Sprintf("fixture-%d", f.Sample),
		FrameUnixNanos: start, CaptureStartUnixNanos: start, CaptureEndUnixNanos: start + SamplePeriodNs - 1_000_000,
		Completeness: l4bobserve.Completeness{State: l4bobserve.CompletenessComplete, LostPackets: l4bobserve.KnownCount(0)},
		Background:   l4bobserve.BackgroundSettled,
		Disposition:  l4bobserve.Disposition{Kind: f.Disposition},
		Stages: []l4bobserve.StageCount{
			{Stage: l4bobserve.StageL2Frame, Output: l4bobserve.KnownCount(n + 5)},
			{Stage: l4bobserve.StageL3Foreground, Input: l4bobserve.KnownCount(n + 5), Output: l4bobserve.KnownCount(n), Rejected: l4bobserve.KnownCount(5)},
		},
	}
	switch f.Disposition {
	case l4bobserve.DispositionFailed:
		rec.Disposition.Stage, rec.Disposition.Reason = l4bobserve.StageL3Foreground, "fixture: foreground mask unavailable"
		rec.Stages = rec.Stages[:1]
		return rec, nil
	case l4bobserve.DispositionUnsettled:
		rec.Background = l4bobserve.BackgroundSettling
	}
	p := l4bobserve.RetainedPoints{Fields: l4bobserve.FieldAcquisitionTime | l4bobserve.FieldIntensity | l4bobserve.FieldChannel |
		l4bobserve.FieldSourceOrdinal | l4bobserve.FieldPacketSequence | l4bobserve.FieldBlockIndex}
	members := map[int64][]uint32{}
	for k, r := range f.Domain {
		p.X, p.Y, p.Z = append(p.X, r.X), append(p.Y, r.Y), append(p.Z, r.Z)
		p.TimeOffsetNanos = append(p.TimeOffsetNanos, int64(k))
		p.Intensity = append(p.Intensity, uint8(k))
		p.Channel = append(p.Channel, r.Channel)
		p.SourceOrdinal = append(p.SourceOrdinal, uint32(2*k+1))
		p.PacketSequence = append(p.PacketSequence, r.Seq)
		p.BlockIndex = append(p.BlockIndex, r.Block)
		if r.Cluster > 0 {
			members[r.Cluster] = append(members[r.Cluster], uint32(k))
		} else {
			rec.Unassigned = append(rec.Unassigned, uint32(k))
			rec.UnassignedReasons = append(rec.UnassignedReasons, r.Reason)
		}
	}
	rec.Points = p
	if f.Disposition == l4bobserve.DispositionSuppressed {
		rec.Disposition.Stage, rec.Disposition.Reason = l4bobserve.StageL4Transform, "fixture: pipeline stops at L3"
		rec.Payload = l4bobserve.PayloadPoints
		rec.Unassigned, rec.UnassignedReasons = nil, nil
		return rec, nil
	}
	rec.Payload = l4bobserve.PayloadPoints | l4bobserve.PayloadMembership
	ids := make([]int64, 0, len(members))
	for id := range members {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	clustered := 0
	for _, id := range ids {
		m := members[id]
		k := m[0]
		rec.Clusters = append(rec.Clusters, l4bobserve.ClusterRecord{ClusterID: id, Members: m, Summary: l4bobserve.ClusterSummary{
			FirstMemberUnixNanos: start, CentroidX: float32(p.X[k]), CentroidY: float32(p.Y[k]), CentroidZ: float32(p.Z[k]),
			PointsCount: len(m)}})
		clustered += len(m)
	}
	rec.Stages = append(rec.Stages, l4bobserve.StageCount{Stage: l4bobserve.StageL4Cluster, Input: l4bobserve.KnownCount(n),
		Output: l4bobserve.KnownCount(clustered), Rejected: l4bobserve.KnownCount(n - clustered)})
	return rec, rec.ValidateFor(l4bobserve.ForegroundComplete())
}
