//go:build pcap

package lidar

import (
	"encoding/json"
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
  velocity lidar lanes score --site L16 --reference-tag KEY=VALUE [--gerardus URL] [--json]

pull reads the site's street use from Gerardus
(GET /api/v1/sites/{l16}/geojson?layers=street_use&frame=site) and writes it
as a lane snapshot, placed by the accepted pose given, exactly as the
painter's PUT /api/lidar/sites/{l16}/lanes does: append-only, superseding the
previous one, every lane version kept with its limit's effective dates. Its
request id is the record and the features' digest, so pulling unchanged
street use again writes nothing.

Gerardus serves no registration record over HTTP, so the accepted record's
id and pose are given (the alignment pane shows them).

score is experiment lane0002's (data/experiments/lane/lane0002.md): it reads
the site's lane proposals and the travel lanes drawn by hand with the
reference tag, both from Gerardus in the site frame, pairs each proposal with
the drawn lane it runs nearest, and prints the lateral RMSE, mean offset and
overlap of each pair, the counts per direction, and the rule's four clauses.
It reads no track.

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
	case "score":
		return lanesScore(args[1:], http.DefaultClient)
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

func lanesScore(args []string, client *http.Client) int {
	fs := flag.NewFlagSet("velocity-lidar-lanes-score", flag.ContinueOnError)
	site := fs.String("site", "", "The site's L16 token")
	tag := fs.String("reference-tag", "reference=lane0002", "The tag the drawn lanes carry, KEY=VALUE")
	gerardus := fs.String("gerardus", envOr("GERARDUS_URL", "http://127.0.0.1:50061"), "Gerardus's address")
	asJSON := fs.Bool("json", false, "Print the score as JSON")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	key, value, ok := strings.Cut(*tag, "=")
	if *site == "" || !ok || key == "" {
		fmt.Fprintln(os.Stderr, "lanes score: --site and --reference-tag KEY=VALUE are required")
		return 2
	}
	read := func(layer string) ([]lanes.Feature, error) {
		u := strings.TrimRight(*gerardus, "/") + "/api/v1/sites/" + url.PathEscape(*site) + "/geojson?layers=" + layer + "&frame=site"
		resp, err := client.Get(u)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		if err != nil {
			return nil, err
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("Gerardus answered %d for %s: %.200s", resp.StatusCode, layer, body)
		}
		return lanes.FromGeoJSONLayer(body, layer)
	}
	proposalFeatures, err := read("lane_proposals")
	if err != nil {
		fmt.Fprintf(os.Stderr, "lanes score: %v\n", err)
		return 1
	}
	streetUse, err := read("street_use")
	if err != nil {
		fmt.Fprintf(os.Stderr, "lanes score: %v\n", err)
		return 1
	}
	drawn := lanes.ScoreFeatures(streetUse, key, value, false)
	proposals := lanes.ScoreFeatures(proposalFeatures, "", "", true)
	score := lanes.ScoreProposals(proposals, drawn)
	if *asJSON {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(score)
		return 0
	}
	fmt.Printf("site %s: %d drawn lanes (%s), %d proposals\n", *site, len(drawn), *tag, len(proposals))
	for _, d := range score.Directions {
		fmt.Printf("  %-28s drawn %d, high %d, medium %d\n", d.Direction, d.Drawn, d.High, d.Medium)
	}
	for _, p := range score.Pairs {
		fmt.Printf("  %s (%s) -> %s: RMSE %.3f m, mean %+.3f m, overlap %.0f%%, covers %.0f%% of the drawn lane\n",
			p.ProposalID, p.Grade, p.DrawnID, p.RMSEM, p.MeanOffsetM, 100*p.OverlapShare, 100*p.DrawnCoverage)
	}
	for _, id := range score.FalseHigh {
		fmt.Printf("  %s (high) pairs with no drawn lane\n", id)
	}
	verdict := func(ok bool) string {
		if ok {
			return "passes"
		}
		return "fails"
	}
	fmt.Printf("clause 1 (high RMSE < %.2f m): %s\n", lanes.ScoreRMSEMaxM, verdict(score.Clause1RMSE))
	fmt.Printf("clause 2 (lanes per direction): %s\n", verdict(score.Clause2Count))
	fmt.Printf("clause 3 (%d of %d drawn lanes found, need %.0f%%): %s\n", score.DrawnFound, score.DrawnTotal, 100*lanes.ScoreFoundShare, verdict(score.Clause3Found))
	fmt.Printf("clause 4 (no false high): %s\n", verdict(score.Clause4NoFalse))
	fmt.Printf("lane0002 at %s: %s\n", *site, verdict(score.Passed))
	return 0
}
