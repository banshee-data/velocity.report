// Command vrlog-check reports what a VRLOG recording contains, so a fresh set
// can be checked before the old ones are deleted.
//
// The question it exists to answer is where the first background frame sits. A
// recording whose background arrives at frame 2 renders a scene immediately;
// one whose background arrives at frame 116 draws foreground over nothing until
// it does, which is what made some replays look broken.
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
