package perframeeval

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/annotation"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// ManoeuvreManifest adds reviewed event intervals to the existing reference;
// it never supplies replacement positions or overrides pack/split eligibility.
type ManoeuvreManifest struct {
	SchemaVersion   int              `json:"schema_version"`
	ReferenceDigest string           `json:"reference_digest"`
	Events          []ManoeuvreEvent `json:"events"`
}

type ManoeuvreEvent struct {
	ID        string `json:"id"`
	EpisodeID string `json:"episode_id"`
	ObjectID  string `json:"object_id"`
	Kind      string `json:"kind"` // brake, turn or lane_change
	// Decimal strings preserve nanoseconds through browser label editors.
	StartNS  int64  `json:"start_ns,string"`
	EndNS    int64  `json:"end_ns,string"`
	Status   string `json:"status"`
	Reviewer string `json:"reviewer"`
}

type ManoeuvreOptions struct {
	Reference   ReferenceOptions
	EventsPath  string
	SamplesPath string
}

type ManoeuvreCounts struct {
	Labelled  int `json:"labelled_frames"`
	Eligible  int `json:"eligible_frames"`
	Gated     int `json:"gated_frames"`
	Missing   int `json:"missing_samples"`
	Ambiguous int `json:"ambiguous_samples"`
}

type ManoeuvreReport struct {
	SchemaVersion int               `json:"schema_version"`
	Reference     ReferenceIdentity `json:"reference"`
	EventsSHA256  string            `json:"events_sha256"`
	SamplesSHA256 string            `json:"samples_sha256"`
	ManoeuvreCounts
	Fraction *float64 `json:"gated_fraction"`
	Status   string   `json:"row_6_status"`
	// This is an individual gate row, never an overall promotion verdict.
	GateVerdict string   `json:"gate_verdict"`
	Caveats     []string `json:"caveats"`
}

// MeasureManoeuvres applies G-UNC-1 row 6. Time matching is exact: these are
// capture-frame samples, not interpolated trajectories. Spatial matching uses
// the predeclared 1 m + half footprint diagonal. Both sides must be unique;
// repeated eligible pairings, even at identical XY, remain ambiguous because
// the samples deliberately do not carry track identity. Gated/assigned flags
// never influence the match or denominator.
func MeasureManoeuvres(opts ManoeuvreOptions) (*ManoeuvreReport, error) {
	ref, err := LoadReference(opts.Reference)
	if err != nil {
		return nil, err
	}
	if ref.Identity.Policy.Status != annotation.ReferenceReviewedOnly {
		return nil, fmt.Errorf("manoeuvre scoring requires reviewed-only references")
	}
	data, err := os.ReadFile(opts.EventsPath)
	if err != nil {
		return nil, fmt.Errorf("read manoeuvre events: %w", err)
	}
	var manifest ManoeuvreManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parse manoeuvre events: %w", err)
	}
	labelled, all, err := manoeuvreFrames(ref, manifest)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(opts.SamplesPath)
	if err != nil {
		return nil, fmt.Errorf("open uncertainty samples: %w", err)
	}
	defer f.Close()
	hash := sha256.New()
	samples, err := readManoeuvreSamples(io.TeeReader(f, hash), labelled)
	if err != nil {
		return nil, err
	}
	eventHash := sha256.Sum256(data)
	r := &ManoeuvreReport{SchemaVersion: 1, Reference: ref.Identity,
		EventsSHA256: hex.EncodeToString(eventHash[:]), SamplesSHA256: hex.EncodeToString(hash.Sum(nil)),
		Status: "insufficient_samples", GateVerdict: "individual_row_only_no_promotion",
		Caveats: []string{"Exact capture-frame time; spatial gate is 1 m plus half footprint diagonal. No accepted-only selection.",
			"Samples SHA-256 identifies the scored bytes; capture, calibration and arm provenance must also be checked against the replay record.",
			"Missing or ambiguous samples prevent a within verdict. No interpolation, nearest-object choice or invented event labels."}}
	keys := make([]int64, 0, len(labelled))
	for ts := range labelled {
		keys = append(keys, ts)
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	for _, ts := range keys {
		for id, point := range labelled[ts] {
			r.Labelled++
			matches, shared := 0, false
			gated := false
			for _, sample := range samples[ts] {
				if !manoeuvreNear(sample, point) {
					continue
				}
				matches++
				gated = sample.Gated
				shared = shared || manoeuvreShared(sample, id, all[ts])
			}
			switch {
			case matches == 0:
				r.Missing++
			case matches != 1 || shared:
				r.Ambiguous++
			default:
				r.Eligible++
				if gated {
					r.Gated++
				}
			}
		}
	}
	r.Fraction, r.Status = manoeuvreVerdict(r.ManoeuvreCounts)
	if !ref.Identity.HeldOut {
		r.Caveats = append(r.Caveats, "Development/tuning split: this is not held-out qualification evidence.")
	}
	if ref.Identity.SplitDigest == "" {
		r.Caveats = append(r.Caveats, "Unfrozen split: this is not frozen qualification evidence.")
	}
	return r, nil
}

type manoeuvreFrameMap map[int64]map[string]l8analytics.SeriesPoint

