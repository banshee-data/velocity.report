//go:build pcap
// +build pcap

package lidar

import (
	"flag"
	"fmt"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
)

// AnnotationExportMain cuts a frozen annotation pack from a VRLOG. args is the
// argument slice after the command word. It returns the exit code.
func AnnotationExportMain(args []string) int {
	fs := flag.NewFlagSet("velocity-lidar-annotation-export", flag.ContinueOnError)
	vrlog := fs.String("vrlog", "", "Source VRLOG directory (required; must have been recorded with points)")
	outDir := fs.String("output", "", "Output pack directory (required; must not exist)")
	startNs := fs.Int64("start-ns", 0, "Excerpt start, capture nanoseconds (0 = recording start)")
	endNs := fs.Int64("end-ns", 0, "Excerpt end, capture nanoseconds (0 = recording end)")
	maxSamples := fs.Int("max-samples", 200, "Cap on exported frames (0 = no cap)")
	coverage := fs.String("coverage", "", "What the source could see: full, foreground_only, or decimated (required)")
	coverageNote := fs.String("coverage-note", "", "Free text recording and export filters separately")

	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: velocity lidar annotation-export --vrlog DIR --output DIR --coverage KIND\n\n")
		fmt.Fprintf(os.Stderr,
			`Cut a frozen excerpt of a recording into an annotation pack: an immutable
point domain that reviewed masks can reference by position.

The pack exists so that a reference identity is not a tracker output. Track IDs
split, merge and change between runs, so scoring a tracker against its own IDs
measures the population as much as the estimator. Object identities in a pack
belong to the pack, and a predicted split does not split the reference.

The source recording is never modified. A pack whose point domain changes is a
new pack, not a revision, and an existing output directory is refused.

Coverage must be stated: a recording holding only foreground points looks
exactly like a sparse full-scene one, and the difference cannot be recovered
later. A mask over a foreground-only recording is a mask over recorded
foreground, not scene segmentation.

Options:
`)
		fs.PrintDefaults()
		fmt.Fprintf(os.Stderr, `
Example:
  # Record 20 s with points, then cut the first 200 frames for annotation
  velocity lidar pcap-replay --pcap capture.pcap --output ./runs/annot \
      --start-seconds 35 --duration-seconds 20 --warmup-seconds 35 --include-points
  velocity lidar annotation-export --vrlog ./runs/annot --output ./packs/site-a \
      --coverage full --coverage-note "full scene, no decimation"
`)
	}

	if err := fs.Parse(args); err != nil {
		// -h is a request that succeeded, not a usage error.
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}
	for name, v := range map[string]string{"--vrlog": *vrlog, "--output": *outDir, "--coverage": *coverage} {
		if v == "" {
			fmt.Fprintf(os.Stderr, "error: %s is required\n", name)
			fs.Usage()
			return 2
		}
	}

	pack, err := annotation.Export(annotation.ExportConfig{
		VRLOGPath:    *vrlog,
		OutDir:       *outDir,
		StartNs:      *startNs,
		EndNs:        *endNs,
		MaxSamples:   *maxSamples,
		Coverage:     annotation.CaptureCoverage(*coverage),
		CoverageNote: *coverageNote,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "annotation-export: %v\n", err)
		return 1
	}

	m := pack.Manifest
	fmt.Printf("pack: %s\n", pack.Dir)
	fmt.Printf("dataset %s, digest %s\n", m.DatasetID, m.PackDigest)
	fmt.Printf("%d samples, %d points, coverage %s\n", m.SampleCount, m.PointCount, m.Coverage)
	fmt.Printf("attributes: intensity=%t classification=%t\n", m.HasIntensity, m.HasClassification)

	c := m.Completeness
	fmt.Printf("window %d..%d ns\n", c.ActualStartNs, c.ActualEndNs)
	if c.FramesWithoutPoints > 0 {
		fmt.Printf("skipped %d frames carrying no point cloud\n", c.FramesWithoutPoints)
	}
	if c.DuplicateTimestamps > 0 {
		fmt.Printf("%d duplicate timestamps (legal; sample ids stay unambiguous)\n", c.DuplicateTimestamps)
	}
	if c.MaxTimestampGapNs > 0 {
		fmt.Printf("largest timestamp gap %.3f s\n", float64(c.MaxTimestampGapNs)/1e9)
	}
	return 0
}
