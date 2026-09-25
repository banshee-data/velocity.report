//go:build pcap

package vrlog_test

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
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

// Phase-1 evidence on real data, kept to one short tap replay plus a shorter
// repeat: the replayeval suite already spends minutes on kirk0 under -race.
// The first ten seconds of kirk0 hold both unsettled and observed frames.
//
// G-OBS-FID: every frame the tap produced decodes from the container equal
// value for value (float bits, presence, order, identities, membership,
// summaries), with equal semantic digests, and Verify reproduces the digests
// the writer sealed. G-OBS-REP (input): a separate replay of the same source
// reproduces the common prefix exactly under a different capture identity.
// Damage injected into copies of the container is refused and located.
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
	cfg.ObservationFrames = func(f l4bobserve.FrameRecord) error { direct = append(direct, f); return nil }
	result, err := replayeval.Run(cfg)
	if err != nil {
		t.Fatal(err)
	}
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

	// Encode cost, with no replay around it: the same frames into a fresh
	// container. Chunking and identity differ; the evidence does not.
	rewrite := m
	rewrite.Capture.UUID, rewrite.Capture.CreatedUnixNanos, rewrite.Limits = "", 0, vrlog.Limits{}
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
	rewritten, err := w.Close()
	writeElapsed := time.Since(writeStart)
	if err != nil || rewritten.Semantic != summary.Semantic || rewritten.Chunks == summary.Chunks {
		t.Fatalf("rewrite = %+v, %v", rewritten, err)
	}
	n := float64(len(direct))
	t.Logf("kirk0 [0, 10) s: %d frames (%v), %d retained points (%.0f/frame), %d clusters, %d members",
		len(direct), dispositions, points, float64(points)/n, clusters, members)
	t.Logf("container: %d chunks, %d bytes, %.0f bytes/frame (%.1f bytes/point); write %.3f ms/frame, read+decode %.3f ms/frame",
		summary.Chunks, summary.ChunkBytes, float64(summary.ChunkBytes)/n, float64(summary.ChunkBytes)/float64(points),
		float64(writeElapsed.Microseconds())/1000/n, float64(readElapsed.Microseconds())/1000/n)

	t.Run("repeat reproduces the evidence", func(t *testing.T) {
		repeat := kirk0Replay(pcap, filepath.Join(root, "repeat"), 0, 5)
		repeat.ObservationLogDir = filepath.Join(root, "repeat.vrlog")
		if _, err := replayeval.Run(repeat); err != nil {
			t.Fatal(err)
		}
		rr, again := decodeAll(t, repeat.ObservationLogDir, vrlog.Options{})
		// The last frame of the shorter replay is the partial rotation it
		// stopped in; every frame before it is the same observation.
		if len(again) < 40 || len(again) > len(direct) {
			t.Fatalf("repeat decoded %d frames", len(again))
		}
		for i := range again[:len(again)-1] {
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
