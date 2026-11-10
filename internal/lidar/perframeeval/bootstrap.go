package perframeeval

import (
	"fmt"
	"math"
	"math/rand"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l8analytics"
)

// The object bootstrap (experiment l5id0015): how wide a split's per-frame
// figures are when the reviewed objects are a sample. A replay is
// byte-deterministic, so two runs of one arm agree exactly; the uncertainty in
// a figure is which objects a reviewer happened to label. The bootstrap draws
// the split's objects with replacement, scores each draw, and reports the
// spread.
//
// Each hypothesis track is attributed, once, to the reference object it lies
// within the gate of on the most frames (ties to the lower object id); a draw
// keeps an object's reference series and the tracks attributed to it, so an
// object drawn twice brings its tracks twice. Tracks attributed to no object
// are left out of every draw, the full sample included: they are the false
// positives on unlabelled returns, which a selective pack cannot judge. The
// bootstrap's point estimate is therefore the labelled-object figure, close to
// but not the same as the full comparison's, and it is reported beside the
// intervals.

// BootstrapOptions are the draws to make.
type BootstrapOptions struct {
	Draws int   `json:"draws"`
	Seed  int64 `json:"seed"`
}

// Percentiles is the 5th, 50th and 95th percentile of one figure over the
// draws.
type Percentiles struct {
	P5  float64 `json:"p5"`
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
}

// BootstrapFigures are the figures the bootstrap reports, each over the draws.
type BootstrapFigures struct {
	HOTA           Percentiles `json:"hota"`
	IDF1           Percentiles `json:"idf1"`
	IDSwitches     Percentiles `json:"id_switches"`
	Fragmentations Percentiles `json:"fragmentations"`
	// FN is the draw's missed reference points (l6cls0005 reads its paired
	// delta: a birth rule must not establish a rise in what is missed).
	FN Percentiles `json:"fn"`
	// Per object: switches and fragmentations divided by the draw's object
	// count, comparable between draws of a different make-up.
	IDSwitchesPerObject     Percentiles `json:"id_switches_per_object"`
	FragmentationsPerObject Percentiles `json:"fragmentations_per_object"`
}

// BootstrapArm is one arm's attribution and spread.
type BootstrapArm struct {
	Label string `json:"label"`
	// Point is the labelled-object figure on the full sample (every object
	// once), the figure the intervals surround.
	Point              Summary          `json:"point"`
	Spread             BootstrapFigures `json:"spread"`
	TracksAttributed   int              `json:"tracks_attributed"`
	TracksUnattributed int              `json:"tracks_unattributed"`
	ObjectsWithNoTrack int              `json:"objects_with_no_track"`
	// ByObject is each reviewed object scored alone, every object once with
	// the tracks attributed to it, in Objects' order: what one object's
	// matched frames are, so a change that costs one object frames shows
	// even where the pooled figures do not move. A track that crosses onto a
	// second object counts for the object it is attributed to only.
	ByObject            []ObjectPoint `json:"by_object"`
	attributedByEpisode []map[string][]l8analytics.TrackSeries
}

// ObjectPoint is one reviewed object's figures with its attributed tracks.
type ObjectPoint struct {
	Object string  `json:"object"`
	Point  Summary `json:"point"`
}

// BootstrapResult is both arms and the paired delta, B less A, with the same
// draws applied to each.
type BootstrapResult struct {
	Options BootstrapOptions `json:"options"`
	Objects []string         `json:"objects"`
	A       BootstrapArm     `json:"arm_a"`
	B       BootstrapArm     `json:"arm_b"`
	Delta   BootstrapFigures `json:"delta_b_minus_a"`
	// DeltaStands names each delta whose 5th and 95th percentiles lie on the
	// same side of zero as its median: the draws agree on its sign.
	DeltaStands map[string]bool `json:"delta_stands"`
	Caveats     []string        `json:"caveats"`
}

// RunBootstrap loads the reference and both arms as Run does and bootstraps
// the objects.
func RunBootstrap(cfg Config, opts BootstrapOptions) (*BootstrapResult, error) {
	if err := cfg.Score.Validate(); err != nil {
		return nil, err
	}
	ref, err := LoadReference(cfg.Reference)
	if err != nil {
		return nil, err
	}
	a, err := LoadArm(cfg.A)
	if err != nil {
		return nil, err
	}
	b, err := LoadArm(cfg.B)
	if err != nil {
		return nil, err
	}
	return Bootstrap(ref, [2]Hypothesis{a, b}, cfg.Score, opts)
}

