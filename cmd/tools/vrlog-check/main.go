// Command vrlog-check reports what a VRLOG recording contains, so a fresh set
// can be checked before the old ones are deleted.
//
// The question it exists to answer is where the first background frame sits. A
// recording whose background arrives at frame 2 renders a scene immediately;
// one whose background arrives at frame 116 draws foreground over nothing until
// it does, which is what made some replays look broken.
//
// A VRLOG 1.x observation container has no background frames to find. It is
// recognised by its root and fully verified instead (every chunk digest,
// record checksum and index, and the semantic digests its writer sealed), so
// one command can check a directory holding both kinds.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints"
	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/vrlog"
)

func main() {
	limit := flag.Int("limit", 2000, "frames to scan per recording")
	flag.Parse()

	dirs := flag.Args()
	if len(dirs) == 0 {
		fmt.Fprintln(os.Stderr, "usage: vrlog-check [-limit N] <vrlog-dir>...")
		os.Exit(2)
	}
	sort.Strings(dirs)

	fmt.Printf("%-38s %-6s %8s %8s %9s %8s %s\n",
		"RUN", "ENC", "FRAMES", "FIRSTBG", "BGFRAMES", "EMPTY", "VERDICT")

	bad := 0
	for _, dir := range dirs {
		if err := report(dir, *limit); err != nil {
			fmt.Printf("%-38s %s\n", filepath.Base(dir), err)
			bad++
		}
	}
	if bad > 0 {
		os.Exit(1)
	}
}

func report(dir string, limit int) error {
	if vrlog.IsContainer(dir) {
		return reportObservations(dir)
	}
	rep, err := recorder.NewReplayer(dir)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = rep.Close() }()

	var (
		frames, backgrounds, empties int
		firstBG                      = -1
		firstBGPoints                int
	)
	for frames < limit {
		frame, err := rep.ReadFrame()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read frame %d: %w", frames, err)
		}
		switch frame.FrameType {
		case l9endpoints.FrameTypeBackground:
			backgrounds++
			if firstBG < 0 {
				firstBG = frames
				if frame.Background != nil {
					firstBGPoints = len(frame.Background.X)
				}
			}
		case l9endpoints.FrameTypeEmpty:
			empties++
		}
		frames++
	}

	verdict := "ok"
	switch {
	case firstBG < 0:
		verdict = "NO BACKGROUND"
	case firstBG > 10:
		verdict = fmt.Sprintf("late background (frame %d)", firstBG)
	case firstBGPoints == 0:
		verdict = "first background is empty"
	}
	if frames == 0 {
		verdict = "NO FRAMES"
	}

	fmt.Printf("%-38s %-6s %8d %8d %9d %8d %s\n",
		filepath.Base(dir), rep.FrameEncoding(), frames, firstBG, backgrounds, empties, verdict)
	return nil
}

// reportObservations verifies a VRLOG 1.x observation container. It ignores
// the frame limit: a container is only sound if all of it is.
func reportObservations(dir string) error {
	r, err := vrlog.Open(dir, vrlog.Options{})
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer func() { _ = r.Close() }()
	report, err := r.Verify()
	if err != nil {
		return fmt.Errorf("verify: %w", err)
	}
	verdict := fmt.Sprintf("ok (%s, %d gaps)", r.Profile().Name, report.Gaps)
	if !report.Closed {
		verdict = "UNCLOSED: no summary, the capture may have been interrupted"
	}
	fmt.Printf("%-38s %-6s %8d %8s %9s %8s %s\n",
		filepath.Base(dir), fmt.Sprintf("obs-%d", vrlog.FormatMajor), report.Frames, "-", "-", "-", verdict)
	return nil
}
