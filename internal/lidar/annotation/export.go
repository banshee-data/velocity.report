package annotation

import (
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

// Export cuts a frozen pack from a VRLOG.
//
// It reads the source and writes a new directory; the recording is never
// modified. Frames without a point cloud are counted and skipped rather than
// written as empty samples, because an empty sample is indistinguishable from
// a frame where everything was occluded.
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
		if len(pc.Intensity) > 0 {
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

		if len(samples) == 0 {
			keepPending()
		}
		samples = append(samples, Sample{
			SourceOrdinal: ordinal - 1,
			SourceFrameID: frameID(frame),
			TimestampNs:   ts,
			SensorID:      header.SensorID,
			PointCount:    len(pc.X),
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

	if err := WritePackWithBackground(cfg.OutDir, m, samples, blocks, backgrounds, backgroundBlocks); err != nil {
		return nil, err
	}
	return OpenPack(cfg.OutDir)
}

// classForeground is the recorder's per-point class for a foreground return
// (visualiser.proto: background=0, foreground=1, ground=2).
const classForeground = 1

func frameID(f *l9endpoints.FrameBundle) uint64 {
	if f.PointCloud != nil {
		return f.PointCloud.FrameID
	}
	return 0
}
