package annotation

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// ExportConfig bounds an excerpt cut from a recording.
type ExportConfig struct {
	VRLOGPath string
	OutDir    string
	// StartNs and EndNs bound the excerpt by capture time. Zero values mean
	// the recording's own bounds.
	StartNs int64
	EndNs   int64
	// MaxSamples caps the excerpt. Annotation is human work measured in
	// minutes per frame, so an unbounded export is a way to produce a pack
	// nobody will ever finish labelling.
	MaxSamples int
	// Coverage and CoverageNote describe what the source could see. The
	// exporter cannot infer this: a recording holding only foreground points
	// looks exactly like a sparse full-scene one.
	Coverage     CaptureCoverage
	CoverageNote string
	// HeightBand is the run's L4 height-band filter, recorded in the manifest
	// when known. A VRLOG does not carry its run's parameters, so the caller
	// supplies them; nil leaves the manifest without one.
	HeightBand *HeightBand
}

// ErrNoReturnIdentity marks a recording whose point frames do not name the
// returns they came from: one made before the VRLOG carried return identity,
// or from a source without packets. Its points cannot become a version 2
// pack, and the remedy is to record again from the PCAP.
var ErrNoReturnIdentity = errors.New("recording carries no return identity")

// Export cuts a frozen pack from a VRLOG.
//
// It reads the source and writes a new directory; the recording is never
// modified. Frames without a point cloud are counted and skipped rather than
// written as empty samples, because an empty sample is indistinguishable from
// a frame where everything was occluded.
//
// The pack is version 2: every point carries the return it was decoded from,
// numbered in one packet table built across the exported frames in recording
// order. A recording without return identity is refused with
// ErrNoReturnIdentity rather than exported as a pack no label set can cite.
func Export(cfg ExportConfig) (*Pack, error) {
	if cfg.VRLOGPath == "" || cfg.OutDir == "" {
		return nil, fmt.Errorf("VRLOGPath and OutDir are required")
	}
	if cfg.Coverage == "" {
		return nil, fmt.Errorf("coverage must be stated explicitly: a limitation the export drops cannot be recovered later")
	}
	switch cfg.Coverage {
	case CoverageFull, CoverageForegroundOnly, CoverageDecimated:
	default:
		return nil, fmt.Errorf("unknown coverage %q", cfg.Coverage)
	}

	// Identify the source before reading it. A pack's whole value is that its
	// provenance is checkable, so a source that cannot be digested is not
	// worth the minutes it takes to export.
	headerSHA, err := fileSHA256(filepath.Join(cfg.VRLOGPath, "header.json"))
	if err != nil {
		return nil, fmt.Errorf("hash vrlog header: %w", err)
	}
	framesSHA, err := dirSHA256(filepath.Join(cfg.VRLOGPath, "frames"))
	if err != nil {
		return nil, fmt.Errorf("hash vrlog frames: %w", err)
	}

	replayer, err := recorder.NewReplayer(cfg.VRLOGPath)
	if err != nil {
		return nil, fmt.Errorf("open vrlog: %w", err)
	}
	defer replayer.Close()
	header := replayer.Header()

	comp := Completeness{RequestedStartNs: cfg.StartNs, RequestedEndNs: cfg.EndNs}
	table := newPackTable()
	var (
		samples  []Sample
		blocks   [][]byte
		ordinal  int
		lastTs   int64
		seenTs   = make(map[int64]int)
		hasInten bool
		hasClass bool
		// Classed points that are not foreground. A recording that kept the
		// whole scene has them in every frame.
		nonForeground int64

		backgrounds      []Background
		backgroundBlocks [][]byte
		// The latest snapshot seen that has not been written yet. Held rather
		// than written at once, because the one in force at the first sample
		// was usually recorded before the excerpt begins.
		pending      *Background
		pendingBlock []byte
	)
	keepPending := func() {
		if pending == nil {
			return
		}
		backgrounds = append(backgrounds, *pending)
		backgroundBlocks = append(backgroundBlocks, pendingBlock)
		pending, pendingBlock = nil, nil
	}

	for {
		frame, err := replayer.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read frame %d: %w", ordinal, err)
		}
		ordinal++

		ts := frame.TimestampNanos
		inWindow := (cfg.StartNs == 0 || ts >= cfg.StartNs) && (cfg.EndNs == 0 || ts <= cfg.EndNs)

		// Before the window test: the background in force when the excerpt
		// begins is whichever was recorded last before it.
		if bg := frame.Background; bg != nil && len(bg.X) > 0 {
			block, err := encodeBackground(bg.X, bg.Y, bg.Z)
			if err != nil {
				return nil, fmt.Errorf("frame %d (ordinal): %w", ordinal-1, err)
			}
			pending = &Background{
				SourceOrdinal: ordinal - 1, TimestampNs: bg.TimestampNanos,
				SequenceNumber: bg.SequenceNumber, SettlingComplete: bg.GridMetadata.SettlingComplete,
				PointCount: len(bg.X),
			}
			pendingBlock = block
			// Once the excerpt has begun, every snapshot belongs to it.
			if len(samples) > 0 {
				keepPending()
			}
		}

		pc := frame.PointCloud
		if pc == nil || len(pc.X) == 0 {
			// A frame with no points does not bound the excerpt. A background
			// frame's timestamp is when the snapshot was taken, and a replay
			// settled ahead of time opens with one stamped after every frame
			// that follows: breaking on it ended a windowed export at frame 0.
			if inWindow && frame.Background == nil {
				comp.FramesWithoutPoints++
			}
			continue
		}
		if cfg.StartNs != 0 && ts < cfg.StartNs {
			continue
		}
		if cfg.EndNs != 0 && ts > cfg.EndNs {
			break
		}

		block, err := encodePoints(Points{
			X: pc.X, Y: pc.Y, Z: pc.Z,
			Intensity: pc.Intensity, Classification: pc.Classification,
		})
		if err != nil {
			return nil, fmt.Errorf("frame %d (ordinal): %w", ordinal-1, err)
		}
		// The replayer hands back the legacy FrameBundle, whose point cloud
		// has no column-presence bit: an absent column decodes as length 0
		// and a present one as one byte per point. Nothing in between is
		// either, and the block would zero-fill the rest under a flag that
		// promised measurements, so it is refused rather than guessed.
		intensityPresent := len(pc.Intensity) == len(pc.X)
		if !intensityPresent && len(pc.Intensity) != 0 {
			return nil, fmt.Errorf("frame %d (ordinal): intensity column holds %d values for %d points, "+
				"which is neither present nor absent", ordinal-1, len(pc.Intensity), len(pc.X))
		}
		if intensityPresent {
			hasInten = true
		}
		if len(pc.Classification) > 0 {
			hasClass = true
		}
		for _, class := range pc.Classification {
			if class != classForeground {
				nonForeground++
			}
		}

		// Duplicate timestamps are legal. They are recorded rather than
		// rejected because the pack-local sample id keeps the reference
		// unambiguous either way.
		seenTs[ts]++
		if seenTs[ts] > 1 {
			comp.DuplicateTimestamps++
		}
		if len(samples) > 0 && ts > lastTs {
			if gap := ts - lastTs; gap > comp.MaxTimestampGapNs {
				comp.MaxTimestampGapNs = gap
			}
		}
		lastTs = ts

		if len(samples) == 0 {
			comp.ActualStartNs = ts
		}
		comp.ActualEndNs = ts

		returnStart, returnEnd, err := table.add(ordinal-1, pc)
		if err != nil {
			return nil, err
		}

		if len(samples) == 0 {
			keepPending()
		}
		samples = append(samples, Sample{
			SourceOrdinal: ordinal - 1,
			SourceFrameID: frameID(frame),
			TimestampNs:   ts,
			SensorID:      header.SensorID,
			PointCount:    len(pc.X),
			HasIntensity:  boolPtr(intensityPresent),
			ReturnStart:   returnStart,
			ReturnEnd:     returnEnd,
		})
		blocks = append(blocks, block)

		if cfg.MaxSamples > 0 && len(samples) >= cfg.MaxSamples {
			break
		}
	}

	if len(samples) == 0 {
		return nil, fmt.Errorf(
			"no point-bearing frames in the requested window: %d frames carried no point cloud "+
				"(record the source with --include-points)", comp.FramesWithoutPoints)
	}

	// The exporter cannot tell a sparse full scene from a foreground-only one
	// by counting points, which is why coverage is stated rather than
	// inferred. It can tell when the statement contradicts the recorder: a
	// full scene always has background in it, and the recorder classed every
	// one of these points as foreground.
	if cfg.Coverage == CoverageFull && hasClass && nonForeground == 0 {
		return nil, fmt.Errorf(
			"coverage %q contradicts the recording: every point in the %d exported frames is "+
				"classed foreground, so it kept foreground only (export it as %q)",
			CoverageFull, len(samples), CoverageForegroundOnly)
	}

	m := Manifest{
		CreatedNs: time.Now().UnixNano(),
		Source: SourceProvenance{
			VRLOGPath:      filepath.Base(cfg.VRLOGPath),
			VRLOGHeaderSHA: headerSHA,
			VRLOGFramesSHA: framesSHA,
			PCAPBasename:   header.PCAPPath,
			SensorID:       header.SensorID,
			RunConfigID:    header.RunConfigID,
			ConfigHash:     header.ConfigHash,
			ParamsHash:     header.ParamsHash,
			BuildVersion:   header.BuildVersion,
			BuildGitSHA:    header.BuildGitSHA,
			HeightBand:     cfg.HeightBand,
		},
		Coordinate: CoordinateContract{
			Units:          "metres",
			FrameID:        header.CoordinateFrame.FrameID,
			ReferenceFrame: header.CoordinateFrame.ReferenceFrame,
			Handedness:     "right",
			OriginNote:     "sensor origin as recorded; no site transform applied",
		},
		Coverage:          cfg.Coverage,
		CoverageNote:      cfg.CoverageNote,
		HasIntensity:      hasInten,
		HasClassification: hasClass,
		Completeness:      comp,
	}

	// A snapshot recorded after the last sample is in force for none of them.
	lastOrdinal := samples[len(samples)-1].SourceOrdinal
	for len(backgrounds) > 0 && backgrounds[len(backgrounds)-1].SourceOrdinal > lastOrdinal {
		backgrounds = backgrounds[:len(backgrounds)-1]
		backgroundBlocks = backgroundBlocks[:len(backgroundBlocks)-1]
	}

	if err := WritePackWithBackground(cfg.OutDir, m, samples, blocks, table.returns(), backgrounds, backgroundBlocks); err != nil {
		return nil, err
	}
	return OpenPack(cfg.OutDir)
}