// Bootstrap draws the reference's objects with replacement opts.Draws times
// and scores both arms on each draw.
func Bootstrap(ref *Reference, arms [2]Hypothesis, score ScoreOptions, opts BootstrapOptions) (*BootstrapResult, error) {
	if opts.Draws < 1 {
		return nil, fmt.Errorf("bootstrap needs at least one draw, got %d", opts.Draws)
	}
	objects := referenceObjects(ref)
	if len(objects) == 0 {
		return nil, fmt.Errorf("the reference has no scored object to draw")
	}
	res := &BootstrapResult{Options: opts, Objects: objects, DeltaStands: map[string]bool{}}
	for i, h := range arms {
		arm, err := attribute(ref, h, score)
		if err != nil {
			return nil, err
		}
		full := make([]int, len(objects))
		for j := range full {
			full[j] = j
		}
		arm.Point = scoreDraw(ref, arm, objects, full, score)
		for j, id := range objects {
			arm.ByObject = append(arm.ByObject, ObjectPoint{Object: id, Point: scoreDraw(ref, arm, objects, []int{j}, score)})
		}
		if i == 0 {
			res.A = arm
		} else {
			res.B = arm
		}
	}

	rng := rand.New(rand.NewSource(opts.Seed))
	var sa, sb []Summary
	draw := make([]int, len(objects))
	for d := 0; d < opts.Draws; d++ {
		for j := range draw {
			draw[j] = rng.Intn(len(objects))
		}
		sa = append(sa, scoreDraw(ref, res.A, objects, draw, score))
		sb = append(sb, scoreDraw(ref, res.B, objects, draw, score))
	}
	n := float64(len(objects))
	res.A.Spread = spread(sa, n)
	res.B.Spread = spread(sb, n)
	deltas := make([]Summary, len(sa))
	for i := range sa {
		deltas[i] = sb[i].minus(sa[i])
	}
	res.Delta = spread(deltas, n)
	for name, p := range map[string]Percentiles{
		"hota": res.Delta.HOTA, "idf1": res.Delta.IDF1,
		"id_switches": res.Delta.IDSwitches, "fragmentations": res.Delta.Fragmentations,
		"fn": res.Delta.FN,
	} {
		res.DeltaStands[name] = (p.P5 > 0 && p.P95 > 0) || (p.P5 < 0 && p.P95 < 0)
	}
	res.Caveats = []string{
		"Draws resample the reviewed objects of one split; the interval is over the objects a reviewer chose, not over road users at large.",
		"Hypothesis tracks attributed to no reviewed object are left out of every draw, so the point figures exclude false positives on unlabelled returns and differ from the full comparison's.",
		"Objects in one episode are not independent: a draw that takes two objects that interact twice counts their interaction twice.",
	}
	return res, nil
}

