package lanes

import (
	"math"
	"sort"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/lidar/lanegeom"
)

// Scoring Gerardus's proposals against drawn lanes (experiment lane0002,
// data/experiments/lane/lane0002.md). Both are in the site frame, read from
// Gerardus's GeoJSON: the proposals' `lane_proposals` layer and the drawn
// lanes, the street-use travel lanes tagged with the reference tag. Nothing
// here reads a track.

// lane0002's rule thresholds, written before the drawing.
const (
	ScoreRMSEMaxM      = 0.30
	ScoreFoundCoverage = 0.70
	ScoreFoundShare    = 0.80
	ScoreStepM         = 1.0
	// A proposal pairs only with a drawn lane whose bearing is within 45°.
	scorePairBearingDeg = 45.0
)

// ScoreLane is one line in the site frame with its direction's key.
type ScoreLane struct {
	ID         string
	Points     []lanegeom.Point
	Direction  string // street and bound
	Grade      string // high or medium for a proposal
	BearingDeg float64
}

// ScorePair is one proposal and the drawn lane it pairs with.
type ScorePair struct {
	ProposalID   string  `json:"proposal_id"`
	DrawnID      string  `json:"drawn_id"`
	Grade        string  `json:"grade"`
	RMSEM        float64 `json:"rmse_m"`
	MeanOffsetM  float64 `json:"mean_offset_m"`
	OverlapShare float64 `json:"overlap_share"`
	// DrawnCoverage is the share of the drawn lane the proposal runs along
	// within ScoreRMSEMaxM.
	DrawnCoverage float64 `json:"drawn_coverage"`
}

// ScoreDirection is one direction's counts.
type ScoreDirection struct {
	Direction string `json:"direction"`
	Drawn     int    `json:"drawn"`
	High      int    `json:"high"`
	Medium    int    `json:"medium"`
}

// Score is a site's result, with lane0002's clauses.
type Score struct {
	Pairs          []ScorePair      `json:"pairs"`
	Directions     []ScoreDirection `json:"directions"`
	DrawnFound     int              `json:"drawn_found"`
	DrawnTotal     int              `json:"drawn_total"`
	FalseHigh      []string         `json:"false_high"`
	Clause1RMSE    bool             `json:"clause_1_rmse"`
	Clause2Count   bool             `json:"clause_2_count"`
	Clause3Found   bool             `json:"clause_3_found"`
	Clause4NoFalse bool             `json:"clause_4_no_false"`
	Passed         bool             `json:"passed"`
}

// ScoreFeatures turns GeoJSON features into lanes: proposals (`confidence`
// high or medium) or drawn travel lanes carrying the reference tag.
func ScoreFeatures(features []Feature, referenceKey, referenceValue string, proposals bool) []ScoreLane {
	return scoreFeatures(features, func(f Feature) bool {
		return referenceKey != "" && f.Tags[referenceKey] == referenceValue
	}, proposals)
}

// ScoreDrawnBy turns the travel lanes saved under one author into drawn
// lanes: the painter writes no free-form tag, so a reference drawing is
// saved under its own author name. Lanes approved from a proposal (they
// carry `proposal`) are never a reference.
func ScoreDrawnBy(features []Feature, author string) []ScoreLane {
	return scoreFeatures(features, func(f Feature) bool {
		return author != "" && f.Author == author && f.Tags["proposal"] == ""
	}, false)
}

