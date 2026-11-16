package motionexport

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

const blockSep = 55.56e-6 // Pandar40P single-return block separation, seconds

// rotation builds one 10 Hz rotation of 1800 blocks (0.2° apart) in 180
// packets of ten. Each block has `channels` returns. The recorded point time
// reproduces today's replay path: the packet's PCAP arrival time (true time
// of its first block plus jitter) plus the channel firetime, with no
// per-block offset. It also returns each point's true firing time.
func rotation(t0 time.Time, startAz float64, jitter float64, channels int, rng *rand.Rand, seq0 uint32) ([]l2frames.Point, []float64) {
	var pts []l2frames.Point
	var truth []float64
	fire := []float64{-1e-6, -3e-6, -5e-6, -7e-6}
	for pk := 0; pk < 180; pk++ {
		pktTrue := float64(pk) * 10 * blockSep
		arrival := pktTrue + jitter*(rng.Float64()*2-1)
		for b := 0; b < 10; b++ {
			blockTrue := pktTrue + float64(b)*blockSep
			az := math.Mod(startAz+float64(pk*10+b)*0.2, 360)
			for c := 0; c < channels; c++ {
				rec := arrival + fire[c%4]
				el := float64(c) - 2
				r := 10 + float64(c)
				x, y, z := l2frames.SphericalToCartesian(r, az, el)
				pts = append(pts, l2frames.Point{X: x, Y: y, Z: z, Distance: r, Azimuth: az, Elevation: el, Channel: c + 1,
					Intensity: uint8(c * 10), Timestamp: t0.Add(time.Duration(rec * 1e9)), BlockID: b,
					UDPSequence: seq0 + uint32(pk), RawBlockAzimuth: uint16(math.Round(az * 100)), PacketKey: uint64(seq0) + uint64(pk) + 1})
				truth = append(truth, blockTrue+fire[c%4])
			}
		}
	}
	return pts, truth
}

func centredRMS(a, b []float64) (rms, mean float64) {
	for i := range a {
		mean += a[i] - b[i]
	}
	mean /= float64(len(a))
	for i := range a {
		d := a[i] - b[i] - mean
		rms += d * d
	}
	return math.Sqrt(rms / float64(len(a))), mean
}

func TestAzimuthClockRemovesArrivalJitterAndBlockSawtooth(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	t0 := time.Date(2026, 9, 2, 18, 33, 0, 0, time.UTC)
	pts, truth := rotation(t0, 350, 300e-6, 4, rng, 0)

	clock, fit := PointTimes(pts, t0, true)
	if !fit.OK {
		t.Fatalf("fit not ok: %+v", fit)
	}
	if math.Abs(fit.PeriodS-0.1) > 1e-4 {
		t.Errorf("period %.6f s, want 0.1", fit.PeriodS)
	}
	if fit.SpanDeg < 359 {
		t.Errorf("azimuth did not unwrap across 0°: span %.1f", fit.SpanDeg)
	}
	rmsClock, offClock := centredRMS(clock, truth)
	packet, _ := PointTimes(pts, t0, false)
	rmsPacket, _ := centredRMS(packet, truth)
	if rmsClock > 20e-6 {
		t.Errorf("azimuth clock error %.1f µs RMS about its constant offset", rmsClock*1e6)
	}
	if rmsPacket < 5*rmsClock {
		t.Errorf("packet time %.1f µs RMS should be far worse than the clock's %.1f µs", rmsPacket*1e6, rmsClock*1e6)
	}
	// What remains is a constant: about half a packet's block span, because
	// the recorded time names the packet, not the block. A constant cancels
	// in frame-to-frame odometry; it is recorded, not hidden.
	if math.Abs(offClock+4.5*blockSep) > 30e-6 {
		t.Errorf("constant offset %.1f µs, want about %.1f", offClock*1e6, -4.5*blockSep*1e6)
	}
	if fit.RMSResidUS < 100 {
		t.Errorf("reported residual %.1f µs should show the recorded jitter and sawtooth", fit.RMSResidUS)
	}
}

func TestPointTimesRefusesAPartialRotation(t *testing.T) {
	rng := rand.New(rand.NewSource(1))
	t0 := time.Unix(0, 0)
	pts, _ := rotation(t0, 0, 0, 1, rng, 0)
	rec, fit := PointTimes(pts[:500], t0, true)
	if fit.OK || fit.Reason == "" {
		t.Fatalf("partial rotation accepted: %+v", fit)
	}
	if rec[10] != pts[10].Timestamp.Sub(t0).Seconds() {
		t.Error("failed fit must fall back to recorded times")
	}
}

