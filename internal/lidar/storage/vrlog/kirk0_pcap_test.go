//go:build pcap

package vrlog_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l4bobserve"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
)

const kirk0 = "../../perf/pcap/kirk0.pcapng"

func requireKirk0(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(kirk0)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(p); err != nil || info.Size() < 1<<20 {
		t.Skipf("reference capture not available (Git LFS): %v", err)
	}
	return p
}

// kirk0Replay processes [0, start+duration) seconds of kirk0: warmup equals
// start, so every replay here begins at the capture's first packet and the
// records of a shorter replay are a prefix of a longer one's.
func kirk0Replay(pcap, out string, start, duration float64) replayeval.Config {
	return replayeval.Config{
		PCAPFile: pcap, SensorID: "kirk0-codec", UDPPort: 2369, OutDir: out,
		StartSeconds: start, WarmupSeconds: start, DurationSeconds: duration,
		ReplayCaseID: "kirk0-codec",
		ObservationCalibration: l4bobserve.Calibration{SensorID: "kirk0-codec", FromFrame: "sensor", ToFrame: "site",
			Transform: [16]float64{1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1}},
	}
}

func decodeAll(t *testing.T, dir string, opts vrlog.Options) (*vrlog.Reader, []l4bobserve.FrameRecord) {
	t.Helper()
	r, err := vrlog.Open(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	var frames []l4bobserve.FrameRecord
	for {
		rec, err := r.Next()
		if errors.Is(err, io.EOF) {
			return r, frames
		}
		if err != nil {
			t.Fatal(err)
		}
		if rec.Kind != vrlog.RecordFrame {
			t.Fatalf("unexpected %s record", rec.Kind)
		}
		frames = append(frames, rec.Frame)
	}
}

// Evidence on real data, kept to one short tap replay plus a shorter repeat:
// the replayeval suite already spends minutes on kirk0 under -race. The
// first ten seconds of kirk0 hold both unsettled and observed frames.
//
// G-OBS-FID: every frame the tap produced decodes from the container equal
// value for value (float bits, presence, order, identities, membership,
// summaries), with equal semantic digests, and Verify reproduces the digests
// the writer sealed. G-OBS-REP (input): a separate replay of the same source
// reproduces the common prefix exactly under a different capture identity.
// Damage injected into copies of the container is refused and located.
//
// Phase 2, on the same replay: replayeval's container is committed through
// the durable group-commit writer; two more writers take the same tap frames,
// one under the default policy (for its measurements) and one in strict mode
// that is killed part-way through publishing a generation. Recovery of the
// killed one promotes that generation and exposes exactly the frames the tap
// produced up to it.
func TestKirk0ObservationContainer(t *testing.T) {
	pcap := requireKirk0(t)
	root := t.TempDir()

	refused := kirk0Replay(pcap, filepath.Join(root, "refused"), 0, 1)
	refused.ReplayCaseID = ""
	refused.ObservationLogDir = filepath.Join(root, "refused.vrlog")
	if _, err := replayeval.Run(refused); err == nil || !strings.Contains(err.Error(), "ReplayCaseID") {
		t.Fatalf("an observation log without identities = %v", err)
	}

	var direct []l4bobserve.FrameRecord
	cfg := kirk0Replay(pcap, filepath.Join(root, "replay"), 8, 2)
	cfg.ObservationLogDir = filepath.Join(root, "kirk0.vrlog")
	cfg.ObservationLogLimits = vrlog.DefaultLimits()
	cfg.ObservationLogLimits.TargetChunkBytes = 256 << 10 // several chunks to damage and seek across
	grouped := durableWriter(t, filepath.Join(root, "grouped.vrlog"), cfg.ObservationCalibration, vrlog.CommitPolicy{})
	killed := durableWriter(t, filepath.Join(root, "killed.vrlog"), cfg.ObservationCalibration, vrlog.CommitPolicy{Strict: true})
	const killGeneration = 40 // strict: generation n commits frame n-1
	vrlog.KillAt(killed, vrlog.StepGenerationPublished, killGeneration)
	var announced vrlog.Frontier
	cfg.ObservationFrames = func(f l4bobserve.FrameRecord) error {
		direct = append(direct, f)
		if err := grouped.AppendFrame(f); err != nil {
			return err
		}
		// Once killed, the writer refuses everything; the replay goes on.
		if killed.AppendFrame(f) == nil {
			announced = killed.Frontier()
		}
		return nil
	}
	result, err := replayeval.Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
	groupedStats := grouped.Stats()
	if _, err := grouped.Close(); err != nil {
		t.Fatal(err)
	}
	killedStats := killed.Stats()
	summary := result.ObservationLog
	if summary == nil || summary.Frames != uint64(len(direct)) || len(direct) != result.FramesRead || summary.Chunks < 3 || summary.RejectedFrames != 0 {
		t.Fatalf("summary %+v for %d direct frames, %d read", summary, len(direct), result.FramesRead)
	}

	readStart := time.Now()
	r, decoded := decodeAll(t, cfg.ObservationLogDir, vrlog.Options{Require: l4bobserve.ForegroundComplete().Capabilities.List()})
	readElapsed := time.Since(readStart)
	if len(decoded) != len(direct) {
		t.Fatalf("decoded %d frames of %d", len(decoded), len(direct))
	}
	stream := l4bobserve.NewStreamDigest()
	var points, clusters, members int
	dispositions := map[l4bobserve.DispositionKind]int{}
	for i := range direct {
		if err := l4bobserve.DiffFrames(direct[i], decoded[i]); err != nil {
			t.Fatalf("frame %d: %v", direct[i].Sequence, err)
		}
		d := l4bobserve.FrameDigest(direct[i])
		if d != l4bobserve.FrameDigest(decoded[i]) {
			t.Fatalf("frame %d: semantic digests differ", direct[i].Sequence)
		}
		stream.AddFrame(d)
		points += direct[i].Points.Len()
		clusters += len(direct[i].Clusters)
		for _, c := range direct[i].Clusters {
			members += len(c.Members)
		}
		dispositions[direct[i].Disposition.Kind]++
	}
	if clusters == 0 || dispositions[l4bobserve.DispositionObserved] == 0 || dispositions[l4bobserve.DispositionUnsettled] == 0 {
		t.Fatalf("the window lacks clusters or a disposition: %d clusters, %v", clusters, dispositions)
	}
	if stream.Sum() != summary.Semantic {
		t.Fatal("the direct frames do not digest to the container's sealed stream digest")
	}
	report, err := r.Verify()
	if err != nil || report.Semantic != summary.Semantic {
		t.Fatalf("verify = %+v, %v", report, err)
	}
	m := r.Manifest()
	if m.Extraction.SourceID != result.ObservationSourceID || m.Extraction.Window.WarmupNanos != 8e9 || m.Extraction.Window.DurationNanos != 2e9 {
		t.Fatalf("manifest extraction = %+v", m.Extraction)
	}
	if _, ok := m.MetadataObject("tuning"); !ok {
		t.Fatal("the effective tuning is not embedded")
	}

	if st := r.Status(); !st.Closed || st.Generation < 2 || summary.Commits == 0 {
		t.Fatalf("the replay's container was not committed in generations: %+v, summary %+v", st, summary)
	}

	// The killed writer announced generation 39 and died publishing 40.
	// Before recovery, a reader sees the acknowledged prefix; recovery
	// promotes the published generation and nothing else.
	if announced.Generation != killGeneration-1 || announced.Records != killGeneration-1 {
		t.Fatalf("frontier announced before the kill = %+v", announced)
	}
	killedDir := filepath.Join(root, "killed.vrlog")
	before, err := vrlog.Open(killedDir, vrlog.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if st := before.Status(); st.Records != killGeneration-1 || len(st.Unpromoted) != 1 || st.State != vrlog.CaptureOpen {
		t.Fatalf("killed container before recovery = %+v", st)
	}
	recovered, err := vrlog.Recover(killedDir, vrlog.Options{})
	if err != nil || recovered.Records != killGeneration || len(recovered.Promoted) != 1 || recovered.State != vrlog.CaptureIncomplete {
		t.Fatalf("recovery = %+v, %v", recovered, err)
	}
	kr, prefix := decodeAll(t, killedDir, vrlog.Options{})
	if len(prefix) != killGeneration {
		t.Fatalf("recovered %d frames", len(prefix))
	}
	for i := range prefix {
		if err := l4bobserve.DiffFrames(direct[i], prefix[i]); err != nil || l4bobserve.FrameDigest(direct[i]) != l4bobserve.FrameDigest(prefix[i]) {
			t.Fatalf("recovered frame %d differs from the tap's: %v", i, err)
		}
	}
	if _, err := kr.Verify(); err != nil {
		t.Fatal(err)
	}
	if again, err := vrlog.Recover(killedDir, vrlog.Options{}); err != nil || again.Wrote || again.PointerRewritten {
		t.Fatalf("second recovery = %+v, %v", again, err)
	}

	// Encode cost, with no replay around it: the same frames into a fresh
	// container, in strict mode so every frame is its own generation (what
	// a 10 Hz live capture at a 100 ms batch age amounts to). Chunking and
	// identity differ; the evidence does not.
	rewrite := m
	rewrite.Capture.UUID, rewrite.Capture.CreatedUnixNanos, rewrite.Limits = "", 0, vrlog.Limits{}
	rewrite.Commit = vrlog.CommitPolicy{Strict: true}
	var memBefore, memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)
	writeStart := time.Now()
	w, err := vrlog.Create(filepath.Join(root, "rewrite.vrlog"), rewrite)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range direct {
		if err := w.AppendFrame(f); err != nil {
			t.Fatal(err)
		}
	}
	strictStats := w.Stats()
	rewritten, err := w.Close()
	writeElapsed := time.Since(writeStart)
	runtime.ReadMemStats(&memAfter)
	if err != nil || rewritten.Semantic != summary.Semantic || rewritten.Chunks == summary.Chunks {
		t.Fatalf("rewrite = %+v, %v", rewritten, err)
	}
	n := float64(len(direct))
	t.Logf("kirk0 [0, 10) s: %d frames (%v), %d retained points (%.0f/frame), %d clusters, %d members",
		len(direct), dispositions, points, float64(points)/n, clusters, members)
	t.Logf("container: %d chunks, %d bytes, %.0f bytes/frame (%.1f bytes/point); strict write %.3f ms/frame, read+decode %.3f ms/frame",
		summary.Chunks, summary.ChunkBytes, float64(summary.ChunkBytes)/n, float64(summary.ChunkBytes)/float64(points),
		float64(writeElapsed.Microseconds())/1000/n, float64(readElapsed.Microseconds())/1000/n)
	for _, run := range []struct {
		name  string
		stats vrlog.WriterStats
	}{{"group commit during the replay", groupedStats}, {"strict during the replay (killed)", killedStats}, {"strict rewrite", strictStats}} {
		logWriterStats(t, run.name, run.stats)
	}
	blocks, files := diskUsage(t, filepath.Join(root, "rewrite.vrlog"))
	t.Logf("strict rewrite on disk: %d files, %d bytes allocated for %d chunk bytes (%.2fx at the block level); writer heap %.0f bytes and %.0f allocations per frame",
		files, blocks, rewritten.ChunkBytes, float64(blocks)/float64(rewritten.ChunkBytes),
		float64(memAfter.TotalAlloc-memBefore.TotalAlloc)/n, float64(memAfter.Mallocs-memBefore.Mallocs)/n)
	t.Logf("process peak RSS %s (the whole test: replay pipeline, three writers and the readers)", peakRSS())

	// The repeat is also a replay that fails part-way: its tap consumer
	// refuses frame 40. The frames accepted up to then are committed and a
	// failure generation says why the extraction stopped.
	t.Run("repeat reproduces the evidence", func(t *testing.T) {
		const stopAt = 40
		repeat := kirk0Replay(pcap, filepath.Join(root, "repeat"), 0, 5)
		repeat.ObservationLogDir = filepath.Join(root, "repeat.vrlog")
		repeat.ObservationFrames = func(f l4bobserve.FrameRecord) error {
			if f.Sequence == stopAt {
				return errors.New("injected stop")
			}
			return nil
		}
		if _, err := replayeval.Run(repeat); err == nil || !strings.Contains(err.Error(), "injected stop") {
			t.Fatalf("a replay whose consumer failed = %v", err)
		}
		rr, again := decodeAll(t, repeat.ObservationLogDir, vrlog.Options{})
		st := rr.Status()
		if st.State != vrlog.CaptureFailed || st.Failure.Cause != vrlog.FailureStopped ||
			!strings.Contains(st.Failure.Detail, "injected stop") || st.Frames != stopAt+1 || st.TailExtentUnknown {
			t.Fatalf("stopped replay's container = %+v, failure %+v", st, st.Failure)
		}
		// Every frame up to the stop is the same observation as the first
		// replay's; none is the partial rotation a window's end leaves.
		if len(again) != stopAt+1 || len(again) > len(direct) {
			t.Fatalf("repeat decoded %d frames", len(again))
		}
		for i := range again {
			if err := l4bobserve.DiffFrames(direct[i], again[i]); err != nil {
				t.Fatalf("frame %d differs between runs: %v", i, err)
			}
			if l4bobserve.FrameDigest(direct[i]) != l4bobserve.FrameDigest(again[i]) {
				t.Fatalf("frame %d digest differs between runs", i)
			}
		}
		a, b := m, rr.Manifest()
		if a.Capture.UUID == b.Capture.UUID {
			t.Fatal("two replays share a capture identity")
		}
		if a.Extraction.SourceID != b.Extraction.SourceID || a.Extraction.CalibrationID != b.Extraction.CalibrationID ||
			a.Provenance.ParamsHash != b.Provenance.ParamsHash {
			t.Fatal("the same source and tuning produced different extraction identities")
		}
	})

	chunks := r.Chunks()
	for name, tc := range map[string]struct {
		inject func(t *testing.T, dir string)
		open   bool // damage Open cannot see without reading the chunk body
		kind   vrlog.CorruptionKind
		chunk  int64
	}{
		"bit flip": {func(t *testing.T, dir string) {
			offset, length := vrlog.RecordSpan(t, dir, 2, 0)
			vrlog.FlipBit(t, vrlog.ChunkPath(dir, 2), offset+uint64(length)/2, 5)
		}, true, vrlog.CorruptDigest, 2},
		"truncation": {func(t *testing.T, dir string) {
			last := uint64(len(chunks) - 1)
			vrlog.Truncate(t, vrlog.ChunkPath(dir, last), int64(chunks[last].Bytes)-1)
		}, false, vrlog.CorruptTruncated, int64(len(chunks) - 1)},
		"reordered chunk": {func(t *testing.T, dir string) {
			vrlog.SwapFiles(t, vrlog.ChunkPath(dir, 1), vrlog.ChunkPath(dir, 2))
			vrlog.SwapFiles(t, vrlog.IndexPath(dir, 1), vrlog.IndexPath(dir, 2))
		}, false, vrlog.CorruptDisagreement, 1},
		"oversized length": {func(t *testing.T, dir string) {
			// Re-sealed and re-committed consistently: only the length lies.
			vrlog.ForgeRecordLength(t, dir, 1, 0, 0xfffffff0)
		}, true, vrlog.CorruptLength, 1},
	} {
		t.Run(name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "copy.vrlog")
			copyTree(t, cfg.ObservationLogDir, dir)
			tc.inject(t, dir)
			damaged, err := vrlog.Open(dir, vrlog.Options{})
			if tc.open {
				if err != nil {
					t.Fatalf("Open should defer this damage to the read: %v", err)
				}
				read := 0
				for err == nil {
					var rec vrlog.Record
					if rec, err = damaged.Next(); err == nil {
						if derr := l4bobserve.DiffFrames(direct[read], rec.Frame); derr != nil {
							t.Fatalf("an undamaged frame changed: %v", derr)
						}
						read++
					}
				}
				if uint64(read) != chunks[tc.chunk].FirstSequence {
					t.Fatalf("read %d frames before the damage in chunk %d", read, tc.chunk)
				}
			}
			var c *vrlog.CorruptionError
			if !errors.As(err, &c) || c.Kind != tc.kind || c.Chunk != tc.chunk {
				t.Fatalf("error = %v, want %s in chunk %d", err, tc.kind, tc.chunk)
			}
			if tc.open && (!c.HaveSequences || c.FirstSequence != chunks[tc.chunk].FirstSequence) {
				t.Fatalf("the damage is not located to its sequences: %+v", c)
			}
		})
	}
}

