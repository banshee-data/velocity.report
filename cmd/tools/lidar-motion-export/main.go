// lidar-motion-export writes a motion segment of an ordered capture sequence
// as sensor-frame LAS 1.4 files with per-point time and a run.json manifest,
// for offline odometry and prior registration (the route SLAM spike's S0).
//
// Build it through make so the manifest carries the build stamp:
//
//	make build-lidar-motion-export
//	/tmp/lidar-motion-export -pcap a.pcap -pcap b.pcap -out-dir /local/motion/<drive> \
//	    -from 2026-09-02T11:33:00-07:00 -to 2026-09-02T11:53:00-07:00 \
//	    -start-site lombard-laguna -end-site columbus-north-point
//
// Only L1 decode and L2 rotation assembly run: no background model, no
// perception, so every rotation is exported from the first packet and no
// tuning config is involved. The ordered capture list is graded at each join
// exactly as replay grades it, and a broken join is refused.
//
// Outputs are raw evidence: write them outside the checkout and never commit them.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/network"
	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/motionexport"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/version"
)

type listFlag []string

func (l *listFlag) String() string     { return strings.Join(*l, ",") }
func (l *listFlag) Set(v string) error { *l = append(*l, v); return nil }

func optionalFloat(name, v string) (*float64, error) {
	if v == "" {
		return nil, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return &f, nil
}

func optionalTime(name, v string) (time.Time, error) {
	if v == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, v)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: %w", name, err)
	}
	return t, nil
}

type options struct {
	pcaps                         listFlag
	outDir                        string
	from, to, timeSource          string
	startSite, endSite            string
	platform, height, roll, pitch string
	mountNotes, sensorID          string
	udpPort, stride               int
	startS, durationS             float64
	splitS, maxRange              float64
	includeIncomplete             bool
}

func parseFlags(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("lidar-motion-export", flag.ContinueOnError)
	fs.Var(&o.pcaps, "pcap", "capture file, in order; repeat for a rotated sequence")
	fs.StringVar(&o.outDir, "out-dir", "", "new or empty local directory for the LAS files and run.json")
	fs.StringVar(&o.from, "from", "", "export rotations starting at or after this RFC 3339 time")
	fs.StringVar(&o.to, "to", "", "export rotations starting at or before this RFC 3339 time")
	fs.StringVar(&o.timeSource, "time-source", motionexport.TimeAzimuthClock, "point time: azimuth_clock_v1 or packet_firetime")
	fs.StringVar(&o.startSite, "start-site", "", "site the segment leaves (site-index id)")
	fs.StringVar(&o.endSite, "end-site", "", "site the segment reaches (site-index id)")
	fs.StringVar(&o.platform, "platform", "", "mount platform as recorded, e.g. car-roof")
	fs.StringVar(&o.height, "mount-height-m", "", "sensor height above the road, if recorded")
	fs.StringVar(&o.roll, "mount-roll-deg", "", "mount roll, if recorded")
	fs.StringVar(&o.pitch, "mount-pitch-deg", "", "mount pitch, if recorded")
	fs.StringVar(&o.mountNotes, "mount-notes", "", "free-text mount record")
	fs.StringVar(&o.sensorID, "sensor-id", "hesai-pandar40p", "sensor id")
	fs.IntVar(&o.udpPort, "udp-port", 2369, "LiDAR UDP port in the capture")
	fs.IntVar(&o.stride, "azimuth-stride", 1, "keep every n-th firing block")
	fs.Float64Var(&o.startS, "start-seconds", 0, "replay window start, seconds into the sequence")
	fs.Float64Var(&o.durationS, "duration-seconds", 0, "replay window length; 0 replays to the end")
	fs.Float64Var(&o.splitS, "split-seconds", 60, "capture seconds per LAS file")
	fs.Float64Var(&o.maxRange, "max-range-m", 0, "drop returns beyond this range; 0 keeps all")
	fs.BoolVar(&o.includeIncomplete, "include-incomplete", false, "also export rotations marked incomplete")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if len(o.pcaps) == 0 || o.outDir == "" {
		return o, errors.New("pcap and out-dir are required")
	}
	return o, nil
}

