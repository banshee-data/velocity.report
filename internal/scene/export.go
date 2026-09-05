package scene

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// DefaultChunkFrames is the retained-frame count per chunk file. Small enough
// that a chunk is a whole-file fetch, so seeking never needs a byte range.
const DefaultChunkFrames = 100

// Options configures one export.
type Options struct {
	// VRLOGPath is the recorded VRLOG directory to read.
	VRLOGPath string
	// OutDir is the export directory to write.
	OutDir string
	// Kind selects what the export carries.
	Kind Kind
	// Stride retains every Nth source frame. 1 keeps every frame. This is a
	// retention interval, not a frame rate.
	Stride int
	// StartFrame and FrameCount bound the source range. FrameCount <= 0 means
	// "to the end of the recording".
	StartFrame int
	FrameCount int
	// ChunkFrames is retained frames per chunk file.
	ChunkFrames int
	// Site and Title label the export for display.
	Site  string
	Title string
	// MaxPointsPerFrame caps the foreground cloud in a clip export. 0 = uncapped.
	MaxPointsPerFrame int
}

// Result reports what an export produced.
type Result struct {
	Header       Header
	Chunks       int
	BytesOnDisk  int64
	SourceFrames int
	Skipped      int
	// DroppedNonMonotonic counts frames discarded because their timestamp did
	// not advance past the previous retained frame.
	DroppedNonMonotonic int
}

// maxNonMonotonic is the tolerance for out-of-order frames: one, or 0.1% of
// the recording, whichever is larger.
func maxNonMonotonic(retained int) int {
	if limit := retained / 1000; limit > 1 {
		return limit
	}
	return 1
}

func (o *Options) applyDefaults() {
	if o.Stride < 1 {
		o.Stride = 1
	}
	if o.ChunkFrames < 1 {
		o.ChunkFrames = DefaultChunkFrames
	}
	if o.Kind == "" {
		o.Kind = KindTracks
	}
}

// round2 quantises to 1 cm. The Pandar40P's range accuracy is +-2 cm, so this
// sits at the sensor's noise floor. Unrounded float64 costs roughly three
// times the bytes after gzip, because 17 significant digits of arithmetic
// noise do not compress.
func round2(v float32) float64 {
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return 0
	}
	return math.Round(float64(v)*100) / 100
}

// round3 quantises an angle to ~0.06 degrees.
func round3(v float32) float64 {
	if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
		return 0
	}
	return math.Round(float64(v)*1000) / 1000
}

