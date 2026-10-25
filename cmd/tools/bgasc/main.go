// bgasc exports a settled background snapshot from the sensor database as an
// ASC point file in the sensor frame, the form Gerardus's `register --asc`
// reads. It picks the snapshot by id, or the newest one for a sensor taken
// within a time window, so a replay's background can be named by the run's
// created_at and completed_at.
//
//	go run ./cmd/tools/bgasc -db sensor_data.db -sensor hesai-pandar40p \
//	    -after 1792896896000000000 -before 1792897300000000000 -out site.asc
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

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
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "-out is required")
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
	if err := copyFile(path, *out); err != nil {
		fail("copy %s to %s: %v", path, *out, err)
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

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
