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
	)

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
		if cfg.StartNs != 0 && ts < cfg.StartNs {
			continue
		}
		if cfg.EndNs != 0 && ts > cfg.EndNs {
			break
		}

		pc := frame.PointCloud
		if pc == nil || len(pc.X) == 0 {
			comp.FramesWithoutPoints++
			continue
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

	if err := WritePack(cfg.OutDir, m, samples, blocks); err != nil {
		return nil, err
	}
	return OpenPack(cfg.OutDir)
}

func frameID(f *l9endpoints.FrameBundle) uint64 {
	if f.PointCloud != nil {
		return f.PointCloud.FrameID
	}
	return 0
}