// Export reads a recorded VRLOG and writes a scene export directory.
//
// Timestamps are copied through unrounded: they are the playback clock, and a
// VRLOG frame is one sensor rotation rather than one tick of a fixed interval.
func Export(opts Options) (*Result, error) {
	opts.applyDefaults()

	rep, err := recorder.NewReplayer(opts.VRLOGPath)
	if err != nil {
		return nil, fmt.Errorf("open vrlog: %w", err)
	}
	defer func() { _ = rep.Close() }()

	src := rep.Header()
	total := int(rep.TotalFrames())
	if total == 0 {
		return nil, fmt.Errorf("vrlog %s contains no frames", opts.VRLOGPath)
	}

	start := opts.StartFrame
	if start < 0 {
		start = 0
	}
	if start >= total {
		return nil, fmt.Errorf("start frame %d is beyond the recording (%d frames)", start, total)
	}
	end := total
	if opts.FrameCount > 0 && start+opts.FrameCount < total {
		end = start + opts.FrameCount
	}

	if err := os.MkdirAll(filepath.Join(opts.OutDir, "frames"), 0o755); err != nil {
		return nil, fmt.Errorf("create export dir: %w", err)
	}

	if err := rep.Seek(uint64(start)); err != nil {
		return nil, fmt.Errorf("seek to frame %d: %w", start, err)
	}

	w := &chunkWriter{
		dir:         filepath.Join(opts.OutDir, "frames"),
		chunkFrames: opts.ChunkFrames,
	}
	// Track identifiers are re-keyed per export. They need only be stable
	// enough for trail continuity inside this part; making them local means a
	// trajectory cannot be linked across parts or sites.
	keys := newTrackKeyer()

	var (
		retained         int
		skipped          int
		nonMonotonic     int
		rotationsSeen    int
		firstNs, lastNs  int64
		sourceFramesRead int
	)

	var pending *Frame

	// commit publishes f, using nextNs (the timestamp of the frame that
	// follows it) to validate the very first frame. Later frames are judged
	// against the last one already published.
	commit := func(f Frame, nextNs int64) error {
		absNs := f.TimeUs // projectFrame leaves absolute ns here; rebased below.
		if retained == 0 {
			if absNs >= nextNs {
				nonMonotonic++
				return nil
			}
			firstNs = absNs
		} else if absNs <= lastNs {
			nonMonotonic++
			return nil
		}
		lastNs = absNs
		retained++
		// Rebase to microseconds from the first retained frame so the value is
		// exact in a browser.
		f.TimeUs = (absNs - firstNs) / 1000
		return w.write(f)
	}

	for idx := start; idx < end; idx++ {
		fb, err := rep.ReadFrame()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("read frame %d: %w", idx, err)
		}
		sourceFramesRead++

		frame, ok := projectFrame(fb, opts, keys)
		if !ok {
			skipped++
			continue
		}

		// Stride counts rotations, not records. Applying it to the raw record
		// index would let interleaved background snapshots shift which
		// rotations survive, so "every second rotation" would not be true.
		rotationIdx := rotationsSeen
		rotationsSeen++
		if rotationIdx%opts.Stride != 0 {
			continue
		}

		// Hold each frame back by one so the opening frame can be judged
		// against its successor rather than becoming the baseline on trust.
		// The recorder is known to stamp the first frame from the wall clock
		// while the rest carry capture time (see VRLOG_RUN_COMPARISON 2.2), so
		// a recording can open with a timestamp from its own future. Anchoring
		// on that frame would reject every frame after it.
		if pending == nil {
			f := frame
			pending = &f
			continue
		}
		if err := commit(*pending, frame.TimeUs); err != nil {
			return nil, err
		}
		f := frame
		pending = &f
	}

	// The final held frame has no successor to check against; accept it if it
	// advances the clock.
	if pending != nil {
		if err := commit(*pending, pending.TimeUs+1); err != nil {
			return nil, err
		}
	}

	if err := w.close(); err != nil {
		return nil, err
	}

	// Reconcile against the recording's own rotation count. A VRLOG written
	// before that field existed reports zero, and a bounded export covers only
	// part of the recording, so neither is treated as a disagreement.
	wholeRecording := start == 0 && end == total
	if src.RotationFrames > 0 && wholeRecording {
		if uint64(rotationsSeen) != src.RotationFrames {
			return nil, fmt.Errorf(
				"%s: read %d rotations but the header declares %d; "+
					"the recording and its index disagree and must not be published",
				opts.VRLOGPath, rotationsSeen, src.RotationFrames)
		}
	}
	if retained == 0 {
		return nil, fmt.Errorf("no frames retained from %s: nothing to publish", opts.VRLOGPath)
	}

	// A handful of out-of-order frames is the known first-frame artefact. A
	// large share means the recording's time domain is genuinely mixed, and
	// publishing it would misrepresent when things happened.
	if limit := maxNonMonotonic(retained); nonMonotonic > limit {
		return nil, fmt.Errorf(
			"%s: %d of %d retained frames have non-monotonic timestamps (limit %d); "+
				"the recording mixes wall-clock and capture time and must not be published",
			opts.VRLOGPath, nonMonotonic, retained+nonMonotonic, limit)
	}

	hdr := Header{
		Version:       FormatVersion,
		Export:        opts.Kind,
		Site:          opts.Site,
		Title:         opts.Title,
		SensorID:      src.SensorID,
		FrameCount:    retained,
		StartNs:       strconv.FormatInt(firstNs, 10),
		DurationSec:   float64(lastNs-firstNs) / 1e9,
		FrameStride:   opts.Stride,
		ChunkEncoding: "gzip",
		ChunkFrames:   opts.ChunkFrames,
		CoordinateFrame: CoordinateFrame{
			FrameID:        src.CoordinateFrame.FrameID,
			ReferenceFrame: src.CoordinateFrame.ReferenceFrame,
		},
		DroppedNonMonotonic: nonMonotonic,
		TuningHash:          src.TuningHash,
		BuildVersion:        src.BuildVersion,
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339),
	}
	if sum, err := sourceFingerprint(opts.VRLOGPath); err == nil {
		hdr.SourceVRLOGSHA256 = sum
	}

	if err := writeJSON(filepath.Join(opts.OutDir, "header.json"), hdr); err != nil {
		return nil, err
	}
	if err := writeJSON(filepath.Join(opts.OutDir, "index.json"), ChunkIndex{
		Version: FormatVersion,
		Chunks:  w.entries,
	}); err != nil {
		return nil, err
	}

	return &Result{
		Header:              hdr,
		Chunks:              len(w.entries),
		BytesOnDisk:         dirBytes(opts.OutDir),
		SourceFrames:        sourceFramesRead,
		Skipped:             skipped,
		DroppedNonMonotonic: nonMonotonic,
	}, nil
}

