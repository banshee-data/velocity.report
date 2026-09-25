//go:build pcap

package replayeval

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/l4perception"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// kirk0Evidence is the observer-test window: a 20-second capture-time warm-up,
// then four scored seconds with a settled grid and real foreground clusters.
func kirk0Evidence(t testing.TB, pcapPath string) Config {
	return Config{
		PCAPFile: pcapPath, SensorID: "test-replay", UDPPort: 2369,
		StartSeconds: 20, WarmupSeconds: 20, DurationSeconds: 4, RequireSettled: true,
		ReplayCaseID: "kirk0-test-case", ObservationCalibration: identityReplayCalibration("test-replay"),
		ObservationMaxSamplePoints: 64,
	}
}

type runCost struct {
	elapsed time.Duration
	alloc   uint64
}

func measuredRun(t *testing.T, cfg Config) (*Result, runCost) {
	t.Helper()
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	result, err := Run(cfg)
	if err != nil {
		t.Fatalf("replay %s: %v", filepath.Base(cfg.OutDir), err)
	}
	cost := runCost{elapsed: time.Since(start)}
	runtime.ReadMemStats(&after)
	cost.alloc = after.TotalAlloc - before.TotalAlloc
	return result, cost
}

func evidenceOracle(t *testing.T, path, sourceID string) observationsqlite.EvidenceOracle {
	t.Helper()
	database, err := db.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	oracle, err := observationsqlite.BuildEvidenceOracle(database.DB, map[string]struct{}{sourceID: {}})
	if err != nil {
		t.Fatal(err)
	}
	return oracle
}