func manoeuvreFrames(ref *Reference, m ManoeuvreManifest) (manoeuvreFrameMap, manoeuvreFrameMap, error) {
	if m.SchemaVersion != 1 || m.ReferenceDigest != ref.Identity.Digest {
		return nil, nil, fmt.Errorf("manoeuvre schema or reference digest mismatch")
	}
	all, labelled := manoeuvreFrameMap{}, manoeuvreFrameMap{}
	byEpisode := map[string]map[string][]l8analytics.SeriesPoint{}
	spans := map[string][]timeSpan{}
	for _, ep := range ref.Episodes {
		spans[ep.EpisodeID] = ep.spans
		byEpisode[ep.EpisodeID] = map[string][]l8analytics.SeriesPoint{}
		for _, series := range ep.Series {
			byEpisode[ep.EpisodeID][series.ID] = series.Points
			for _, p := range series.Points {
				// Ignored references can make a join ambiguous, never eligible.
				if all[p.TimestampNanos] == nil {
					all[p.TimestampNanos] = map[string]l8analytics.SeriesPoint{}
				}
				all[p.TimestampNanos][series.ID] = p
			}
		}
	}
	seen := map[string]bool{}
	for _, event := range m.Events {
		if strings.TrimSpace(event.ID) == "" || seen[event.ID] || event.StartNS > event.EndNS ||
			event.Status != "reviewed" || strings.TrimSpace(event.Reviewer) == "" ||
			(event.Kind != "brake" && event.Kind != "turn" && event.Kind != "lane_change") {
			return nil, nil, fmt.Errorf("invalid, duplicate or unreviewed manoeuvre event %q", event.ID)
		}
		seen[event.ID] = true
		inside := false
		for _, span := range spans[event.EpisodeID] {
			if event.StartNS >= span.first && event.EndNS <= span.last {
				inside = true
			}
		}
		if !inside {
			return nil, nil, fmt.Errorf("event %q crosses or lies outside its episode interval", event.ID)
		}
		points := byEpisode[event.EpisodeID][event.ObjectID]
		count := 0
		for _, p := range points {
			if p.Ignore || p.TimestampNanos < event.StartNS || p.TimestampNanos > event.EndNS {
				continue
			}
			if labelled[p.TimestampNanos] == nil {
				labelled[p.TimestampNanos] = map[string]l8analytics.SeriesPoint{}
			}
			labelled[p.TimestampNanos][event.ObjectID] = p // overlapping events count once
			count++
		}
		if count == 0 {
			return nil, nil, fmt.Errorf("event %q contains no reviewed scored frames in its episode/object", event.ID)
		}
	}
	return labelled, all, nil
}

func readManoeuvreSamples(reader io.Reader, frames manoeuvreFrameMap) (map[int64][]l5tracks.UncertaintySample, error) {
	out := map[int64][]l5tracks.UncertaintySample{}
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	line := 0
	for scanner.Scan() {
		line++
		var sample l5tracks.UncertaintySample
		if err := json.Unmarshal(scanner.Bytes(), &sample); err != nil {
			return nil, fmt.Errorf("uncertainty sample line %d: %w", line, err)
		}
		var required struct {
			Time  *int64   `json:"frame_unix_nanos"`
			X     *float32 `json:"x"`
			Y     *float32 `json:"y"`
			Rank  *int     `json:"rank"`
			Gated *bool    `json:"gated"`
		}
		// A missing false-valued gated field must not become a passing sample.
		_ = json.Unmarshal(scanner.Bytes(), &required) // same field types as above
		if required.Time == nil || required.X == nil || required.Y == nil || required.Rank == nil || required.Gated == nil {
			return nil, fmt.Errorf("incomplete uncertainty sample at line %d", line)
		}
		// encoding/json already refuses non-finite and overflowing coordinates.
		if sample.Rank != 1 && sample.Rank != 2 {
			return nil, fmt.Errorf("invalid uncertainty sample at line %d", line)
		}
		if frames[sample.FrameUnixNanos] != nil {
			out[sample.FrameUnixNanos] = append(out[sample.FrameUnixNanos], sample)
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read uncertainty samples: %w", err)
	}
	return out, nil
}

func manoeuvreNear(sample l5tracks.UncertaintySample, point l8analytics.SeriesPoint) bool {
	return math.Hypot(float64(sample.X)-float64(point.X), float64(sample.Y)-float64(point.Y)) <= 1+0.5*float64(point.FootprintDiagonalMetres)
}

func manoeuvreShared(sample l5tracks.UncertaintySample, targetID string, points map[string]l8analytics.SeriesPoint) bool {
	for id, point := range points {
		if id != targetID && manoeuvreNear(sample, point) {
			return true
		}
	}
	return false
}

func manoeuvreVerdict(c ManoeuvreCounts) (*float64, string) {
	if c.Eligible == 0 {
		return nil, "insufficient_samples"
	}
	fraction := float64(c.Gated) / float64(c.Eligible)
	if c.Missing != 0 || c.Ambiguous != 0 {
		return &fraction, "insufficient_samples"
	}
	if fraction < 0.01 {
		return &fraction, "within"
	}
	return &fraction, "outside"
}
