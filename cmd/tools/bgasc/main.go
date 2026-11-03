// bgasc exports a settled background snapshot from the sensor database as an
// ASC point file in the sensor frame, the form Gerardus's `register --asc`
// reads. It picks the snapshot by id, or the newest one for a sensor taken
// within a time window, so a replay's background can be named by the run's
// created_at and completed_at. With -format pcd it writes the same points, in the same
// order and with the same values, as an ASCII PCD file for an offline PCL oracle; the point's
// ordinal in the file is its ordinal in the ASC export.
//
//	go run ./cmd/tools/bgasc -db sensor_data.db -sensor hesai-pandar40p \
//	    -after 1792896896000000000 -before 1792897300000000000 -out site.asc
//	go run ./cmd/tools/bgasc -db sensor_data.db -snapshot 12 -format pcd -out site.pcd
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
)

func main() {
	dbPath := flag.String("db", "sensor_data.db", "sensor database")
	sensor := flag.String("sensor", "hesai-pandar40p", "sensor id")
	id := flag.Int64("snapshot", 0, "snapshot id; 0 picks by time window")
	after := flag.Int64("after", 0, "earliest taken_unix_nanos (inclusive)")
	before := flag.Int64("before", 0, "latest taken_unix_nanos (inclusive); 0 = now")
	out := flag.String("out", "", "where to write the ASC file (required)")
	limit := flag.Int("limit", 200, "recent snapshots searched when picking by time")
	format := flag.String("format", "asc", "output format: asc (Gerardus's register --asc) or pcd (ASCII PCD, the same points in the same order)")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
		os.Exit(2)
	}
	if *format != "asc" && *format != "pcd" {
		fmt.Fprintf(os.Stderr, "-format must be asc or pcd, not %q\n", *format)
		os.Exit(2)
	}
	d, err := db.OpenDB(*dbPath)
	if err != nil {
		fail("open %s: %v", *dbPath, err)
	}
	defer d.Close()

	var snap *l3grid.BgSnapshot
	if *id > 0 {
		snap, err = d.GetBgSnapshotByID(*id)
		if err != nil {
			fail("snapshot %d: %v", *id, err)
		}
	} else {
		recent, err := d.ListRecentBgSnapshots(*sensor, *limit)
		if err != nil {
			fail("list snapshots: %v", err)
		}
		for _, s := range recent {
			if s.TakenUnixNanos < *after || (*before > 0 && s.TakenUnixNanos > *before) {
				continue
			}
			if snap == nil || s.TakenUnixNanos > snap.TakenUnixNanos {
				snap = s
			}
		}
	}
	if snap == nil {
		fail("no snapshot matched")
	}
	path, err := l3grid.ExportBgSnapshotToASC(snap, nil)
	if err != nil {
		fail("export: %v", err)
	}
	write := copyFile
	if *format == "pcd" {
		write = ascToPCD
	}
	if err := write(path, *out); err != nil {
		fail("write %s to %s: %v", path, *out, err)
	}
	os.Remove(path)
	fmt.Printf("snapshot %d taken %d reason %s -> %s\n", snap.SnapshotID, snap.TakenUnixNanos, snap.SnapshotReason, *out)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	o, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(o, in); err != nil {
		o.Close()
		return err
	}
	return o.Close()
}

// pcdHeader is the header of an unorganised ASCII PCD v0.7 file holding the six columns of
// bgasc's ASC export: X Y Z Intensity AverageRangeMeters TimesSeenCount.
const pcdHeader = `# .PCD v0.7 - settled background exported by bgasc
VERSION 0.7
FIELDS x y z intensity average_range times_seen
SIZE 4 4 4 4 4 4
TYPE F F F F F U
COUNT 1 1 1 1 1 1
WIDTH %d
HEIGHT 1
VIEWPOINT 0 0 0 1 0 0 0
POINTS %d
DATA ascii
`

// ascToPCD rewrites an ASC export as ASCII PCD. Each data line is copied verbatim, so values
// and order are the ASC file's; comment and blank lines are dropped, and a line without the
// six columns is refused rather than written short.
func ascToPCD(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	var lines []string
	scanner := bufio.NewScanner(in)
	for n := 1; scanner.Scan(); n++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if len(strings.Fields(line)) != 6 {
			return fmt.Errorf("line %d: want 6 columns, got %q", n, line)
		}
		lines = append(lines, line)
	}
	if err := scanner.Err(); err != nil {
		return err
	}
	if len(lines) == 0 {
		return fmt.Errorf("no points in %s", src)
	}
	o, err := os.Create(dst)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(o)
	fmt.Fprintf(w, pcdHeader, len(lines), len(lines))
	for _, line := range lines {
		w.WriteString(line)
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		o.Close()
		return err
	}
	return o.Close()
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