// The foreground-complete tap is an observer of the existing pipeline. With it
// enabled, the schema-2 tracker baseline and the reduced SQLite evidence stay
// byte-identical; every processed frame has a record in sequence order; each
// record's membership partitions its retained domain exactly; and each cluster
// summary is the one the legacy observation stored for the same frame, with
// the legacy capped sample drawn from that cluster's members.
func TestObservationFramesDescribeEveryKirk0Frame(t *testing.T) {
	pcapPath := requireKirk0(t)
	dir := t.TempDir()

	without := kirk0Evidence(t, pcapPath)
	without.OutDir = filepath.Join(dir, "without")
	without.ObservationDBPath = filepath.Join(dir, "without.db")
	plainResult, plainCost := measuredRun(t, without)

	var records []l4bobserve.FrameRecord
	with := kirk0Evidence(t, pcapPath)
	with.OutDir = filepath.Join(dir, "with")
	with.ObservationDBPath = filepath.Join(dir, "with.db")
	with.ObservationFrames = func(record l4bobserve.FrameRecord) error {
		records = append(records, record)
		return nil
	}
	result, tapCost := measuredRun(t, with)

	plain, err := os.ReadFile(filepath.Join(without.OutDir, "tracking_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	tapped, err := os.ReadFile(filepath.Join(with.OutDir, "tracking_baseline.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(plain, tapped) {
		t.Fatalf("the observation frame tap changed the tracker baseline:\n%s\n%s", plain, tapped)
	}
	if result.ObservationSourceID != plainResult.ObservationSourceID {
		t.Fatalf("source identity moved: %s vs %s", plainResult.ObservationSourceID, result.ObservationSourceID)
	}
	plainOracle := evidenceOracle(t, without.ObservationDBPath, plainResult.ObservationSourceID)
	tappedOracle := evidenceOracle(t, with.ObservationDBPath, result.ObservationSourceID)
	if !reflect.DeepEqual(plainOracle, tappedOracle) {
		t.Fatalf("the tap changed the reduced SQLite evidence:\n%+v\n%+v", plainOracle.Tables, tappedOracle.Tables)
	}

	if len(records) == 0 || len(records) != result.FramesRead || result.ObservationFrames != len(records) {
		t.Fatalf("records = %d, result count = %d, frames read = %d", len(records), result.ObservationFrames, result.FramesRead)
	}
	stream := l4bobserve.NewStreamValidator(l4bobserve.ForegroundComplete())
	for i, record := range records {
		if record.Sequence != uint64(i) {
			t.Fatalf("record %d carries sequence %d", i, record.Sequence)
		}
		if err := stream.AddFrame(record); err != nil {
			t.Fatal(err)
		}
	}

	database, err := db.NewDB(with.ObservationDBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	legacy, err := observationsqlite.NewObservationStore(database).ListBySource(result.ObservationSourceID)
	if err != nil {
		t.Fatal(err)
	}
	byFrame := map[int64]map[int64]l4bobserve.Record{}
	for _, observation := range legacy {
		r := observation.Snapshot()
		if byFrame[r.FrameUnixNanos] == nil {
			byFrame[r.FrameUnixNanos] = map[int64]l4bobserve.Record{}
		}
		byFrame[r.FrameUnixNanos][r.Cluster.ClusterID] = r
	}

	stats := struct {
		dispositions, completeness map[string]int
		reasons                    map[l4bobserve.RejectionReason]int
		points, maxPoints          int
		members, clusters, sample  int
		bytes                      uint64
	}{dispositions: map[string]int{}, completeness: map[string]int{}, reasons: map[l4bobserve.RejectionReason]int{}}
	seenFrames := map[int64]bool{}
	for _, record := range records {
		if seenFrames[record.FrameUnixNanos] {
			t.Fatalf("frame time %d repeats; legacy identities would collide", record.FrameUnixNanos)
		}
		seenFrames[record.FrameUnixNanos] = true
		stats.dispositions[fmt.Sprint(record.Disposition.Kind)]++
		stats.completeness[fmt.Sprint(record.Completeness.State)]++
		n := record.Points.Len()
		stats.points += n
		stats.maxPoints = max(stats.maxPoints, n)
		stats.bytes += recordBytes(record)
		for _, reason := range record.UnassignedReasons {
			stats.reasons[reason]++
		}
		if !record.Payload.Has(l4bobserve.PayloadMembership) {
			t.Fatalf("frame %d has no membership: %+v", record.Sequence, record.Disposition)
		}
		covered := len(record.Unassigned)
		for _, cluster := range record.Clusters {
			covered += len(cluster.Members)
		}
		if covered != n {
			t.Fatalf("frame %d partition covers %d of %d", record.Sequence, covered, n)
		}

		stored := byFrame[record.FrameUnixNanos]
		if len(stored) != len(record.Clusters) {
			t.Fatalf("frame %d: %d clusters, %d legacy observations", record.Sequence, len(record.Clusters), len(stored))
		}
		for _, cluster := range record.Clusters {
			r, ok := stored[cluster.ClusterID]
			if !ok {
				t.Fatalf("frame %d cluster %d has no legacy observation", record.Sequence, cluster.ClusterID)
			}
			if err := sameSummary(cluster.Summary, r.Cluster); err != nil {
				t.Fatalf("frame %d cluster %d: %v", record.Sequence, cluster.ClusterID, err)
			}
			if err := sampleDrawnFromMembers(record, cluster, r.Cluster.RetainedPoints); err != nil {
				t.Fatalf("frame %d cluster %d: %v", record.Sequence, cluster.ClusterID, err)
			}
			stats.clusters++
			stats.members += len(cluster.Members)
			stats.sample += len(r.Cluster.RetainedPoints)
		}
		delete(byFrame, record.FrameUnixNanos)
	}
	if len(byFrame) != 0 {
		t.Fatalf("%d legacy frames have no frame record", len(byFrame))
	}
	if stats.clusters == 0 {
		t.Fatal("the window produced no clusters to compare")
	}

	reasons := make([]string, 0, len(stats.reasons))
	for reason, count := range stats.reasons {
		reasons = append(reasons, fmt.Sprintf("%s=%d", reason, count))
	}
	sort.Strings(reasons)
	t.Logf("frames=%d dispositions=%v completeness=%v retained=%d (max %d/frame) clusters=%d members=%d legacy-sample=%d unassigned=[%s]",
		len(records), stats.dispositions, stats.completeness, stats.points, stats.maxPoints,
		stats.clusters, stats.members, stats.sample, strings.Join(reasons, " "))
	t.Logf("record payload %.1f MiB total, %.0f KiB/frame mean; replay without tap %s / %.0f MiB allocated, with tap %s / %.0f MiB (retaining every record)",
		float64(stats.bytes)/(1<<20), float64(stats.bytes)/1024/float64(len(records)),
		plainCost.elapsed.Round(time.Millisecond), float64(plainCost.alloc)/(1<<20),
		tapCost.elapsed.Round(time.Millisecond), float64(tapCost.alloc)/(1<<20))
}

func TestObservationFramesRequireIdentityAndFailOnRefusal(t *testing.T) {
	pcapPath := requireKirk0(t)
	cfg := kirk0Evidence(t, pcapPath)
	cfg.OutDir = filepath.Join(t.TempDir(), "no-identity")
	cfg.ReplayCaseID = ""
	cfg.ObservationFrames = func(l4bobserve.FrameRecord) error { return nil }
	if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "ReplayCaseID") {
		t.Fatalf("tap without a replay case = %v", err)
	}

	cfg = kirk0Evidence(t, pcapPath)
	cfg.OutDir = filepath.Join(t.TempDir(), "refused")
	cfg.StartSeconds, cfg.WarmupSeconds, cfg.DurationSeconds, cfg.RequireSettled = 0, 0, 1, false
	delivered := 0
	cfg.ObservationFrames = func(l4bobserve.FrameRecord) error {
		delivered++
		if delivered == 3 {
			return fmt.Errorf("caller out of memory")
		}
		return nil
	}
	if _, err := Run(cfg); err == nil || !strings.Contains(err.Error(), "caller out of memory") {
		t.Fatalf("a refused record did not fail the replay: %v", err)
	}
}

func sameSummary(s l4bobserve.ClusterSummary, c l4perception.WorldCluster) error {
	type view struct {
		TS                                  int64
		CX, CY, CZ, L, W, H, P95, Intensity float32
		Count                               int
		Clipped                             bool
		OBB                                 *l4bobserve.OrientedBox
	}
	legacy := view{c.TSUnixNanos, c.CentroidX, c.CentroidY, c.CentroidZ, c.BoundingBoxLength, c.BoundingBoxWidth,
		c.BoundingBoxHeight, c.HeightP95, c.IntensityMean, c.PointsCount, c.GroundClipped, nil}
	if c.OBB != nil {
		legacy.OBB = &l4bobserve.OrientedBox{CenterX: c.OBB.CenterX, CenterY: c.OBB.CenterY, CenterZ: c.OBB.CenterZ,
			Length: c.OBB.Length, Width: c.OBB.Width, Height: c.OBB.Height, HeadingRad: c.OBB.HeadingRad}
	}
	tapped := view{s.FirstMemberUnixNanos, s.CentroidX, s.CentroidY, s.CentroidZ, s.BoundingBoxLength, s.BoundingBoxWidth,
		s.BoundingBoxHeight, s.HeightP95, s.IntensityMean, s.PointsCount, s.GroundClipped, s.OBB}
	if !reflect.DeepEqual(legacy, tapped) {
		return fmt.Errorf("summary differs from the legacy observation:\n%+v\n%+v", tapped, legacy)
	}
	return nil
}

// sampleDrawnFromMembers checks, bit for bit, that each point of the legacy
// capped sample is a member of the cluster: same float64 XYZ, acquisition time
// and intensity.
func sampleDrawnFromMembers(record l4bobserve.FrameRecord, cluster l4bobserve.ClusterRecord, sample []l4perception.WorldPoint) error {
	type key struct {
		x, y, z   uint64
		t         int64
		intensity uint8
	}
	p := record.Points
	members := make(map[key]int, len(cluster.Members))
	for _, m := range cluster.Members {
		members[key{math.Float64bits(p.X[m]), math.Float64bits(p.Y[m]), math.Float64bits(p.Z[m]),
			record.FrameUnixNanos + p.TimeOffsetNanos[m], p.Intensity[m]}]++
	}
	for i, s := range sample {
		k := key{math.Float64bits(s.X), math.Float64bits(s.Y), math.Float64bits(s.Z), s.Timestamp.UnixNano(), s.Intensity}
		if members[k] == 0 {
			return fmt.Errorf("legacy sample point %d is not a member", i)
		}
		members[k]--
	}
	return nil
}

// recordBytes is the in-memory payload of a record's columns and membership,
// excluding slice headers and summaries: the number a durable writer's point
// budget starts from.
func recordBytes(r l4bobserve.FrameRecord) uint64 {
	p := r.Points
	n := uint64(len(p.X)*24 + len(p.TimeOffsetNanos)*8 + len(p.Intensity) + len(p.Channel)*2 + len(p.ReturnIndex) +
		len(p.SourceOrdinal)*4 + len(p.PacketSequence)*4 + len(p.BlockIndex)*2)
	for _, c := range r.Clusters {
		n += uint64(len(c.Members) * 4)
	}
	return n + uint64(len(r.Unassigned)*5)
}

// BenchmarkObservationFrameTapKirk0 measures the tap's cost on the whole
// kirk0 capture with a streaming sink that retains nothing, against the same
// replay without it. It is not part of the perf gate; run it explicitly:
//
//	go test -tags=pcap -run '^$' -bench ObservationFrameTapKirk0 -benchtime 1x -benchmem ./internal/lidar/replayeval/
func BenchmarkObservationFrameTapKirk0(b *testing.B) {
	p, err := filepath.Abs(kirk0)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := os.Stat(p); err != nil {
		b.Skipf("reference capture not available: %v", err)
	}
	sequence, err := PrepareCaptureSequence([]string{p}, 2369)
	if err != nil {
		b.Fatal(err)
	}
	digest, err := fileSHA256(p)
	if err != nil {
		b.Fatal(err)
	}
	for _, variant := range []string{"without-tap", "with-tap"} {
		b.Run(variant, func(b *testing.B) {
			var frames, points, bytes atomic.Uint64
			for i := 0; i < b.N; i++ {
				cfg := Config{PCAPFiles: []string{p}, PCAPSHA256s: []string{digest}, CaptureSequence: sequence,
					SensorID: "bench-replay", UDPPort: 2369, OutDir: filepath.Join(b.TempDir(), "run"),
					ReplayCaseID: "kirk0-bench", ObservationCalibration: identityReplayCalibration("bench-replay")}
				if variant == "with-tap" {
					cfg.ObservationFrames = func(record l4bobserve.FrameRecord) error {
						frames.Add(1)
						points.Add(uint64(record.Points.Len()))
						bytes.Add(recordBytes(record))
						return nil
					}
				}
				stop := make(chan struct{})
				peak := make(chan uint64)
				go samplePeakHeap(stop, peak)
				if _, err := Run(cfg); err != nil {
					b.Fatal(err)
				}
				close(stop)
				b.ReportMetric(float64(<-peak)/(1<<20), "peak-heap-MiB")
			}
			if n := frames.Load(); n > 0 {
				b.ReportMetric(float64(n)/float64(b.N), "frames/op")
				b.ReportMetric(float64(points.Load())/float64(n), "retained-pts/frame")
				b.ReportMetric(float64(bytes.Load())/float64(n)/1024, "record-KiB/frame")
			}
		})
	}
}

func samplePeakHeap(stop <-chan struct{}, peak chan<- uint64) {
	var stats runtime.MemStats
	var highest uint64
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			peak <- highest
			return
		case <-ticker.C:
			runtime.ReadMemStats(&stats)
			highest = max(highest, stats.HeapInuse)
		}
	}
}
