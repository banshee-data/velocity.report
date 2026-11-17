package motionexport

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
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
	return rotationAt(t0, startAz, 0.2, jitter, channels, rng, seq0)
}

// rotationAt builds one rotation at the azimuth step of the given rate: 0.2°
// per block at 10 Hz (1800 blocks), 0.4° at 20 Hz (900 blocks). The block
// separation is the same at both rates; the rotor turns twice as fast.
func rotationAt(t0 time.Time, startAz, azStep float64, jitter float64, channels int, rng *rand.Rand, seq0 uint32) ([]l2frames.Point, []float64) {
	var pts []l2frames.Point
	var truth []float64
	fire := []float64{-1e-6, -3e-6, -5e-6, -7e-6}
	packets := int(math.Round(360 / azStep / 10))
	for pk := 0; pk < packets; pk++ {
		pktTrue := float64(pk) * 10 * blockSep
		arrival := pktTrue + jitter*(rng.Float64()*2-1)
		for b := 0; b < 10; b++ {
			blockTrue := pktTrue + float64(b)*blockSep
			az := math.Mod(startAz+float64(pk*10+b)*azStep, 360)
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

// On a real capture each block keeps only the channels that returned, so the
// mean point time of a packet's blocks differs by a few microseconds of
// channel firetime. Ordering blocks by that mean shuffled them within a
// packet, and where a packet straddled the 0/360° seam the unwrap put some
// blocks a whole turn off: their points were timed 1-3 rotations early. On the
// September archive that was 0.7% of points in 89% of rotations.
func TestAzimuthClockUnwrapsTheSeamWhenChannelsDropOut(t *testing.T) {
	t0 := time.Date(2026, 9, 2, 18, 33, 0, 0, time.UTC)
	for r := 0; r < 20; r++ {
		rng := rand.New(rand.NewSource(int64(100 + r)))
		// Start azimuths off the 2° packet grid, so one packet straddles 0°.
		all, allTruth := rotation(t0, 350.1+0.1*float64(r%19), 0, 4, rng, uint32(r*1000))
		var pts []l2frames.Point
		var truth []float64
		for i := 0; i < len(all); i += 4 {
			keep := 1 + rng.Intn(15) // a non-empty subset of the four channels
			for c := 0; c < 4; c++ {
				if keep&(1<<c) != 0 {
					pts = append(pts, all[i+c])
					truth = append(truth, allTruth[i+c])
				}
			}
		}
		clock, fit := PointTimes(pts, t0, true)
		if !fit.OK || fit.Rejected != 0 {
			t.Fatalf("rotation %d: fit %+v", r, fit)
		}
		if fit.SpanDeg > 360 || fit.SpanDeg < 359 {
			t.Errorf("rotation %d: unwrapped span %.1f°, want just under 360°", r, fit.SpanDeg)
		}
		// Recorded times carry the packet sawtooth (up to ~250 µs about the
		// line); a block a turn off would be 100 ms.
		if fit.MaxResidUS > 1000 {
			t.Errorf("rotation %d: max residual %.0f µs, want no block a turn off", r, fit.MaxResidUS)
		}
		_, off := centredRMS(clock, truth)
		worst := 0.0
		for i := range clock {
			worst = math.Max(worst, math.Abs(clock[i]-truth[i]-off))
		}
		if worst > 50e-6 {
			t.Errorf("rotation %d: a point is %.1f ms from its firing time", r, worst*1e3)
		}
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

func TestAzimuthClockAt20Hz(t *testing.T) {
	rng := rand.New(rand.NewSource(9))
	t0 := time.Date(2026, 9, 2, 18, 33, 0, 0, time.UTC)
	pts, truth := rotationAt(t0, 120, 0.4, 300e-6, 4, rng, 0)
	clock, fit := PointTimes(pts, t0, true)
	if !fit.OK || math.Abs(fit.PeriodS-0.05) > 1e-4 {
		t.Fatalf("20 Hz fit %+v", fit)
	}
	if rms, _ := centredRMS(clock, truth); rms > 25e-6 {
		t.Errorf("20 Hz clock error %.1f µs RMS with half the blocks per rotation", rms*1e6)
	}
}

func TestExporterKeepsOneRotationInN(t *testing.T) {
	t0 := time.Unix(1_700_000_000, 0)
	rng := rand.New(rand.NewSource(5))
	dir := t.TempDir()
	e, err := NewExporter(Config{OutDir: dir, KeepEveryRotation: 2}, Manifest{})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 5; i++ {
		if err := e.AddFrame(frame(t0, i, rng)); err != nil {
			t.Fatal(err)
		}
	}
	man, err := e.Close()
	if err != nil {
		t.Fatal(err)
	}
	if man.Frames.Exported != 3 || man.Frames.Decimated != 2 || man.Selection.KeepEveryRotation != 2 {
		t.Errorf("frames %+v selection %+v", man.Frames, man.Selection)
	}
	// The measured rate is the source's: each kept rotation is still 100 ms.
	if math.Abs(man.Time.RotationHz-10) > 0.01 {
		t.Errorf("rotation_hz %.3f, want the source's 10", man.Time.RotationHz)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "frames.csv"))
	if !strings.Contains(string(b), "\n2,4,") {
		t.Errorf("third exported frame should be source rotation 4:\n%s", b)
	}
}

// delayPackets adds d to the recorded time of every point in the given
// packets, as a late-arriving packet would carry.
func delayPackets(pts []l2frames.Point, packets map[uint32]bool, d time.Duration) {
	for i := range pts {
		if packets[pts[i].UDPSequence] {
			pts[i].Timestamp = pts[i].Timestamp.Add(d)
		}
	}
}

func TestClockFitReportsLatePacketsAndKeepsThemOutOfItsResidual(t *testing.T) {
	rng := rand.New(rand.NewSource(21))
	t0 := time.Date(2026, 9, 2, 18, 33, 0, 0, time.UTC)
	pts, truth := rotation(t0, 10, 20e-6, 4, rng, 0)
	// Three of 180 packets arrive 5 ms late: 30 of 1800 blocks.
	delayPackets(pts, map[uint32]bool{40: true, 90: true, 140: true}, 5*time.Millisecond)
	clock, fit := PointTimes(pts, t0, true)
	if !fit.OK {
		t.Fatalf("a few late packets should not refuse the fit: %+v", fit)
	}
	if fit.Rejected != 30 {
		t.Errorf("rejected %d blocks, want the 30 late ones", fit.Rejected)
	}
	if fit.MaxRejectedResidUS < 4000 {
		t.Errorf("max rejected residual %.0f µs should show the 5 ms delay", fit.MaxRejectedResidUS)
	}
	if fit.MaxResidUS > 1000 || fit.RMSResidUS > 400 {
		t.Errorf("kept-block residual (rms %.0f, max %.0f µs) must exclude the rejected blocks", fit.RMSResidUS, fit.MaxResidUS)
	}
	// The line is right for a late packet: its points take their firing time.
	if rms, _ := centredRMS(clock, truth); rms > 20e-6 {
		t.Errorf("clock error %.1f µs RMS with late packets", rms*1e6)
	}
}

func TestClockFitRefusesWhenTooManyBlocksAreRejected(t *testing.T) {
	rng := rand.New(rand.NewSource(22))
	t0 := time.Date(2026, 9, 2, 18, 33, 0, 0, time.UTC)
	pts, _ := rotation(t0, 10, 20e-6, 4, rng, 0)
	late := map[uint32]bool{}
	for pk := uint32(0); pk < 180; pk += 7 { // 26 packets, 260 of 1800 blocks: 14%
		late[pk] = true
	}
	delayPackets(pts, late, 5*time.Millisecond)
	rec, fit := PointTimes(pts, t0, true)
	if fit.OK || !strings.Contains(fit.Reason, "rejected") {
		t.Fatalf("fit with %d of %d blocks rejected should be refused: %+v", fit.Rejected, fit.Blocks, fit)
	}
	if rec[0] != pts[0].Timestamp.Sub(t0).Seconds() {
		t.Error("a refused fit must fall back to recorded times")
	}
}

func TestLASHeaderBoundsContainTheStoredCoordinates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "b.las")
	w, err := CreateLAS(path, "test", time.Unix(0, 0))
	if err != nil {
		t.Fatal(err)
	}
	// 0.00026 m rounds up to one grid step (0.0005 m); -0.00076 rounds down to
	// two (-0.001 m). Bounds taken from the raw values would exclude both.
	for _, p := range []LASPoint{{X: 0.00026, Y: -0.00076, Z: 1.23426, Ring: 1}, {X: -0.00076, Y: 0.00026, Z: 1.23376, Ring: 2}} {
		if err := w.Write(p); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := ReadLASInfo(f)
	if err != nil {
		t.Fatal(err)
	}
	for c, want := range [][2]float64{{-0.001, 0.0005}, {-0.001, 0.0005}, {1.234, 1.2345}} {
		if math.Abs(info.Min[c]-want[0]) > 1e-12 || math.Abs(info.Max[c]-want[1]) > 1e-12 {
			t.Errorf("axis %d bounds [%.6f, %.6f], want the stored [%.6f, %.6f]", c, info.Min[c], info.Max[c], want[0], want[1])
		}
	}
	if info.FirstPointXYZ[0] > info.Max[0] || info.FirstPointXYZ[0] < info.Min[0] {
		t.Errorf("stored x %.6f outside header bounds", info.FirstPointXYZ[0])
	}
}
