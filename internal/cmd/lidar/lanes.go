//go:build pcap

package lidar

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	coredb "github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/lanes"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

const lanesUsage = `velocity lidar lanes — the lane snapshot

Usage:
  velocity lidar lanes pull --db FILE --site L16 --sensor-id ID --record-id RECORD --pose TX,TY,TZ,QW,QX,QY,QZ
                            [--gerardus URL] [--author NAME]

pull reads the site's street use from Gerardus
(GET /api/v1/sites/{l16}/geojson?layers=street_use&frame=site) and writes it
as a lane snapshot, placed by the accepted pose given, exactly as the
painter's PUT /api/lidar/sites/{l16}/lanes does: append-only, superseding the
previous one, every lane version kept with its limit's effective dates. Its
request id is the record and the features' digest, so pulling unchanged
street use again writes nothing.

Gerardus serves no registration record over HTTP, so the accepted record's
id and pose are given (the alignment pane shows them).

Flags:`

// LanesMain runs `velocity lidar lanes`.
func LanesMain(args []string) int {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Println(lanesUsage)
		return 0
	}
	switch args[0] {
	case "pull":
		return lanesPull(args[1:], http.DefaultClient)
	default:
		fmt.Fprintf(os.Stderr, "unknown lanes command %q\n\n%s\n", args[0], lanesUsage)
		return 2
	}
}

func lanesPull(args []string, client *http.Client) int {
	fs := flag.NewFlagSet("velocity-lidar-lanes-pull", flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(fs.Output(), lanesUsage)
		fs.PrintDefaults()
	}
	dbPath := fs.String("db", "", "Database to write the snapshot into")
	site := fs.String("site", "", "The site's L16 token")
	sensorID := fs.String("sensor-id", "", "The sensor the pose places")
	recordID := fs.String("record-id", "", "The accepted registration record")
	poseText := fs.String("pose", "", "The record's pose: translation (m) and rotation quaternion, TX,TY,TZ,QW,QX,QY,QZ")
	gerardus := fs.String("gerardus", envOr("GERARDUS_URL", "http://127.0.0.1:50061"), "Gerardus's address")
	author := fs.String("author", envOr("USER", "lanes pull"), "Who wrote the snapshot")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *dbPath == "" || *site == "" || *sensorID == "" || *recordID == "" || *poseText == "" {
		fmt.Fprintln(os.Stderr, "lanes pull: --db, --site, --sensor-id, --record-id and --pose are required")
		return 2
	}
	pose, err := parsePose(*poseText)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lanes pull: %v\n", err)
		return 2
	}
	u := strings.TrimRight(*gerardus, "/") + "/api/v1/sites/" + url.PathEscape(*site) + "/geojson?layers=street_use&frame=site"
	resp, err := client.Get(u)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lanes pull: %v\n", err)
		return 1
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "lanes pull: Gerardus answered %d: %.300s\n", resp.StatusCode, body)
		return 1
	}
	features, err := lanes.FromGeoJSON(body)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lanes pull: %v\n", err)
		return 1
	}
	req := lanes.Request{
		RequestID: "pull:" + *recordID + ":" + lanes.Digest(features), Writer: lanes.WriterPull,
		SensorID: *sensorID, RecordID: *recordID, Pose: pose, Author: *author,
		GerardusAddress: *gerardus, Features: features,
	}
	db, err := coredb.NewDB(*dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "lanes pull: open %s: %v\n", *dbPath, err)
		return 1
	}
	defer db.Close()
	if _, err := sqlite.NewSiteStore(db).ByToken(*site); err != nil {
		fmt.Fprintf(os.Stderr, "lanes pull: site %s: %v (locate a case there first)\n", *site, err)
		return 1
	}
	snap, repeated, err := sqlite.NewLaneSnapshotStore(db).Put(*site, req, time.Now())
	if err != nil {
		fmt.Fprintf(os.Stderr, "lanes pull: %v\n", err)
		return 1
	}
	verb := "wrote"
	if repeated {
		verb = "already held"
	}
	fmt.Printf("%s snapshot %s: %d features, %d travel lanes, %d directions\n", verb, snap.SnapshotID, len(snap.Features), len(snap.Lanes), len(snap.Directions))
	for _, d := range snap.Directions {
		limit := "no common limit"
		if d.Limit != nil {
			limit = fmt.Sprintf("%d %s (%s)", d.Limit.Value, d.Limit.Unit, d.Limit.Source)
		}
		fmt.Printf("  %s: %d lane(s), %.0f°, %s\n", d.Name, len(d.Lanes), d.BearingDeg, limit)
	}
	return 0
}

func parsePose(s string) (lanes.Pose, error) {
	parts := strings.Split(s, ",")
	if len(parts) != 7 {
		return lanes.Pose{}, fmt.Errorf("--pose needs seven numbers: TX,TY,TZ,QW,QX,QY,QZ")
	}
	var v [7]float64
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return lanes.Pose{}, fmt.Errorf("--pose: %q is not a number", p)
		}
		v[i] = f
	}
	return lanes.Pose{Translation: [3]float64{v[0], v[1], v[2]}, Rotation: [4]float64{v[3], v[4], v[5], v[6]}}, nil
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