func run(args []string) error {
	o, err := parseFlags(args)
	if err != nil {
		return err
	}
	from, err := optionalTime("from", o.from)
	if err != nil {
		return err
	}
	to, err := optionalTime("to", o.to)
	if err != nil {
		return err
	}
	var m motionexport.Mount
	m.Platform, m.Notes = o.platform, o.mountNotes
	if m.HeightM, err = optionalFloat("mount-height-m", o.height); err != nil {
		return err
	}
	if m.RollDeg, err = optionalFloat("mount-roll-deg", o.roll); err != nil {
		return err
	}
	if m.PitchDeg, err = optionalFloat("mount-pitch-deg", o.pitch); err != nil {
		return err
	}
	man := motionexport.Manifest{
		Build:   map[string]string{"version": version.Version, "git_sha": version.GitSHA, "build_time": version.BuildTime},
		Sensor:  map[string]string{"model": "Hesai Pandar40P", "sensor_id": o.sensorID, "udp_port": strconv.Itoa(o.udpPort)},
		Segment: motionexport.Segment{StartSite: o.startSite, EndSite: o.endSite},
		Mount:   m,
	}
	for _, p := range o.pcaps {
		d, err := motionexport.Digest(p)
		if err != nil {
			return err
		}
		man.Captures = append(man.Captures, d)
	}
	sequence, err := replayeval.PrepareCaptureSequence(o.pcaps, o.udpPort)
	if err != nil {
		return err
	}
	duration := o.durationS
	if duration == 0 {
		duration = -1 // capseq reads -1 as "to the end of the sequence"
	}
	steps, err := sequence.Plan(o.startS, duration)
	if err != nil {
		return err
	}
	parserCfg, err := parse.LoadPandar40PConfig()
	if err != nil {
		return err
	}

	ex, err := motionexport.NewExporter(motionexport.Config{OutDir: o.outDir, From: from, To: to, TimeSource: o.timeSource,
		SplitSeconds: o.splitS, AzimuthStride: o.stride, MaxRangeM: o.maxRange, IncludeIncomplete: o.includeIncomplete,
		Software: "velocity " + version.Version}, man)
	if err != nil {
		return err
	}
	var mu sync.Mutex
	var tapErr error
	// Offline frame boundaries: no wall-clock timeout, flush on Close, and
	// back-pressure rather than dropping a rotation when the writer is slow.
	fb := l2frames.NewFrameBuilder(l2frames.FrameBuilderConfig{SensorID: o.sensorID, FrameChCapacity: 32,
		BufferTimeout: time.Duration(math.MaxInt64), CleanupInterval: time.Duration(math.MaxInt64),
		FrameCallback: func(f *l2frames.LiDARFrame) {
			mu.Lock()
			defer mu.Unlock()
			if tapErr == nil {
				tapErr = ex.AddFrame(f)
			}
		}})
	fb.SetBlockOnFrameChannel(true)
	_, err = network.ReadPCAPSequence(context.Background(), steps, network.SequenceReplayConfig{
		UDPPort: o.udpPort, Parser: parse.NewPandar40PParser(*parserCfg), FrameBuilder: fb})
	fb.Close()
	mu.Lock()
	defer mu.Unlock()
	out, cerr := ex.Close()
	switch {
	case err != nil:
		return err
	case tapErr != nil:
		return tapErr
	case cerr != nil:
		return cerr
	}
	fmt.Printf("exported rotations=%d files=%d median_period=%.6fs median_clock_resid=%.1fus p95=%.1fus fit_failed=%d period_outliers=%d\n",
		out.Frames.Exported, len(out.Files), out.Time.MedianPeriodS, out.Time.MedianResidUS, out.Time.P95ResidUS, out.Time.ClockFitFailed, out.Time.PeriodOutliers)
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