// packTable builds a pack's packet table from the exported frames, in
// recording order, and renumbers each frame's returns into it.
//
// A frame's own table lists its packets in first-seen order. A packet split
// between two consecutive frames, as when the rotation wraps inside it, ends
// one frame's table and starts the next one's; it is listed once. Any other
// repetition of a packet means the recording does not identify its returns
// uniquely, and the export is refused rather than guess which one a point
// was.
type packTable struct {
	layout  ReturnLayout
	packets []uint64
	index   map[uint64]int
	// The first packet of the previous sample, and where its range ended: a
	// packet may be shared with that sample and no other, and the next
	// sample's returns must start after it.
	prevFirst int
	prevEnd   uint64
	perSample [][]uint64
}

func newPackTable() *packTable {
	return &packTable{index: make(map[uint64]int)}
}

// add renumbers one frame's returns into the table and returns its range.
func (t *packTable) add(ordinal int, pc *l9endpoints.PointCloudFrame) (start, end uint64, err error) {
	if !pc.HasReturnIdentity() {
		return 0, 0, fmt.Errorf("frame %d (ordinal): %w; it was recorded before return identity "+
			"existed or from a source without packets. Re-record it by replaying the PCAP with this "+
			"build, then export the new VRLOG", ordinal, ErrNoReturnIdentity)
	}
	layout := ReturnLayout{Model: pc.ReturnModel, Blocks: pc.ReturnBlocks, Channels: pc.ReturnChannels}
	if err := layout.validate(); err != nil {
		return 0, 0, fmt.Errorf("frame %d (ordinal): %w", ordinal, err)
	}
	if len(t.perSample) == 0 {
		t.layout = layout
	} else if layout != t.layout {
		return 0, 0, fmt.Errorf("frame %d (ordinal) numbers returns as %+v, earlier frames as %+v", ordinal, layout, t.layout)
	}
	for i, r := range pc.ReturnRefs {
		if r < pc.ReturnStart || r >= pc.ReturnEnd || (i > 0 && r <= pc.ReturnRefs[i-1]) {
			return 0, 0, fmt.Errorf("frame %d (ordinal) point %d: return %d is out of point order or outside "+
				"the frame's [%d,%d): the frame repeats a packet", ordinal, i, r, pc.ReturnStart, pc.ReturnEnd)
		}
	}

	// The leading packets already in the table must be its tail, in order,
	// and belong to the previous sample. Every packet after them is new.
	keys := pc.PacketKeys
	shared := 0
	for shared < len(keys) {
		if _, ok := t.index[keys[shared]]; !ok {
			break
		}
		shared++
	}
	tail := len(t.packets) - shared
	for j := 0; j < shared; j++ {
		if tail < t.prevFirst || t.packets[tail+j] != keys[j] {
			return 0, 0, t.repeated(ordinal, keys[j])
		}
	}
	mapping := make([]uint64, len(keys))
	for j := range keys {
		if j < shared {
			mapping[j] = uint64(tail + j)
			continue
		}
		if _, ok := t.index[keys[j]]; ok {
			return 0, 0, t.repeated(ordinal, keys[j])
		}
		t.index[keys[j]] = len(t.packets)
		mapping[j] = uint64(len(t.packets))
		t.packets = append(t.packets, keys[j])
	}

	per := layout.PerPacket()
	renumber := func(r uint64) uint64 { return mapping[r/per]*per + r%per }
	start, end = renumber(uint64(pc.ReturnStart)), renumber(uint64(pc.ReturnEnd)-1)+1
	if len(t.perSample) > 0 && start < t.prevEnd {
		return 0, 0, fmt.Errorf("frame %d (ordinal) starts at return %d, inside the previous sample's frame, "+
			"which ends at %d: two frames claim the same returns", ordinal, start, t.prevEnd)
	}
	returns := make([]uint64, len(pc.ReturnRefs))
	for i, r := range pc.ReturnRefs {
		returns[i] = renumber(uint64(r))
	}
	t.perSample = append(t.perSample, returns)
	t.prevFirst = int(mapping[0])
	t.prevEnd = end
	return start, end, nil
}

func (t *packTable) repeated(ordinal int, key uint64) error {
	return fmt.Errorf("frame %d (ordinal): packet %016x appeared earlier in the recording, not only at the "+
		"previous frame's boundary: its returns cannot be told apart, so this recording cannot be exported", ordinal, key)
}

func (t *packTable) returns() PackReturns {
	return PackReturns{Layout: t.layout, Packets: t.packets, Returns: t.perSample}
}

// classForeground is the recorder's per-point class for a foreground return
// (visualiser.proto: background=0, foreground=1, ground=2).
const classForeground = 1

// boolPtr gives each sample its own flag rather than a share of a loop
// variable's address.
func boolPtr(v bool) *bool { return &v }

func frameID(f *l9endpoints.FrameBundle) uint64 {
	if f.PointCloud != nil {
		return f.PointCloud.FrameID
	}
	return 0
}
