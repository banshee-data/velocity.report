// pcl-frame-export exports one complete sensor frame for offline PCL compatibility checks.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/version"
)

// writeFrame retains the nearest return per native channel/azimuth cell. Ordinals
// index the original frame; missing cells have ordinal -1 and NaN geometry.
func writeFrame(w io.Writer, points []l2frames.Point, bins int) error {
	if bins <= 0 {
		return fmt.Errorf("positive azimuth bin count required")
	}
	cells := make([]int, 40*bins)
	for i := range cells {
		cells[i] = -1
	}
	for i, p := range points {
		if p.Channel < 1 || p.Channel > 40 || p.Distance <= 0 || math.IsNaN(p.Distance) || math.IsInf(p.Distance, 0) || math.IsNaN(p.Azimuth) || math.IsInf(p.Azimuth, 0) {
			continue
		}
		az := math.Mod(p.Azimuth, 360)
		if az < 0 {
			az += 360
		}
		j := (p.Channel-1)*bins + int(az/360*float64(bins))
		if cells[j] < 0 || p.Distance < points[cells[j]].Distance {
			cells[j] = i
		}
	}
	b := bufio.NewWriter(w)
	fmt.Fprintf(b, "# build_version %s build_git_sha %s build_time %s\n", version.Version, version.GitSHA, version.BuildTime)
	fmt.Fprintf(b, "# PCD v0.7 sensor XYZ; nearest return per native ring/bin\nVERSION 0.7\nFIELDS x y z range ring azimuth elevation source_ordinal\nSIZE 4 4 4 4 4 4 4 4\nTYPE F F F F I F F I\nCOUNT 1 1 1 1 1 1 1 1\nWIDTH %d\nHEIGHT 40\nPOINTS %d\nDATA ascii\n", bins, len(cells))
	for j, i := range cells {
		if i < 0 {
			fmt.Fprintf(b, "nan nan nan nan %d nan nan -1\n", j/bins+1)
			continue
		}
		p := points[i]
		fmt.Fprintf(b, "%.9g %.9g %.9g %.9g %d %.9g %.9g %d\n", p.X, p.Y, p.Z, p.Distance, p.Channel, p.Azimuth, p.Elevation, i)
	}
	return b.Flush()
}

func run() error {
	pcap := flag.String("pcap", "", "source capture")
	out := flag.String("out", "", "new output PCD file")
	replay := flag.String("replay-output", "", "new local replay output directory")
	tuning := flag.String("tuning", "", "checked-in tuning config")
	flag.Parse()
	if *pcap == "" || *out == "" || *replay == "" || *tuning == "" {
		return fmt.Errorf("pcap, out, replay-output and tuning are required")
	}
	var exportErr error
	saved := false
	_, err := replayeval.Run(replayeval.Config{PCAPFile: *pcap, OutDir: *replay, TuningFile: *tuning, SensorID: "hesai-pandar40p", UDPPort: 2369, StartSeconds: 20, DurationSeconds: 0.5, WarmupSeconds: 20, FrameTap: func(f replayeval.FrameTapFrame) {
		if saved || exportErr != nil || !f.Scored || !f.Frame.SpinComplete {
			return
		}
		dst, e := os.OpenFile(*out, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			exportErr = e
			return
		}
		exportErr = writeFrame(dst, f.Frame.Points, f.AzimuthBins)
		if e = dst.Close(); exportErr == nil {
			exportErr = e
		}
		saved = exportErr == nil
		fmt.Printf("exported frame=%s points=%d bins=%d complete=%t\n", f.At.UTC().Format("2006-01-02T15:04:05.999999999Z"), len(f.Frame.Points), f.AzimuthBins, f.Frame.SpinComplete)
	}})
	if err != nil {
		return err
	}
	if exportErr != nil {
		return exportErr
	}
	if !saved {
		return fmt.Errorf("no complete scored frame")
	}
	return nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
