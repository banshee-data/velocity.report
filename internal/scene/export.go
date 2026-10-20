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

// DefaultChunkSeconds is the target wall-clock span of one chunk file.
//
// Chunks are cut by duration rather than by frame count so a boundary means
// the same thing whatever the stride: at stride 2 a fixed 300 frames is a
// minute, at stride 1 it is thirty seconds. A minute also makes a chunk
// listing legible — chunk N is roughly minute N.
//
// Size is not why. Measured on the reference capture, moving from 20 s to 60 s
// chunks saved 1.4% and a single whole-recording chunk saved 2.0%: NDJSON
// frames are so self-similar that gzip's window saturates within a few frames.
// The trade is fetch granularity against file count, not compression.
const DefaultChunkSeconds = 60.0

// MaxChunkFrames bounds a chunk when a scene is dense enough that a minute of
// it would be an unreasonable single fetch.
const MaxChunkFrames = 1500

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
	// StartFrame and FrameCount bound the source range in sensor rotations,
	// as Stride does: background snapshots are not counted. FrameCount <= 0
	// means "to the end of the recording".
	//
	// Counting records instead made the cut depend on how many snapshots
	// preceded it, and that number varies with the replay's speed; the same
	// capture recorded identically at analysis, 1x and 0.5x then exported
	// from three different rotations.
	StartFrame int
	FrameCount int
	// ChunkSeconds is the target span of one chunk file. Chunks also rotate at
	// MaxChunkFrames so a dense scene cannot produce an outsized fetch.
	ChunkSeconds float64
	// Site and Title label the export for display.
	Site  string
	Title string
	// MaxPointsPerFrame caps the foreground cloud in a clip export. 0 = uncapped.
	MaxPointsPerFrame int
	// VoxelMetres downsamples a background export. 0 uses the default.
	VoxelMetres float64
	// BucketSeconds is the timeline summary resolution. 0 uses the default.
	BucketSeconds float64
	// KeepTrackIDs writes the tracker's own track identifiers instead of
	// re-keying them per export. Only for exports that stay on the device:
	// the operator's Tracks page matches a clicked box to the run's tracks by
	// identifier. A published export re-keys, so a trajectory cannot be linked
	// across parts or sites.
	KeepTrackIDs bool
	// SmoothedTrail writes each track's x, y, vx, vy as the whole-track RTS
	// smoother's estimate rather than the filter's online posterior (see
	// trail.go). It needs the recording's filter record, which a replay
	// writes beside its chunks; the header then names the trail's
	// estimator and stage. Off, the trail is the posterior, as before.
	SmoothedTrail bool
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
	// PointCount is the number of points a background export wrote.
	PointCount int
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
	if o.ChunkSeconds <= 0 {
		o.ChunkSeconds = DefaultChunkSeconds
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

	// The smoothed trail is decided before any frame is read: a recording
	// that cannot be smoothed must not be half-exported as though it could.
	var trail *smoothedTrail
	trailEstimator := TrailEstimatorOnline
	if opts.SmoothedTrail {
		trail, err = loadSmoothedTrail(opts.VRLOGPath)
		if err != nil {
			return nil, fmt.Errorf("smoothed trail: %w", err)
		}
		trailEstimator = trail.estimator
	}

	// The range is in rotations, so the recording is read from its first
	// record and the snapshots before the start are passed over rather than
	// sought past: a record index cannot name a rotation.
	start := opts.StartFrame
	if start < 0 {
		start = 0
	}
	if src.RotationFrames > 0 && uint64(start) >= src.RotationFrames {
		return nil, fmt.Errorf("start frame %d is beyond the recording (%d rotations)", start, src.RotationFrames)
	}
	end := -1 // no bound: to the end of the recording
	if opts.FrameCount > 0 {
		end = start + opts.FrameCount
	}

	if err := os.MkdirAll(filepath.Join(opts.OutDir, "frames"), 0o755); err != nil {
		return nil, fmt.Errorf("create export dir: %w", err)
	}

	w := &chunkWriter{
		dir:         filepath.Join(opts.OutDir, "frames"),
		chunkSpanUs: int64(opts.ChunkSeconds * 1e6),
		maxFrames:   MaxChunkFrames,
	}
	// Track identifiers are re-keyed per export. They need only be stable
	// enough for trail continuity inside this part; making them local means a
	// trajectory cannot be linked across parts or sites.
	keys := newTrackKeyer()
	// The timeline summary is folded in during this same pass, so annotating
	// the scrubber costs no extra read of the recording.
	timeline := newTimelineAccumulator(opts.BucketSeconds)

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
		timeline.observe(f)
		return w.write(f)
	}

	for idx := 0; ; idx++ {
		fb, err := rep.ReadFrame()
		if err != nil {
			if err == io.EOF {
				break
			}
			return nil, fmt.Errorf("read frame %d: %w", idx, err)
		}

		if !isRotation(fb) {
			if rotationsSeen >= start {
				sourceFramesRead++
				skipped++
			}
			continue
		}

		// The range and the stride count rotations, not records. Applying
		// either to the raw record index would let interleaved background
		// snapshots, whose number varies with the replay's speed, shift
		// which rotations survive.
		rotationIdx := rotationsSeen
		rotationsSeen++
		if rotationIdx < start {
			continue
		}
		if end >= 0 && rotationIdx >= end {
			break
		}
		sourceFramesRead++
		if (rotationIdx-start)%opts.Stride != 0 {
			continue
		}
		frame, _ := projectFrame(fb, opts, keys, trail)

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
	wholeRecording := start == 0 && end < 0
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
	// A filter record that matches none of the recording's track frames is
	// another recording's, or the frames' capture times are not the
	// tracker's: either way the trail would be the online one under a
	// smoothed name.
	if trail != nil && trail.summary.Substituted == 0 && trail.summary.OnlineFallback > 0 {
		return nil, fmt.Errorf("smoothed trail: the filter record matches none of the %d track frames read from %s",
			trail.summary.OnlineFallback, opts.VRLOGPath)
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
		ChunkSeconds:  opts.ChunkSeconds,
		CoordinateFrame: CoordinateFrame{
			FrameID:        src.CoordinateFrame.FrameID,
			ReferenceFrame: src.CoordinateFrame.ReferenceFrame,
		},
		DroppedNonMonotonic: nonMonotonic,
		TuningHash:          src.TuningHash,
		BuildVersion:        src.BuildVersion,
		GeneratedAt:         time.Now().UTC().Format(time.RFC3339),
		TrailEstimator:      trailEstimator,
	}
	if trail != nil {
		summary := trail.summary
		hdr.TrailSmoothing = &summary
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
	if err := timeline.write(opts.OutDir); err != nil {
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

// isRotation reports whether a record is a sensor rotation: what the range,
// the stride and the header's rotation count all count.
//
// Background snapshots are pipeline state, not observations of the street,
// and they are not sensor rotations. They also carry an inherited timestamp
// that can sit outside the range of the rotations around them — a recording
// opens with one so a replay has a scene from frame zero, and after a settling
// pass that timestamp is the end of the capture. Skipping them keeps the
// exported timeline monotonic and the frame count equal to the recording's
// rotation count.
func isRotation(fb *l9endpoints.FrameBundle) bool {
	return fb != nil && fb.TimestampNanos != 0 && fb.FrameType != l9endpoints.FrameTypeBackground
}

// projectFrame narrows a FrameBundle to the viewer-facing record for the
// requested export kind. It reports false when the frame carries nothing worth
// publishing. trail, when set, supplies each track's smoothed state in place
// of the recorded posterior.
func projectFrame(fb *l9endpoints.FrameBundle, opts Options, keys *trackKeyer, trail *smoothedTrail) (Frame, bool) {
	if !isRotation(fb) {
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
				// The four state components are the only thing a smoothed
				// trail changes; the rest of the record is the tracker's.
				x, y, vx, vy := trail.state(t.TrackID, fb.TimestampNanos, t.X, t.Y, t.VX, t.VY)
				out.Tracks = append(out.Tracks, TrackJSON{
					ID:       trackID(t.TrackID, opts, keys),
					X:        round2(x),
					Y:        round2(y),
					Z:        round2(t.Z),
					VX:       round2(vx),
					VY:       round2(vy),
					Speed:    round2(t.SpeedMps),
					MaxSpeed: round2(t.MaxSpeedMps),
					Heading:  round3(t.HeadingRad),
					Length:   round2(t.BBoxLength),
					Width:    round2(t.BBoxWidth),
					Height:   round2(t.BBoxHeight),
					BoxYaw:   round3(t.BBoxHeadingRad),
					Class:    t.ObjectClass,
					Conf:     round2(t.ClassConfidence),
				})
				if t.BBoxCenterValid {
					bx, by := round2(t.BBoxCenterX), round2(t.BBoxCenterY)
					out.Tracks[len(out.Tracks)-1].BoxX = &bx
					out.Tracks[len(out.Tracks)-1].BoxY = &by
				}
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

// trackID is the identifier a track carries in the export: its own when the
// export stays on the device, a per-export key otherwise.
func trackID(src string, opts Options, keys *trackKeyer) string {
	if opts.KeepTrackIDs {
		return src
	}
	return keys.local(src)
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
	chunkSpanUs int64
	maxFrames   int

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
	if f.TimeUs-w.curT0 >= w.chunkSpanUs || w.curN >= w.maxFrames {
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