func scoreFeatures(features []Feature, isReference func(Feature) bool, proposals bool) []ScoreLane {
	var out []ScoreLane
	for _, f := range features {
		if f.Kind != KindTravelLane || len(f.XYZ) < 6 {
			continue
		}
		grade := f.Tags["confidence"]
		if proposals && grade != "high" && grade != "medium" {
			continue
		}
		if !proposals && !isReference(f) {
			continue
		}
		l := ScoreLane{ID: f.FeatureID, Grade: grade}
		for i := 0; i+2 < len(f.XYZ); i += 3 {
			l.Points = append(l.Points, lanegeom.Point{X: f.XYZ[i], Y: f.XYZ[i+1]})
		}
		n := len(f.XYZ)
		l.BearingDeg = siteBearing(f.XYZ[0], f.XYZ[1], f.XYZ[n-3], f.XYZ[n-2])
		if b, err := strconv.ParseFloat(f.Tags[TagBearing], 64); err == nil {
			l.BearingDeg = b
		}
		bound := f.Tags[TagBound]
		if bound == "" {
			bound = Bound(l.BearingDeg)
		}
		street := f.Tags[TagStreet]
		if street == "" {
			street = f.Tags[TagCNN]
		}
		l.Direction = street + " " + bound
		out = append(out, l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// ScoreProposals pairs each proposal with the drawn lane of the closest
// bearing (within 45°) whose centreline it runs nearest, measures the
// lateral offset at every metre of the proposal that projects inside the
// drawn lane, and applies lane0002's rule to the high proposals.
func ScoreProposals(proposals, drawn []ScoreLane) Score {
	var s Score
	s.DrawnTotal = len(drawn)
	found := map[string]bool{}
	for _, p := range proposals {
		best := ScorePair{RMSEM: math.Inf(1)}
		for _, d := range drawn {
			if angleGap(p.BearingDeg, d.BearingDeg) > scorePairBearingDeg {
				continue
			}
			rmse, mean, overlap := lateral(p.Points, d.Points)
			if overlap == 0 || rmse >= best.RMSEM {
				continue
			}
			cov := coverage(d.Points, p.Points)
			best = ScorePair{ProposalID: p.ID, DrawnID: d.ID, Grade: p.Grade, RMSEM: rmse, MeanOffsetM: mean, OverlapShare: overlap, DrawnCoverage: cov}
		}
		if best.DrawnID == "" || best.OverlapShare < 0.5 || best.RMSEM > 1.5 {
			if p.Grade == "high" {
				s.FalseHigh = append(s.FalseHigh, p.ID)
			}
			continue
		}
		best.RMSEM = round3(best.RMSEM)
		best.MeanOffsetM = round3(best.MeanOffsetM)
		best.OverlapShare = round3(best.OverlapShare)
		best.DrawnCoverage = round3(best.DrawnCoverage)
		s.Pairs = append(s.Pairs, best)
		if p.Grade == "high" && best.RMSEM < ScoreRMSEMaxM && best.DrawnCoverage >= ScoreFoundCoverage {
			found[best.DrawnID] = true
		}
	}
	s.DrawnFound = len(found)

	dirs := map[string]*ScoreDirection{}
	get := func(k string) *ScoreDirection {
		if dirs[k] == nil {
			dirs[k] = &ScoreDirection{Direction: k}
		}
		return dirs[k]
	}
	for _, d := range drawn {
		get(d.Direction).Drawn++
	}
	for _, p := range proposals {
		if p.Grade == "high" {
			get(p.Direction).High++
		} else {
			get(p.Direction).Medium++
		}
	}
	s.Clause2Count = true
	for _, k := range sortedKeys(dirs) {
		d := *dirs[k]
		s.Directions = append(s.Directions, d)
		if d.High+d.Medium != d.Drawn {
			s.Clause2Count = false
		}
	}
	s.Clause1RMSE = true
	for _, pr := range s.Pairs {
		if pr.Grade == "high" && pr.RMSEM >= ScoreRMSEMaxM {
			s.Clause1RMSE = false
		}
	}
	s.Clause3Found = s.DrawnTotal > 0 && float64(s.DrawnFound) >= ScoreFoundShare*float64(s.DrawnTotal)
	s.Clause4NoFalse = len(s.FalseHigh) == 0
	s.Passed = s.Clause1RMSE && s.Clause2Count && s.Clause3Found && s.Clause4NoFalse && s.DrawnTotal > 0
	return s
}

// lateral samples a every ScoreStepM and projects each sample onto b: the
// RMSE and mean of the signed offsets of samples that fall inside b, and the
// share of samples that do.
func lateral(a, b []lanegeom.Point) (rmse, mean, overlap float64) {
	lane := lanegeom.Lane{Points: b}
	samples := sample(a, ScoreStepM)
	if len(samples) == 0 {
		return 0, 0, 0
	}
	var sum, sum2 float64
	n := 0
	for _, p := range samples {
		pr, err := lane.Project(p)
		if err != nil || pr.Beyond {
			continue
		}
		sum += pr.Lateral
		sum2 += pr.Lateral * pr.Lateral
		n++
	}
	if n == 0 {
		return 0, 0, 0
	}
	return math.Sqrt(sum2 / float64(n)), sum / float64(n), float64(n) / float64(len(samples))
}

// coverage is the share of b's samples within ScoreRMSEMaxM of a.
func coverage(b, a []lanegeom.Point) float64 {
	lane := lanegeom.Lane{Points: a}
	samples := sample(b, ScoreStepM)
	if len(samples) == 0 {
		return 0
	}
	n := 0
	for _, p := range samples {
		if pr, err := lane.Project(p); err == nil && !pr.Beyond && math.Abs(pr.Lateral) < ScoreRMSEMaxM {
			n++
		}
	}
	return float64(n) / float64(len(samples))
}

func sample(pts []lanegeom.Point, step float64) []lanegeom.Point {
	var out []lanegeom.Point
	carry := 0.0
	for i := 1; i < len(pts); i++ {
		a, b := pts[i-1], pts[i]
		seg := math.Hypot(b.X-a.X, b.Y-a.Y)
		if seg < 1e-9 {
			continue
		}
		for t := carry; t <= seg; t += step {
			f := t / seg
			out = append(out, lanegeom.Point{X: a.X + f*(b.X-a.X), Y: a.Y + f*(b.Y-a.Y)})
		}
		carry = math.Mod(seg-carry, step)
		if carry > 0 {
			carry = step - carry
		}
	}
	return out
}

func sortedKeys(m map[string]*ScoreDirection) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func round3(v float64) float64 { return math.Round(v*1000) / 1000 }
