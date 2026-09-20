// Command lidar-track-scorecard computes the label-free track scorecard
// (internal/lidar/l8analytics.ComputeTrackScorecard) from one immutable
// evidence database written by lidar-state-estimation-baseline.
//
// It exists so that a default-off option can be compared on against off
// across the whole replay corpus without labels: track lifetimes, how tracks
// end, constant-velocity coast error, and innovations stratified by range and
// support. See data/experiments/try/campaign/OBJECTIVES.md.
//
// Output is canonical JSON. Two independent replays of the same input must
// give byte-identical output, and the tool prints the SHA-256 so a sweep can
// check that without parsing anything. Nothing in the output depends on wall
// time, a filesystem path, or the random track_id.
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// sourceScorecard is one evidence source's result. A database written by one
// baseline run of one case has exactly one source.
type sourceScorecard struct {
	SourceID  string                     `json:"source_id"`
	Scorecard l8analytics.TrackScorecard `json:"scorecard"`
	// Reference is present only when -reference named another evidence
	// database. Empty otherwise, so a scorecard without one is unchanged.
	Reference []ReferenceComparison `json:"reference,omitempty"`
}

type document struct {
	SchemaVersion       int               `json:"schema_version"`
	ScoringStartSeconds float64           `json:"scoring_start_seconds"`
	Sources             []sourceScorecard `json:"sources"`
}

func main() {
	var (
		dbPath       = flag.String("observations", "", "observations.db written by lidar-state-estimation-baseline (required)")
		scoringStart = flag.Float64("scoring-start-seconds", 0, "seconds after the first frame at which scoring starts; use the replay's warm-up")
		jsonOut      = flag.String("json", "", "path for the canonical JSON result (required)")
		reference    = flag.String("reference", "", "another observations.db to score against per frame (MOTA/MOTP/IDSW/FM/HOTA). "+
			"It is a reference run, not ground truth: the result measures divergence from it")
		matchDist = flag.Float64("reference-match-metres", 2.0, "association gate for reference matching, and the distance at which HOTA similarity reaches zero")
	)
	flag.Parse()
	if *dbPath == "" || *jsonOut == "" {
		fatal(fmt.Errorf("-observations and -json are required"))
	}
	doc, err := score(*dbPath, *scoringStart, *reference, *matchDist)
	if err != nil {
		fatal(err)
	}
	payload, err := marshal(doc)
	if err != nil {
		fatal(err)
	}
	if err := os.WriteFile(*jsonOut, payload, 0o644); err != nil {
		fatal(fmt.Errorf("write %s: %w", *jsonOut, err))
	}
	sum := sha256.Sum256(payload)
	fmt.Printf("sha256:%s\n", hex.EncodeToString(sum[:]))
	for _, s := range doc.Sources {
		p := s.Scorecard.Population
		fmt.Fprintf(os.Stderr, "%s: tracks=%d median_lifetime=%.2fs under_1s=%.1f%% ended=%d\n",
			s.SourceID, p.Tracks, p.LifetimeMedianSecs, 100*p.ShareUnderOneSecond, s.Scorecard.Termination.Ended)
	}
}

// marshal is the canonical encoding: struct fields in declaration order, map
// keys sorted by encoding/json, two-space indent, trailing newline.
func marshal(doc document) ([]byte, error) {
	payload, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal scorecard: %w", err)
	}
	return append(payload, '\n'), nil
}

func score(dbPath string, scoringStartSeconds float64, referenceDBPath string, matchDistanceMetres float64) (document, error) {
	doc := document{SchemaVersion: 1, ScoringStartSeconds: scoringStartSeconds, Sources: []sourceScorecard{}}
	// Read through internal/db and the storage layer, as lidar-e1-analysis
	// does: only those packages may touch database/sql.
	database, err := db.NewDBWithMigrationCheck(dbPath, false)
	if err != nil {
		return doc, err
	}
	defer database.Close()
	observations := observationsqlite.NewObservationStore(database)
	states := observationsqlite.NewStateEstimateStore(database)

	sourceIDs, err := observations.ListSourceIDs()
	if err != nil {
		return doc, err
	}
	for _, sourceID := range sourceIDs {
		summaries, err := observations.ListClusterSummariesBySource(sourceID)
		if err != nil {
			return doc, err
		}
		frameStates, err := states.ListFrameStateEstimatesBySource(sourceID)
		if err != nil {
			return doc, err
		}
		clusters := make([]l8analytics.ScorecardCluster, 0, len(summaries))
		var firstFrame int64
		for i, c := range summaries {
			if i == 0 || c.FrameUnixNanos < firstFrame {
				firstFrame = c.FrameUnixNanos
			}
			clusters = append(clusters, l8analytics.ScorecardCluster{
				ObservationID: c.ObservationID, FrameUnixNanos: c.FrameUnixNanos,
				X: c.X, Y: c.Y, PointsCount: c.PointsCount,
			})
		}
		estimates := make([]l8analytics.ScorecardEstimate, 0, len(frameStates))
		for _, f := range frameStates {
			e, r := f.Estimate, f.Residual
			estimates = append(estimates, l8analytics.ScorecardEstimate{
				CreationSequence: e.CreationSequence, FrameUnixNanos: e.FrameUnixNanos,
				ObservationID: e.ObservationID,
				X:             float64(e.X), Y: float64(e.Y), VX: float64(e.VX), VY: float64(e.VY),
				MeasurementX: float64(r.MeasurementX), MeasurementY: float64(r.MeasurementY),
				InnovationX: float64(r.InnovationX), InnovationY: float64(r.InnovationY),
				NIS: float64(r.NIS),
			})
		}
		scoringStart := firstFrame + int64(scoringStartSeconds*1e9)
		opts := l8analytics.ScorecardOptions{ScoringStartNanos: scoringStart}
		entry := sourceScorecard{
			SourceID: sourceID, Scorecard: l8analytics.ComputeTrackScorecard(estimates, clusters, opts),
		}
		if referenceDBPath != "" {
			candidate, err := loadSeries(states, sourceID, scoringStart)
			if err != nil {
				return doc, err
			}
			entry.Reference, err = compareToReference(referenceDBPath, sourceID, candidate, matchDistanceMetres, scoringStart)
			if err != nil {
				return doc, err
			}
		}
		doc.Sources = append(doc.Sources, entry)
	}
	return doc, nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "lidar-track-scorecard: %v\n", err)
	os.Exit(1)
}