// projectFrame narrows a FrameBundle to the viewer-facing record for the
// requested export kind. It reports false when the frame carries nothing worth
// publishing.
func projectFrame(fb *l9endpoints.FrameBundle, opts Options, keys *trackKeyer) (Frame, bool) {
	if fb == nil || fb.TimestampNanos == 0 {
		return Frame{}, false
	}
	// Background snapshots are pipeline state, not observations of the street,
	// and they are not sensor rotations. They also carry an inherited
	// timestamp that can sit outside the range of the rotations around them —
	// a recording opens with one so a replay has a scene from frame zero, and
	// after a settling pass that timestamp is the end of the capture. Skipping
	// them keeps the exported timeline monotonic and the frame count equal to
	// the recording's rotation count.
	if fb.FrameType == l9endpoints.FrameTypeBackground {
		return Frame{}, false
	}
	// Absolute nanoseconds at this point; Export rebases to relative
	// microseconds once the first retained frame is known.
	out := Frame{Frame: fb.FrameID, TimeUs: fb.TimestampNanos}

	if opts.Kind == KindTracks || opts.Kind == KindClip {
		if fb.Tracks != nil {
			for i := range fb.Tracks.Tracks {
				t := &fb.Tracks.Tracks[i]
				// Deleted tracks are pruning bookkeeping, not observations.
				if t.State == l9endpoints.TrackStateDeleted {
					continue
				}
				out.Tracks = append(out.Tracks, TrackJSON{
					ID:      keys.local(t.TrackID),
					X:       round2(t.X),
					Y:       round2(t.Y),
					Z:       round2(t.Z),
					VX:      round2(t.VX),
					VY:      round2(t.VY),
					Speed:   round2(t.SpeedMps),
					Heading: round3(t.HeadingRad),
					Length:  round2(t.BBoxLength),
					Width:   round2(t.BBoxWidth),
					Height:  round2(t.BBoxHeight),
					BoxYaw:  round3(t.BBoxHeadingRad),
					Class:   t.ObjectClass,
					Conf:    round2(t.ClassConfidence),
				})
			}
		}
	}

	if opts.Kind == KindClip && fb.PointCloud != nil {
		pc := fb.PointCloud
		n := len(pc.X)
		if n > len(pc.Y) {
			n = len(pc.Y)
		}
		if n > len(pc.Z) {
			n = len(pc.Z)
		}
		step := 1
		if opts.MaxPointsPerFrame > 0 && n > opts.MaxPointsPerFrame {
			step = (n + opts.MaxPointsPerFrame - 1) / opts.MaxPointsPerFrame
		}
		for i := 0; i < n; i += step {
			var intensity float64
			if i < len(pc.Intensity) {
				intensity = float64(pc.Intensity[i])
			}
			out.Points = append(out.Points, [4]float64{
				round2(pc.X[i]), round2(pc.Y[i]), round2(pc.Z[i]), intensity,
			})
		}
	}

	// An empty tracks frame is still a real observation: the street was empty
	// at that moment, and dropping it would distort the timeline.
	return out, true
}