// referenceObjects is every object with a scored point in some episode,
// sorted. A series whose every point is ignored (an object outside the
// split, or one the policy leaves out, such as a parked car) is truth for
// nothing: it is never drawn.
func referenceObjects(ref *Reference) []string {
	seen := map[string]bool{}
	for _, ep := range ref.Episodes {
		for _, s := range ep.Series {
			for _, p := range s.Points {
				if !p.Ignore {
					seen[s.ID] = true
					break
				}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// attribute aligns the arm to each episode and gives each hypothesis track to
// the reference object it lies within the gate of on the most frames.
func attribute(ref *Reference, hyp Hypothesis, score ScoreOptions) (BootstrapArm, error) {
	arm := BootstrapArm{Label: hyp.Identity.Label}
	withTrack := map[string]bool{}
	for _, ep := range ref.Episodes {
		aligned, stats := alignToEpisode(hyp.Series, ep, score.FrameToleranceNanos)
		if f := stats.UnalignedFraction(); f > score.MaxUnalignedFraction {
			return BootstrapArm{}, fmt.Errorf("arm %s, episode %s: %.1f%% of points unaligned", hyp.Identity.Label, ep.EpisodeID, 100*f)
		}
		refAt := map[int64][]refPoint{}
		for _, s := range ep.Series {
			for _, p := range s.Points {
				if p.Ignore {
					continue
				}
				refAt[p.TimestampNanos] = append(refAt[p.TimestampNanos], refPoint{id: s.ID, p: p})
			}
		}
		byObject := map[string][]l8analytics.TrackSeries{}
		for _, h := range aligned {
			hits := map[string]int{}
			for _, p := range h.Points {
				for _, r := range refAt[p.TimestampNanos] {
					if within(score.Gate, r.p, p) {
						hits[r.id]++
					}
				}
			}
			best, bestN := "", 0
			for id, n := range hits {
				if n > bestN || (n == bestN && id < best) {
					best, bestN = id, n
				}
			}
			if bestN == 0 {
				arm.TracksUnattributed++
				continue
			}
			arm.TracksAttributed++
			byObject[best] = append(byObject[best], h)
			withTrack[best] = true
		}
		arm.attributedByEpisode = append(arm.attributedByEpisode, byObject)
	}
	for _, id := range referenceObjects(ref) {
		if !withTrack[id] {
			arm.ObjectsWithNoTrack++
		}
	}
	return arm, nil
}

type refPoint struct {
	id string
	p  l8analytics.SeriesPoint
}

// within is the gate's test between one reference and one hypothesis point.
func within(g l8analytics.MatchGate, r, h l8analytics.SeriesPoint) bool {
	limit := g.Metres
	if g.Kind == l8analytics.GateFootprint {
		limit += 0.5 * float64(r.FootprintDiagonalMetres)
	}
	return math.Hypot(float64(r.X-h.X), float64(r.Y-h.Y)) <= limit
}

// scoreDraw scores one arm on the objects a draw names, by index into
// objects, each occurrence a separate copy of the object and its tracks.
func scoreDraw(ref *Reference, arm BootstrapArm, objects []string, draw []int, score ScoreOptions) Summary {
	var clear []l8analytics.TrackMetrics
	var hota []l8analytics.HOTAResult
	var identity []l8analytics.IdentityMetrics
	for e, ep := range ref.Episodes {
		series := map[string]l8analytics.TrackSeries{}
		for _, s := range ep.Series {
			series[s.ID] = s
		}
		var refs, hyps []l8analytics.TrackSeries
		for k, j := range draw {
			id := objects[j]
			s, ok := series[id]
			if !ok {
				continue
			}
			suffix := fmt.Sprintf("#%d", k)
			refs = append(refs, l8analytics.TrackSeries{ID: id + suffix, Points: s.Points})
			for _, h := range arm.attributedByEpisode[e][id] {
				hyps = append(hyps, l8analytics.TrackSeries{ID: h.ID + suffix, Points: h.Points})
			}
		}
		m := l8analytics.EvaluatePerFrame(refs, hyps, score.Gate, score.Alphas)
		clear = append(clear, m.CLEARMOT)
		hota = append(hota, m.HOTA)
		identity = append(identity, m.Identity)
	}
	pooled, err := l8analytics.CombineHOTA(hota)
	if err != nil {
		pooled = l8analytics.HOTAResult{}
	}
	return summarise(l8analytics.PerFrameResult{
		CLEARMOT: l8analytics.CombineTrackMetrics(clear),
		HOTA:     pooled,
		Identity: l8analytics.CombineIdentity(identity),
	})
}

// spread is the percentiles of each figure over the draws; n is the draw's
// object count, for the per-object rates.
func spread(s []Summary, n float64) BootstrapFigures {
	pick := func(f func(Summary) float64) Percentiles {
		v := make([]float64, len(s))
		for i := range s {
			v[i] = f(s[i])
		}
		sort.Float64s(v)
		return Percentiles{P5: quantile(v, 0.05), P50: quantile(v, 0.5), P95: quantile(v, 0.95)}
	}
	return BootstrapFigures{
		HOTA:                    pick(func(x Summary) float64 { return x.HOTA }),
		IDF1:                    pick(func(x Summary) float64 { return x.IDF1 }),
		IDSwitches:              pick(func(x Summary) float64 { return float64(x.IDSwitches) }),
		Fragmentations:          pick(func(x Summary) float64 { return float64(x.Fragmentations) }),
		FN:                      pick(func(x Summary) float64 { return float64(x.FN) }),
		IDSwitchesPerObject:     pick(func(x Summary) float64 { return float64(x.IDSwitches) / n }),
		FragmentationsPerObject: pick(func(x Summary) float64 { return float64(x.Fragmentations) / n }),
	}
}

// quantile is the linearly interpolated quantile of sorted values.
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return math.NaN()
	}
	pos := q * float64(len(sorted)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return sorted[lo]
	}
	return sorted[lo] + (pos-float64(lo))*(sorted[hi]-sorted[lo])
}
