package main

import (
	"fmt"
	"sort"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Scoring one run's tracks against another's, per frame.
//
// What this is, and is not. Real per-frame ground truth does not exist yet:
// annotation poses are defined but unwritten, and track labels are per-track
// with no position. What a second evidence database gives is a *reference run*,
// which is another pass of the same tracker over the same capture with one
// setting changed. Comparing against it measures divergence from that run, not
// correctness, and the report says so in the output rather than relying on
// whoever reads it to remember.
//
// Divergence is still the thing worth measuring for the question in hand. If an
// arm's extra candidate tracks are pieces of the reference's tracks, the
// identity-switch and fragmentation counts against that reference rise. If they
// are objects the reference never found, false positives rise while identity
// and fragmentation stay flat. Those are different answers and a temporal-IoU
// score cannot tell them apart.

// ReferenceComparison is the per-frame scoring of one source against another.
type ReferenceComparison struct {
	// Kind records what the reference is, so a number is never read as
	// accuracy when it is agreement.
	Kind string `json:"kind"`
	Note string `json:"note"`
	// ReferenceSourceID and CandidateSourceID identify the two evidence
	// sources compared.
	ReferenceSourceID string `json:"reference_source_id"`
	CandidateSourceID string `json:"candidate_source_id"`
	// MatchDistanceMetres is the association gate, and for HOTA the distance
	// at which similarity reaches zero.
	MatchDistanceMetres float64                  `json:"match_distance_metres"`
	ReferenceTracks     int                      `json:"reference_tracks"`
	CandidateTracks     int                      `json:"candidate_tracks"`
	Metrics             l8analytics.TrackMetrics `json:"metrics"`
	HOTA                l8analytics.HOTAResult   `json:"hota"`
}

// loadSeries reads one evidence source's accepted estimates as track series,
// keyed by creation sequence. The random track_id is never used: it is not
// reproducible across replays, and the whole contract of this tool is a
// digest that is.
func loadSeries(states *observationsqlite.StateEstimateStore, sourceID string, fromNanos int64) ([]l8analytics.TrackSeries, error) {
	frameStates, err := states.ListFrameStateEstimatesBySource(sourceID)
	if err != nil {
		return nil, err
	}
	bySequence := map[int64]*l8analytics.TrackSeries{}
	for _, f := range frameStates {
		e := f.Estimate
		if e.FrameUnixNanos < fromNanos {
			continue
		}
		series, ok := bySequence[e.CreationSequence]
		if !ok {
			series = &l8analytics.TrackSeries{ID: fmt.Sprintf("seq-%06d", e.CreationSequence)}
			bySequence[e.CreationSequence] = series
		}
		series.Points = append(series.Points, l8analytics.SeriesPoint{
			TimestampNanos: e.FrameUnixNanos, X: e.X, Y: e.Y,
		})
	}

	sequences := make([]int64, 0, len(bySequence))
	for seq := range bySequence {
		sequences = append(sequences, seq)
	}
	sort.Slice(sequences, func(i, j int) bool { return sequences[i] < sequences[j] })

	out := make([]l8analytics.TrackSeries, 0, len(sequences))
	for _, seq := range sequences {
		series := bySequence[seq]
		sort.Slice(series.Points, func(i, j int) bool {
			return series.Points[i].TimestampNanos < series.Points[j].TimestampNanos
		})
		out = append(out, *series)
	}
	return out, nil
}

// compareToReference scores the candidate source against every source in the
// reference database. A reference database written by one baseline run of one
// case holds exactly one.
func compareToReference(referenceDBPath, candidateSourceID string, candidate []l8analytics.TrackSeries,
	matchDistanceMetres float64, fromNanos int64) ([]ReferenceComparison, error) {

	database, err := db.NewDBWithMigrationCheck(referenceDBPath, false)
	if err != nil {
		return nil, fmt.Errorf("open reference %s: %w", referenceDBPath, err)
	}
	defer database.Close()

	observations := observationsqlite.NewObservationStore(database)
	states := observationsqlite.NewStateEstimateStore(database)
	sourceIDs, err := observations.ListSourceIDs()
	if err != nil {
		return nil, err
	}

	out := make([]ReferenceComparison, 0, len(sourceIDs))
	for _, sourceID := range sourceIDs {
		reference, err := loadSeries(states, sourceID, fromNanos)
		if err != nil {
			return nil, err
		}
		out = append(out, ReferenceComparison{
			Kind: "reference_run",
			Note: "another replay of the same capture with one setting changed; this measures " +
				"divergence from that run, not accuracy. No per-frame human reference exists yet.",
			ReferenceSourceID:   sourceID,
			CandidateSourceID:   candidateSourceID,
			MatchDistanceMetres: matchDistanceMetres,
			ReferenceTracks:     len(reference),
			CandidateTracks:     len(candidate),
			Metrics:             l8analytics.ComputeTrackMetrics(reference, candidate, matchDistanceMetres),
			HOTA:                l8analytics.ComputeHOTA(reference, candidate, matchDistanceMetres, nil),
		})
	}
	return out, nil
}
