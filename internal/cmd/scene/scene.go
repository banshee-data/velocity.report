// Package scene provides the `velocity scene` command surface.
package scene

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sceneexport "github.com/banshee-data/velocity.report/internal/scene"
)

const usage = `Usage:
  velocity scene export   --vrlog DIR --out DIR [options]
  velocity scene vantages FILE

export   Write a static, browser-servable JSON view of a recorded VRLOG. The
         recorded VRLOG remains the source of truth; an export is a derived
         view of it.

vantages Check a scene's vantages.json before publishing it. Vantages live in
         that one file at the scene root, beside manifest.json — an export
         neither reads nor writes them.

Export options:
`

// Main routes the `scene` subcommands. args is everything after the command word.
func Main(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	switch args[0] {
	case "export":
		return exportMain(args[1:])
	case "vantages":
		return vantagesMain(args[1:])
	case "help", "--help", "-h":
		fmt.Print(usage)
		return 0
	default:
		fmt.Fprintf(os.Stderr, "unknown scene command: %q\n\n%s", args[0], usage)
		return 2
	}
}

func exportMain(args []string) int {
	var opts sceneexport.Options
	var kind string

	fs := flag.NewFlagSet("velocity-scene-export", flag.ContinueOnError)
	fs.StringVar(&opts.VRLOGPath, "vrlog", "", "Recorded VRLOG directory to read (required)")
	fs.StringVar(&opts.OutDir, "out", "", "Export directory to write (required)")
	fs.StringVar(&kind, "export", "tracks", "What to export: tracks, clip, or background")
	fs.IntVar(&opts.Stride, "stride", 1, "Retain every Nth source frame (a retention interval, not a frame rate)")
	fs.IntVar(&opts.StartFrame, "start-frame", 0, "First source frame to read")
	fs.IntVar(&opts.FrameCount, "frame-count", 0, "Source frames to read (0 = to the end)")
	fs.Float64Var(&opts.ChunkSeconds, "chunk-seconds", sceneexport.DefaultChunkSeconds, "Target span of one chunk file in seconds")
	fs.StringVar(&opts.Site, "site", "", "Site identifier recorded in the export header")
	fs.StringVar(&opts.Title, "title", "", "Human-readable scene title")
	fs.IntVar(&opts.MaxPointsPerFrame, "max-points", 0, "Cap foreground points per frame in a clip export (0 = uncapped)")
	fs.Float64Var(&opts.VoxelMetres, "voxel", sceneexport.DefaultBackgroundVoxel, "Downsampling grid for a background export, in metres")
	fs.Float64Var(&opts.BucketSeconds, "bucket-seconds", sceneexport.DefaultBucketSeconds, "Timeline summary resolution in seconds")
	fs.Usage = func() {
		fmt.Fprint(os.Stderr, usage)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  velocity scene export --vrlog run.vrlog --out web/part-000 --stride 2\n")
		fmt.Fprintf(os.Stderr, "  velocity scene export --vrlog run.vrlog --out web/clip-0 --export clip --frame-count 300\n")
	}

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	if opts.VRLOGPath == "" || opts.OutDir == "" {
		fmt.Fprintln(os.Stderr, "error: --vrlog and --out are both required")
		fs.Usage()
		return 2
	}

	switch sceneexport.Kind(kind) {
	case sceneexport.KindTracks, sceneexport.KindClip, sceneexport.KindBackground:
		opts.Kind = sceneexport.Kind(kind)
	default:
		fmt.Fprintf(os.Stderr, "error: unknown --export %q (want tracks, clip, or background)\n", kind)
		return 2
	}

	export := sceneexport.Export
	if opts.Kind == sceneexport.KindBackground {
		export = sceneexport.ExportBackground
	}
	res, err := export(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scene export failed: %v\n", err)
		return 1
	}

	fmt.Printf("wrote %s\n", filepath.Clean(opts.OutDir))
	fmt.Printf("  export        %s\n", res.Header.Export)

	// A background export is a single snapshot: frame counts, stride and
	// duration describe nothing about it.
	if res.Header.Export != sceneexport.KindBackground {
		fmt.Printf("  frames        %d retained from %d source (stride %d)\n",
			res.Header.FrameCount, res.SourceFrames, res.Header.FrameStride)
		fmt.Printf("  duration      %.1f s\n", res.Header.DurationSec)
	}
	if res.DroppedNonMonotonic > 0 {
		fmt.Printf("  dropped       %d frame(s) with non-monotonic timestamps\n", res.DroppedNonMonotonic)
	}
	if res.PointCount > 0 {
		fmt.Printf("  points        %d\n", res.PointCount)
	}
	if res.Header.Export != sceneexport.KindBackground {
		fmt.Printf("  chunks        %d\n", res.Chunks)
	}
	fmt.Printf("  bytes on disk %d (%.1f KB)\n", res.BytesOnDisk, float64(res.BytesOnDisk)/1024)
	if res.Header.Export != sceneexport.KindBackground && res.Header.DurationSec > 0 {
		fmt.Printf("  per minute    %.1f KB\n",
			float64(res.BytesOnDisk)/1024/(res.Header.DurationSec/60))
	}
	return 0
}

// vantagesMain checks a scene's vantages.json.
//
// Vantages are hand-edited between exports, so the failure this guards against
// is a typo published to a live scene: the viewer would quietly fall back to
// compass bearings and label an intersection wrongly rather than say anything.
func vantagesMain(args []string) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") {
		fmt.Fprintf(os.Stderr, "Usage: velocity scene vantages FILE\n\n"+
			"FILE is a scene's %s, at the scene root beside manifest.json.\n",
			sceneexport.VantagesFile)
		return 2
	}

	list, err := sceneexport.LoadVantages(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}

	fmt.Printf("%s: %d vantage(s)\n", filepath.Clean(args[0]), len(list))
	for _, v := range list {
		fmt.Printf("  %-14s %s  bearing %g deg, angle %g deg, zoom %g",
			v.ID, v.Label, v.AzimuthDeg, v.PolarDeg, v.Zoom)
		if v.OffsetX != 0 || v.OffsetY != 0 {
			fmt.Printf(", offset %g/%g m", v.OffsetX, v.OffsetY)
		}
		fmt.Println()
	}
	return 0
}
