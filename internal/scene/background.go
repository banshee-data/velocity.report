package scene

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
)

// DefaultBackgroundVoxel is the downsampling grid for the static scene.
//
// Coarser than a clip on purpose: the background exists to tell a viewer where
// the kerb and the buildings are, not to be measured. A settled snapshot from a
// Pandar40P carries about 69,000 points, which is more than a browser needs to
// convey a street.
const DefaultBackgroundVoxel = 0.25

// ExportBackground writes the static scene: one settled background snapshot,
// voxel-downsampled, as a single gzipped file.
//
// It is written as one file rather than chunks because a viewer fetches it once
// and keeps it for the whole session; there is nothing to seek through.
func ExportBackground(opts Options) (*Result, error) {
	opts.applyDefaults()
	voxel := opts.VoxelMetres
	if voxel <= 0 {
		voxel = DefaultBackgroundVoxel
	}

	rep, err := recorder.NewReplayer(opts.VRLOGPath)
	if err != nil {
		return nil, fmt.Errorf("open vrlog: %w", err)
	}
	defer func() { _ = rep.Close() }()

	snapshot, framesRead, err := firstSettledBackground(rep)
	if err != nil {
		return nil, err
	}

	points := voxelDownsample(snapshot, voxel)
	if len(points) == 0 {
		return nil, fmt.Errorf("%s: background snapshot has no usable points", opts.VRLOGPath)
	}

	export := BackgroundExport{
		Version:     FormatVersion,
		Site:        opts.Site,
		Title:       opts.Title,
		VoxelMetres: voxel,
		PointCount:  len(points),
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Points:      points,
	}
	fillBounds(&export)
	if sum, err := sourceFingerprint(opts.VRLOGPath); err == nil {
		export.SourceVRLOGSHA256 = sum
	}

	if err := os.MkdirAll(opts.OutDir, 0o755); err != nil {
		return nil, fmt.Errorf("create export dir: %w", err)
	}
	if err := writeGzipJSON(filepath.Join(opts.OutDir, "background.json.gz"), export); err != nil {
		return nil, err
	}

	return &Result{
		Header: Header{
			Version:     FormatVersion,
			Export:      KindBackground,
			Site:        opts.Site,
			Title:       opts.Title,
			FrameCount:  1,
			GeneratedAt: export.GeneratedAt,
		},
		Chunks:       1,
		BytesOnDisk:  dirBytes(opts.OutDir),
		SourceFrames: framesRead,
		PointCount:   len(points),
	}, nil
}

// firstSettledBackground returns the first background snapshot whose grid has
// finished settling. An unsettled snapshot still has moving objects burned into
// it, which is exactly what the background is meant to exclude.
func firstSettledBackground(rep *recorder.Replayer) (*l9endpoints.BackgroundSnapshot, int, error) {
	total := int(rep.TotalFrames())
	var fallback *l9endpoints.BackgroundSnapshot

	for i := 0; i < total; i++ {
		fb, err := rep.ReadFrame()
		if err != nil {
			break
		}
		if fb.FrameType != l9endpoints.FrameTypeBackground || fb.Background == nil {
			continue
		}
		if len(fb.Background.X) == 0 {
			continue
		}
		if fb.Background.GridMetadata.SettlingComplete {
			return fb.Background, i + 1, nil
		}
		if fallback == nil {
			fallback = fb.Background
		}
	}
	if fallback != nil {
		return fallback, total, nil
	}
	return nil, total, fmt.Errorf("no background snapshot with points found")
}

// voxelDownsample keeps one point per occupied cell, which preserves the shape
// of the scene while cutting the count by an order of magnitude.
func voxelDownsample(bg *l9endpoints.BackgroundSnapshot, voxel float64) [][4]float64 {
	n := len(bg.X)
	if n > len(bg.Y) {
		n = len(bg.Y)
	}
	if n > len(bg.Z) {
		n = len(bg.Z)
	}

	type key struct{ x, y, z int64 }
	seen := make(map[key]struct{}, n/4)
	out := make([][4]float64, 0, n/4)

	for i := 0; i < n; i++ {
		x, y, z := float64(bg.X[i]), float64(bg.Y[i]), float64(bg.Z[i])
		if math.IsNaN(x) || math.IsNaN(y) || math.IsNaN(z) {
			continue
		}
		k := key{
			int64(math.Floor(x / voxel)),
			int64(math.Floor(y / voxel)),
			int64(math.Floor(z / voxel)),
		}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}

		// Confidence is how often the cell was seen; it stands in for intensity
		// so a viewer can fade the least certain returns.
		var conf float64
		if i < len(bg.Confidence) {
			conf = float64(bg.Confidence[i])
		}
		out = append(out, [4]float64{
			math.Round(x*100) / 100,
			math.Round(y*100) / 100,
			math.Round(z*100) / 100,
			conf,
		})
	}
	return out
}

func fillBounds(e *BackgroundExport) {
	if len(e.Points) == 0 {
		return
	}
	e.MinX, e.MaxX = e.Points[0][0], e.Points[0][0]
	e.MinY, e.MaxY = e.Points[0][1], e.Points[0][1]
	e.MinZ, e.MaxZ = e.Points[0][2], e.Points[0][2]
	zs := make([]float64, 0, len(e.Points))
	for _, p := range e.Points {
		e.MinX = math.Min(e.MinX, p[0])
		e.MaxX = math.Max(e.MaxX, p[0])
		e.MinY = math.Min(e.MinY, p[1])
		e.MaxY = math.Max(e.MaxY, p[1])
		e.MinZ = math.Min(e.MinZ, p[2])
		e.MaxZ = math.Max(e.MaxZ, p[2])
		zs = append(zs, p[2])
	}
	// The road is the low end of the distribution, but not its minimum: a
	// single return down a stairwell should not become the ground plane.
	sort.Float64s(zs)
	e.GroundZ = zs[int(float64(len(zs))*0.02)]
}

func writeGzipJSON(path string, v any) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	defer func() { _ = f.Close() }()

	gz := gzip.NewWriter(f)
	if err := json.NewEncoder(gz).Encode(v); err != nil {
		_ = gz.Close()
		return fmt.Errorf("encode %s: %w", filepath.Base(path), err)
	}
	if err := gz.Close(); err != nil {
		return fmt.Errorf("close %s: %w", filepath.Base(path), err)
	}
	return nil
}