// durableWriter creates a test-owned container for tap frames. Its source
// identity is a test label, not a replay's: the frames are what is compared.
func durableWriter(t *testing.T, dir string, calibration l4bobserve.Calibration, policy vrlog.CommitPolicy) *vrlog.Writer {
	t.Helper()
	calibrationID, err := l4bobserve.CalibrationID(calibration)
	if err != nil {
		t.Fatal(err)
	}
	w, err := vrlog.Create(dir, vrlog.Manifest{
		Capture: vrlog.CaptureIdentity{SensorID: calibration.SensorID, SourceType: "pcap"},
		Extraction: vrlog.ExtractionIdentity{SourceID: "source/v1/kirk0-durable-test", CalibrationID: calibrationID,
			CoordinateFrame: "site/" + calibration.SensorID, ExtractorID: "l4.test/kirk0-durable"},
		Calibration: calibration,
		Commit:      policy,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = w.Abandon() })
	return w
}

func logWriterStats(t *testing.T, name string, s vrlog.WriterStats) {
	var total uint64
	for _, b := range s.ObjectBytes {
		total += b
	}
	total -= s.ObjectBytes["reserve"]
	t.Logf("%s: %d generations; publish p50 %s p95 %s max %s (sync p50 %s p95 %s max %s); batch age p50 %s max %s; "+
		"%d bytes written for %d payload bytes (%.3fx logical, reserve excluded), %d files, %d syncs",
		name, s.Publish.Count, s.Publish.P50, s.Publish.P95, s.Publish.Max, s.Sync.P50, s.Sync.P95, s.Sync.Max,
		s.BatchAge.P50, s.BatchAge.Max, total, s.PayloadBytes, float64(total)/float64(s.PayloadBytes), s.Files, s.Syncs)
}

// diskUsage returns the bytes allocated to the files under dir, and their
// number.
func diskUsage(t *testing.T, dir string) (int64, int) {
	var blocks int64
	files := 0
	filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		files++
		if st, ok := info.Sys().(*syscall.Stat_t); ok {
			blocks += st.Blocks * 512
		}
		return nil
	})
	return blocks, files
}

// peakRSS reads the process's high-water resident set (Linux only).
func peakRSS() string {
	data, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return "unavailable"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmHWM:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "VmHWM:"))
		}
	}
	return "unavailable"
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	err := filepath.WalkDir(from, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, path)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(to, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}