// trackKeyer maps source track identifiers to short, export-local ones.
type trackKeyer struct {
	seen map[string]string
	next int
}

func newTrackKeyer() *trackKeyer {
	return &trackKeyer{seen: make(map[string]string)}
}

func (k *trackKeyer) local(src string) string {
	if v, ok := k.seen[src]; ok {
		return v
	}
	v := encodeBase36(k.next)
	k.next++
	k.seen[src] = v
	return v
}

func encodeBase36(n int) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	if n == 0 {
		return "0"
	}
	var buf []byte
	for n > 0 {
		buf = append([]byte{alphabet[n%36]}, buf...)
		n /= 36
	}
	return string(buf)
}

// chunkWriter accumulates frames into gzipped NDJSON chunk files.
type chunkWriter struct {
	dir         string
	chunkFrames int

	entries []ChunkEntry
	cur     *os.File
	gz      *gzip.Writer
	curID   int
	curN    int
	curT0   int64
	curT1   int64
}

func (w *chunkWriter) write(f Frame) error {
	if w.gz == nil {
		if err := w.open(); err != nil {
			return err
		}
		w.curT0 = f.TimeUs
	}
	line, err := json.Marshal(f)
	if err != nil {
		return fmt.Errorf("marshal frame %d: %w", f.Frame, err)
	}
	if _, err := w.gz.Write(append(line, '\n')); err != nil {
		return fmt.Errorf("write frame %d: %w", f.Frame, err)
	}
	w.curN++
	w.curT1 = f.TimeUs
	if w.curN >= w.chunkFrames {
		return w.rotate()
	}
	return nil
}

func (w *chunkWriter) open() error {
	name := filepath.Join(w.dir, fmt.Sprintf("chunk_%04d.ndjson.gz", w.curID))
	f, err := os.Create(name)
	if err != nil {
		return fmt.Errorf("create chunk: %w", err)
	}
	w.cur = f
	w.gz = gzip.NewWriter(f)
	w.curN = 0
	return nil
}

func (w *chunkWriter) rotate() error {
	if w.gz == nil {
		return nil
	}
	if err := w.gz.Close(); err != nil {
		return fmt.Errorf("close chunk gzip: %w", err)
	}
	if err := w.cur.Close(); err != nil {
		return fmt.Errorf("close chunk file: %w", err)
	}
	w.entries = append(w.entries, ChunkEntry{
		ID: w.curID, Frames: w.curN, StartUs: w.curT0, EndUs: w.curT1,
	})
	w.gz, w.cur = nil, nil
	w.curID++
	w.curN = 0
	return nil
}

func (w *chunkWriter) close() error {
	if w.curN > 0 {
		return w.rotate()
	}
	if w.gz != nil {
		_ = w.gz.Close()
		_ = w.cur.Close()
		w.gz, w.cur = nil, nil
	}
	return nil
}

// sourceFingerprint hashes the source VRLOG's header and seek index, which
// together identify the recording without reading every chunk.
func sourceFingerprint(vrlogPath string) (string, error) {
	h := sha256.New()
	for _, name := range []string{"header.json", "index.bin"} {
		b, err := os.ReadFile(filepath.Join(vrlogPath, name))
		if err != nil {
			return "", err
		}
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", filepath.Base(path), err)
	}
	if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", filepath.Base(path), err)
	}
	return nil
}

func dirBytes(dir string) int64 {
	var n int64
	_ = filepath.Walk(dir, func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			n += fi.Size()
		}
		return nil
	})
	return n
}