func frame(t0 time.Time, i int, rng *rand.Rand) *l2frames.LiDARFrame {
	pts, _ := rotation(t0.Add(time.Duration(i)*100*time.Millisecond), float64(i), 100e-6, 4, rng, uint32(i*180))
	return &l2frames.LiDARFrame{StartTimestamp: pts[0].Timestamp, Points: pts, SpinComplete: true, PointCount: len(pts)}
}

func TestExporterWritesLASFilesAndManifest(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	t0 := time.Date(2026, 9, 2, 18, 33, 0, 0, time.UTC)
	rng := rand.New(rand.NewSource(3))
	e, err := NewExporter(Config{OutDir: dir, SplitSeconds: 0.15, Software: "test"},
		Manifest{Segment: Segment{StartSite: "lombard-laguna", EndSite: "columbus-north-point"}})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 3; i++ {
		if err := e.AddFrame(frame(t0, i, rng)); err != nil {
			t.Fatal(err)
		}
	}
	inc := frame(t0, 3, rng)
	inc.SpinComplete = false
	_ = e.AddFrame(inc)
	man, err := e.Close()
	if err != nil {
		t.Fatal(err)
	}
	if man.Frames.Exported != 3 || man.Frames.Incomplete != 1 {
		t.Errorf("frames %+v", man.Frames)
	}
	if len(man.Files) != 2 {
		t.Fatalf("files %d, want 2 (split after 0.15 s)", len(man.Files))
	}
	if man.Time.ClockFitFailed != 0 || math.Abs(man.Time.MedianPeriodS-0.1) > 1e-4 {
		t.Errorf("time summary %+v", man.Time)
	}
	var total uint64
	for _, pf := range man.Files {
		f, err := os.Open(filepath.Join(dir, pf.Path))
		if err != nil {
			t.Fatal(err)
		}
		info, err := ReadLASInfo(f)
		f.Close()
		if err != nil {
			t.Fatal(err)
		}
		if info.Major != 1 || info.Minor != 4 || info.PointFormat != 6 || info.RecordLength != 39 || info.HeaderSize != 375 || info.NumberOfVLRs != 1 {
			t.Errorf("%s header %+v", pf.Path, info)
		}
		if info.Points != pf.Points {
			t.Errorf("%s: header %d points, manifest %d", pf.Path, info.Points, pf.Points)
		}
		st, _ := os.Stat(filepath.Join(dir, pf.Path))
		if want := int64(info.PointOffset) + int64(info.Points)*39; st.Size() != want || pf.Bytes != want {
			t.Errorf("%s: size %d, manifest %d, want %d", pf.Path, st.Size(), pf.Bytes, want)
		}
		if got := info.ExtraNames; len(got) != 3 || got[0] != "ring" || got[1] != "azimuth_deg" || got[2] != "frame" {
			t.Errorf("extra bytes %v", got)
		}
		total += info.Points
		if pf.Path == "segment-0000.las" {
			if info.FirstRing != 1 || info.FirstFrame != 0 {
				t.Errorf("first point ring %d frame %d", info.FirstRing, info.FirstFrame)
			}
			x, _, _ := l2frames.SphericalToCartesian(10, 0, -2)
			if math.Abs(info.FirstPointXYZ[0]-x) > 0.0005 {
				t.Errorf("first x %.4f want %.4f", info.FirstPointXYZ[0], x)
			}
		}
	}
	if total != 3*1800*4 {
		t.Errorf("points %d, want %d", total, 3*1800*4)
	}
	b, err := os.ReadFile(filepath.Join(dir, "run.json"))
	if err != nil {
		t.Fatal(err)
	}
	var back Manifest
	if err := json.Unmarshal(b, &back); err != nil || back.Schema != ManifestSchema || back.Segment.EndSite != "columbus-north-point" {
		t.Errorf("run.json %v %+v", err, back.Segment)
	}
	if _, err := os.Stat(filepath.Join(dir, "frames.csv")); err != nil {
		t.Error(err)
	}
	if _, err := NewExporter(Config{OutDir: dir}, Manifest{}); err == nil {
		t.Error("exporter accepted a non-empty output directory")
	}
}

func TestExporterDecimatesByAzimuthAndRange(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	rng := rand.New(rand.NewSource(4))
	e, err := NewExporter(Config{OutDir: t.TempDir(), AzimuthStride: 3, MaxRangeM: 12.5}, Manifest{})
	if err != nil {
		t.Fatal(err)
	}
	if err := e.AddFrame(frame(t0, 0, rng)); err != nil {
		t.Fatal(err)
	}
	man, err := e.Close()
	if err != nil {
		t.Fatal(err)
	}
	// Ranges are 10-13 m: 3 of 4 channels pass 12.5 m; every third block.
	if got, want := man.Files[0].Points, uint64(600*3); got != want {
		t.Errorf("points %d, want %d", got, want)
	}
}
