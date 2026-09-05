// Package scene provides the `velocity scene` command surface.
package scene

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	sceneexport "github.com/banshee-data/velocity.report/internal/scene"
)

const usage = `Usage: velocity scene export --vrlog DIR --out DIR [options]

Write a static, browser-servable JSON view of a recorded VRLOG. The recorded
VRLOG remains the source of truth; an export is a derived view of it.

Options:
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
	fs.IntVar(&opts.ChunkFrames, "chunk-frames", sceneexport.DefaultChunkFrames, "Retained frames per chunk file")
	fs.StringVar(&opts.Site, "site", "", "Site identifier recorded in the export header")
	fs.StringVar(&opts.Title, "title", "", "Human-readable scene title")
	fs.IntVar(&opts.MaxPointsPerFrame, "max-points", 0, "Cap foreground points per frame in a clip export (0 = uncapped)")

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

	res, err := sceneexport.Export(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "scene export failed: %v\n", err)
		return 1
	}

	fmt.Printf("wrote %s\n", filepath.Clean(opts.OutDir))
	fmt.Printf("  export        %s\n", res.Header.Export)
	fmt.Printf("  frames        %d retained from %d source (stride %d)\n",
		res.Header.FrameCount, res.SourceFrames, res.Header.FrameStride)
	fmt.Printf("  duration      %.1f s\n", res.Header.DurationSec)
	if res.DroppedNonMonotonic > 0 {
		fmt.Printf("  dropped       %d frame(s) with non-monotonic timestamps\n", res.DroppedNonMonotonic)
	}
	fmt.Printf("  chunks        %d\n", res.Chunks)
	fmt.Printf("  bytes on disk %d (%.1f KB)\n", res.BytesOnDisk, float64(res.BytesOnDisk)/1024)
	if res.Header.FrameCount > 0 {
		fmt.Printf("  per minute    %.1f KB\n",
			float64(res.BytesOnDisk)/1024/(res.Header.DurationSec/60))
	}
	return 0
}
