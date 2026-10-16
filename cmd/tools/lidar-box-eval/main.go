// Command lidar-box-eval replays a capture through the offline pipeline and
// scores the box a viewer draws for each confirmed track: whether the box
// holds the points it was drawn from, and whether it turns the way a road
// vehicle can.
//
// The tracker's own published box is always scored (variant "published"),
// and so is every box-model variant named with -variants, each run beside
// the tracker on the same clusters and the same tracked states. A box-model
// variant reads the tracker and writes nothing back, so one replay scores
// them all, and an arm whose tuning enables the box model must reproduce the
// matching variant's figures under "published".
//
// The output is a JSON summary per variant and track group; -rows adds one
// CSV line per scored track frame for plotting. Neither belongs in Git.
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/config"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
	"github.com/banshee-data/velocity.report/internal/version"
)

func main() {
	var (
		pcaps      = flag.String("pcap", "", "capture files, comma separated, in order (required)")
		udpPort    = flag.Int("udp-port", 2369, "UDP port the capture's packets are on")
		tuning     = flag.String("tuning", "", "tuning JSON; empty uses the embedded defaults")
		warmup     = flag.Float64("warmup", 20, "warm-up seconds before scoring")
		duration   = flag.Float64("duration", 0, "scoring seconds; 0 scores the rest of the capture")
		caseName   = flag.String("case", "", "case label written into the summary")
		outPath    = flag.String("out", "", "summary JSON path (required)")
		rowsPath   = flag.String("rows", "", "optional CSV of every scored track frame")
		variants   = flag.String("variants", "all", "box-model variants to run beside the tracker, comma separated, 'all' or 'none'; see -list")
		list       = flag.Bool("list", false, "list the box-model variants and exit")
		sensorX    = flag.Float64("sensor-x", 0, "sensor X in the world frame, for the box model's unseen side")
		sensorY    = flag.Float64("sensor-y", 0, "sensor Y in the world frame")
		tolerance  = flag.Float64("tolerance", 0.10, "a point is outside a box when further than this beyond its edge (metres)")
		requireSet = flag.Bool("require-settled", true, "refuse a capture whose background is unsettled at the scoring boundary")
	)
	flag.Parse()
	// The pipeline reads the default tuning document wherever it runs from.
	config.SetEmbeddedDefaults(radarassets.TuningDefaults)
	if *list {
		for _, v := range boxVariants() {
			fmt.Printf("%-16s %s\n", v.name, v.about)
		}
		return
	}
	if *pcaps == "" || *outPath == "" {
		log.Fatal("-pcap and -out are required")
	}
	chosen, err := selectVariants(*variants)
	if err != nil {
		log.Fatal(err)
	}
	for i := range chosen {
		chosen[i].cfg.SensorX, chosen[i].cfg.SensorY = *sensorX, *sensorY
	}

	sc := newScorer(chosen, *tolerance)
	if *rowsPath != "" {
		f, err := os.Create(*rowsPath)
		if err != nil {
			log.Fatal(err)
		}
		defer f.Close()
		sc.rows = csv.NewWriter(f)
		defer sc.rows.Flush()
		_ = sc.rows.Write(rowHeader)
	}

	tmp, err := os.MkdirTemp("", "lidar-box-eval-")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	started := time.Now()
	res, err := replayeval.Run(replayeval.Config{
		PCAPFiles:          strings.Split(*pcaps, ","),
		OutDir:             tmp,
		TuningFile:         *tuning,
		SensorID:           "hesai-pandar40p",
		UDPPort:            *udpPort,
		StartSeconds:       *warmup,
		WarmupSeconds:      *warmup,
		DurationSeconds:    *duration,
		RequireSettled:     *requireSet,
		KeepClusterMembers: true,
		TrackerTap:         sc.frame,
	})
	if err != nil {
		log.Fatal(err)
	}
	summary := sc.summary()
	summary.Case = *caseName
	summary.Captures = strings.Split(*pcaps, ",")
	summary.Tuning = *tuning
	summary.WarmupSeconds, summary.DurationSeconds = *warmup, *duration
	summary.Tolerance = *tolerance
	summary.Build = version.Version + " " + version.GitSHA
	summary.Recording = res.VRLOGPath
	summary.WallSeconds = time.Since(started).Seconds()
	b, err := json.MarshalIndent(summary, "", "  ")
	if err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*outPath, append(b, '\n'), 0o644); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%s: %d scored frames, %d confirmed tracks, wrote %s in %.0f s\n",
		*caseName, summary.Frames, summary.Tracks, *outPath, summary.WallSeconds)
}

// variant is one box model run beside the tracker.
type variant struct {
	name  string
	about string
	cfg   l5tracks.BoxModelConfig
}

func boxVariants() []variant {
	def := l5tracks.DefaultBoxModelConfig()
	def.Enabled = true
	with := func(name, about string, edit func(*l5tracks.BoxModelConfig)) variant {
		c := def
		edit(&c)
		return variant{name: name, about: about, cfg: c}
	}
	return []variant{
		{name: "km", about: "box model, defaults", cfg: def},
		with("km-pca", "axis from the cluster's PCA box, no members", func(c *l5tracks.BoxModelConfig) { c.AxisSource = l5tracks.BoxAxisPCA }),
		with("km-free", "no yaw-rate bound", func(c *l5tracks.BoxModelConfig) {
			c.MaxCurvature, c.MinYawRate, c.UnlabelledYawRate = 100, 100, 100
		}),
		with("km-k15", "curvature bound 0.15 /m", func(c *l5tracks.BoxModelConfig) { c.MaxCurvature = 0.15 }),
		with("km-k25", "curvature bound 0.25 /m", func(c *l5tracks.BoxModelConfig) { c.MaxCurvature = 0.25 }),
		with("km-sig1", "axis sigma × 1", func(c *l5tracks.BoxModelConfig) { c.AxisSigmaScale = 1 }),
		with("km-yaw02", "yaw acceleration sigma 0.2 rad/s²", func(c *l5tracks.BoxModelConfig) { c.YawAccelSigma = 0.2 }),
		with("km-floor0", "no vehicle length floor", func(c *l5tracks.BoxModelConfig) { c.VehicleLengthFloor = 0 }),
		with("km-full", "fit on 256 points at a 1° coarse step", func(c *l5tracks.BoxModelConfig) { c.FitMaxPoints, c.FitCoarseStepDeg = 0, 0 }),
		with("km-cheap64", "fit on 64 points at a 2° coarse step", func(c *l5tracks.BoxModelConfig) { c.FitMaxPoints, c.FitCoarseStepDeg = 64, 2 }),
		with("km-pull0", "centre carried only, no sensor anchoring", func(c *l5tracks.BoxModelConfig) { c.CentrePull = 0 }),
		with("km-pull1", "centre anchored every frame", func(c *l5tracks.BoxModelConfig) { c.CentrePull = 1 }),
		with("km-w5", "yaw floor 5°/s, unlabelled 45°/s, as the first build", func(c *l5tracks.BoxModelConfig) { c.MinYawRate = 5 * math.Pi / 180 }),
	}
}

func selectVariants(spec string) ([]variant, error) {
	all := boxVariants()
	switch spec {
	case "all":
		return all, nil
	case "none", "":
		return nil, nil
	}
	byName := map[string]variant{}
	for _, v := range all {
		byName[v.name] = v
	}
	var out []variant
	for _, name := range strings.Split(spec, ",") {
		v, ok := byName[strings.TrimSpace(name)]
		if !ok {
			return nil, fmt.Errorf("unknown variant %q", name)
		}
		out = append(out, v)
	}
	return out, nil
}

// box is a drawn rectangle.
type box struct{ x, y, psi, length, width float64 }

// trackState is what the scorer keeps per variant and track between frames.
type trackState struct {
	est      *l5tracks.BoxEstimator
	have     bool
	last     box
	lastAt   int64
	lastX    float64
	lastY    float64
	sumAbs   float64 // Σ|Δψ| over the track, visible (mod 180°)
	sumNet   float64 // Σ Δψ
	path     float64 // metres travelled by the tracked position
	lengths  []float64
	widths   []float64
	rows     int
	class    string
	estNanos int64
	calls    int
}

type rowAgg struct {
	rows, outsideAny, outside2pct int
	overwide                      int
	shareSum                      float64
	maxOut                        []float64
	pairs, kinematic, rest, jump  int
	restPairs                     int
	sideslip                      []float64
	efficiency, axisErr           []float64
	slip                          []float64
	movingRows                    int
}

type scorer struct {
	variants  []variant
	tolerance float64
	frames    int
	tracks    map[string]bool
	class     map[string]string
	state     map[string]map[string]*trackState // variant → track → state
	rows      *csv.Writer
	// agg holds per variant, per track, the per-row results; groups are
	// assembled at the end from each track's final class.
	agg map[string]map[string]*rowAgg
}

const published = "published"

func newScorer(vs []variant, tolerance float64) *scorer {
	s := &scorer{variants: vs, tolerance: tolerance, tracks: map[string]bool{}, class: map[string]string{},
		state: map[string]map[string]*trackState{}, agg: map[string]map[string]*rowAgg{}}
	for _, name := range s.names() {
		s.state[name] = map[string]*trackState{}
		s.agg[name] = map[string]*rowAgg{}
	}
	return s
}

func (s *scorer) names() []string {
	out := []string{published}
	for _, v := range s.variants {
		out = append(out, v.name)
	}
	return out
}

var rowHeader = []string{"variant", "track", "t_ns", "class", "speed", "x", "y", "psi_deg", "length", "width",
	"points", "outside", "max_out", "course_deg", "dpsi_deg", "span_a", "span_n", "ref_axis_deg", "ref_area"}

func (s *scorer) frame(f replayeval.TrackerTapFrame) {
	clusterOf := map[string]int{}
	for i, id := range f.Associations {
		if id != "" {
			clusterOf[id] = i
		}
	}
	// A track born this frame is not in the associations: it took an
	// unassociated cluster, which its LastClusterID names.
	byID := map[int64]int{}
	for i := range f.Clusters {
		byID[f.Clusters[i].ClusterID] = i
	}
	for _, t := range f.Tracks {
		if _, ok := clusterOf[t.TrackID]; !ok && t.ObservationCount == 1 && t.LastObservedUnixNanos == f.At.UnixNano() {
			if i, ok := byID[t.LastClusterID]; ok {
				clusterOf[t.TrackID] = i
			}
		}
	}
	now := f.At.UnixNano()
	live := map[string]bool{}
	if f.Scored {
		s.frames++
	}
	for _, t := range f.Tracks {
		live[t.TrackID] = true
		if t.ObjectClass != "" {
			s.class[t.TrackID] = t.ObjectClass
		}
		var cluster *l5tracks.WorldCluster
		if i, ok := clusterOf[t.TrackID]; ok && i < len(f.Clusters) {
			cluster = &f.Clusters[i]
		}
		in := l5tracks.BoxInput{Nanos: now, X: float64(t.X), Y: float64(t.Y), VX: float64(t.VX), VY: float64(t.VY),
			VelCov: [3]float64{float64(t.P[10]), float64(t.P[11]), float64(t.P[15])}, Cluster: cluster,
			Merge: t.MergeCandidate}
		// Every variant follows every track from birth, warm-up included, so
		// a scored frame's box has the history it would have had live.
		boxes := map[string]box{}
		boxes[published] = publishedBox(t)
		for _, v := range s.variants {
			st := s.trackState(v.name, t.TrackID)
			if st.est == nil {
				st.est = l5tracks.NewBoxEstimator(v.cfg)
			}
			started := time.Now()
			b := st.est.Observe(in)
			st.estNanos += time.Since(started).Nanoseconds()
			st.calls++
			boxes[v.name] = box{b.X, b.Y, b.HeadingRad, b.Length, b.Width}
		}
		if !f.Scored || t.TrackState != l5tracks.TrackConfirmed {
			// Keep the continuity state current so the first scored pair is
			// measured against the previous frame, not the track's birth.
			for name, b := range boxes {
				st := s.trackState(name, t.TrackID)
				st.have, st.last, st.lastAt = b.length > 0, b, now
				st.lastX, st.lastY = float64(t.X), float64(t.Y)
			}
			continue
		}
		s.tracks[t.TrackID] = true
		ref := minAreaRect(cluster)
		for name, b := range boxes {
			s.score(name, t, b, cluster, ref, now)
		}
	}
	for _, name := range s.names() {
		for id := range s.state[name] {
			if !live[id] {
				delete(s.state[name], id)
			}
		}
	}
}

func publishedBox(t *l5tracks.TrackedObject) box {
	if t.BoxLength > 0 {
		return box{float64(t.BoxX), float64(t.BoxY), float64(t.BoxHeadingRad), float64(t.BoxLength), float64(t.BoxWidth)}
	}
	return box{float64(t.X), float64(t.Y), float64(t.OBBHeadingRad), float64(t.OBBLength), float64(t.OBBWidth)}
}

func (s *scorer) trackState(name, id string) *trackState {
	st := s.state[name][id]
	if st == nil {
		st = &trackState{}
		s.state[name][id] = st
	}
	return st
}

// Evaluation bounds. They are the scorer's, independent of any variant's
// configuration: a car turns no tighter than about a 4 m radius, and a
// heading that changes by more than 10°/s beyond that is not a vehicle's.
const (
	evalCurvature     = 0.25
	evalYawSlackRad   = 10 * math.Pi / 180
	evalRestSpeed     = 0.5
	evalRestTurnRad   = 1 * math.Pi / 180
	evalJumpRad       = 30 * math.Pi / 180
	evalSideslipSpeed = 3.0
	evalMaxPairDt     = 0.25
	// evalOverwideMetres is wider than any road vehicle is allowed to be
	// (2.6 m in the US and the EU) with a margin: a box whose shorter side
	// exceeds it is drawn across a merge or along a wrong axis.
	evalOverwideMetres = 2.8
)

func (s *scorer) score(name string, t *l5tracks.TrackedObject, b box, cluster *l5tracks.WorldCluster, ref areaRef, now int64) {
	st := s.trackState(name, t.TrackID)
	agg := s.agg[name][t.TrackID]
	if agg == nil {
		agg = &rowAgg{}
		s.agg[name][t.TrackID] = agg
	}
	speed := math.Hypot(float64(t.VX), float64(t.VY))
	course := math.Atan2(float64(t.VY), float64(t.VX))
	points, outside, maxOut := 0, 0, 0.0
	spanA, spanN := 0.0, 0.0
	if cluster != nil && b.length > 0 {
		members := cluster.Members
		if len(members) == 0 {
			members = cluster.RetainedPoints
		}
		c, sn := math.Cos(b.psi), math.Sin(b.psi)
		aLo, aHi, nLo, nHi := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for _, p := range members {
			dx, dy := p.X-b.x, p.Y-b.y
			a, n := dx*c+dy*sn, -dx*sn+dy*c
			aLo, aHi, nLo, nHi = math.Min(aLo, a), math.Max(aHi, a), math.Min(nLo, n), math.Max(nHi, n)
			da, dn := math.Abs(a)-b.length/2, math.Abs(n)-b.width/2
			d := math.Hypot(math.Max(da, 0), math.Max(dn, 0))
			if d > s.tolerance {
				outside++
			}
			maxOut = math.Max(maxOut, d)
		}
		points = len(members)
		if points > 0 {
			spanA, spanN = aHi-aLo, nHi-nLo
			if ref.ok && spanA*spanN > 0 {
				agg.efficiency = append(agg.efficiency, math.Min(ref.area/(spanA*spanN), 1))
				if ref.axisOK {
					agg.axisErr = append(agg.axisErr, foldQuarterAbs(b.psi-ref.axis)*180/math.Pi)
				}
			}
			agg.rows++
			if outside > 0 {
				agg.outsideAny++
			}
			share := float64(outside) / float64(points)
			if share > 0.02 {
				agg.outside2pct++
			}
			agg.shareSum += share
			agg.maxOut = append(agg.maxOut, maxOut)
			if math.Min(b.length, b.width) > evalOverwideMetres {
				agg.overwide++
			}
			st.lengths = append(st.lengths, b.length)
			st.widths = append(st.widths, b.width)
			st.rows++
		}
	}
	if speed >= evalSideslipSpeed && b.length > 0 {
		agg.movingRows++
		agg.sideslip = append(agg.sideslip, math.Abs(foldHalf(b.psi-course))*180/math.Pi)
	}
	dpsi := math.NaN()
	if st.have && b.length > 0 {
		dt := float64(now-st.lastAt) / 1e9
		if dt > 0 && dt <= evalMaxPairDt {
			d := foldHalf(b.psi - st.last.psi)
			dpsi = d * 180 / math.Pi
			agg.pairs++
			if math.Abs(d)/dt > evalCurvature*speed+evalYawSlackRad {
				agg.kinematic++
			}
			if speed < evalRestSpeed {
				agg.restPairs++
				if math.Abs(d) > evalRestTurnRad {
					agg.rest++
				}
			}
			if math.Abs(d) > evalJumpRad {
				agg.jump++
			}
			// The box centre's motion that the track's does not explain.
			agg.slip = append(agg.slip, math.Hypot((b.x-st.last.x)-(float64(t.X)-st.lastX), (b.y-st.last.y)-(float64(t.Y)-st.lastY)))
			st.sumAbs += math.Abs(d)
			st.sumNet += d
			st.path += math.Hypot(float64(t.X)-st.lastX, float64(t.Y)-st.lastY)
		}
	}
	if s.rows != nil {
		_ = s.rows.Write([]string{name, t.TrackID, strconv.FormatInt(now, 10), s.class[t.TrackID],
			f3(speed), f3(b.x), f3(b.y), f3(b.psi * 180 / math.Pi), f3(b.length), f3(b.width),
			strconv.Itoa(points), strconv.Itoa(outside), f3(maxOut), f3(course * 180 / math.Pi), f3(dpsi), f3(spanA), f3(spanN), refAxis(ref), f3(ref.area)})
	}
	st.have, st.last, st.lastAt = b.length > 0, b, now
	st.lastX, st.lastY = float64(t.X), float64(t.Y)
	st.class = s.class[t.TrackID]
	// Tracks that end are summarised at the end from the per-track record.
	s.keep(name, t.TrackID, st)
}

// kept holds each scored track's continuity record after the track ends.
var kept = map[string]map[string]*trackState{}

func (s *scorer) keep(name, id string, st *trackState) {
	if kept[name] == nil {
		kept[name] = map[string]*trackState{}
	}
	kept[name][id] = st
}

func f3(v float64) string {
	if math.IsNaN(v) {
		return ""
	}
	return strconv.FormatFloat(v, 'f', 3, 64)
}

func foldHalf(a float64) float64 { return 0.5 * math.Atan2(math.Sin(2*a), math.Cos(2*a)) }

// Summary is the tool's output.
type Summary struct {
	Case            string                         `json:"case"`
	Captures        []string                       `json:"captures"`
	Tuning          string                         `json:"tuning"`
	Recording       string                         `json:"-"`
	Build           string                         `json:"build"`
	WarmupSeconds   float64                        `json:"warmup_seconds"`
	DurationSeconds float64                        `json:"duration_seconds"`
	Tolerance       float64                        `json:"tolerance_metres"`
	Frames          int                            `json:"scored_frames"`
	Tracks          int                            `json:"confirmed_tracks"`
	WallSeconds     float64                        `json:"wall_seconds"`
	Variants        map[string]map[string]*Measure `json:"variants"`
	Definitions     map[string]string              `json:"definitions"`
}

// Measure is one variant's figures over one group of tracks.
type Measure struct {
	Tracks             int     `json:"tracks"`
	Rows               int     `json:"rows"`
	OutsideAnyShare    float64 `json:"outside_any_share"`
	Outside2pctShare   float64 `json:"outside_2pct_share"`
	OutsidePointsShare float64 `json:"outside_points_share"`
	MaxOutP50          float64 `json:"max_out_p50_m"`
	MaxOutP90          float64 `json:"max_out_p90_m"`
	MaxOutP99          float64 `json:"max_out_p99_m"`
	OverwideShare      float64 `json:"overwide_share"`
	Pairs              int     `json:"pairs"`
	KinematicShare     float64 `json:"kinematic_violation_share"`
	RestPairs          int     `json:"rest_pairs"`
	RestTurnShare      float64 `json:"rest_turn_share"`
	JumpShare          float64 `json:"jump_share"`
	SideslipRows       int     `json:"sideslip_rows"`
	SideslipP50        float64 `json:"sideslip_p50_deg"`
	SideslipP90        float64 `json:"sideslip_p90_deg"`
	SideslipOver15     float64 `json:"sideslip_over15_share"`
	EfficiencyP10      float64 `json:"area_efficiency_p10"`
	EfficiencyP50      float64 `json:"area_efficiency_p50"`
	EfficiencyUnder80  float64 `json:"area_efficiency_under80_share"`
	AxisRows           int     `json:"min_area_axis_rows"`
	SlipP50            float64 `json:"centre_slip_p50_m"`
	SlipP90            float64 `json:"centre_slip_p90_m"`
	SlipP99            float64 `json:"centre_slip_p99_m"`
	AxisErrP50         float64 `json:"min_area_axis_error_p50_deg"`
	AxisErrP90         float64 `json:"min_area_axis_error_p90_deg"`
	AxisErrOver10      float64 `json:"min_area_axis_error_over10_share"`
	WigglePer10mP50    float64 `json:"wiggle_deg_per_10m_p50"`
	WigglePer10mP90    float64 `json:"wiggle_deg_per_10m_p90"`
	LengthP50          float64 `json:"length_p50_m"`
	WidthP50           float64 `json:"width_p50_m"`
	LengthCVP50        float64 `json:"length_cv_p50"`
	MicrosPerUpdate    float64 `json:"micros_per_update,omitempty"`
}

func group(class string) string {
	switch class {
	case "car", "truck", "bus":
		return "vehicle"
	case "pedestrian":
		return "pedestrian"
	case "cyclist", "motorcyclist":
		return "cycle"
	}
	return "other"
}

func (s *scorer) summary() Summary {
	out := Summary{Frames: s.frames, Tracks: len(s.tracks), Variants: map[string]map[string]*Measure{},
		Definitions: map[string]string{
			"outside_any_share":         "share of scored rows (a confirmed track's associated frame) with any cluster member further than the tolerance outside the drawn box",
			"outside_2pct_share":        "share of scored rows with more than 2 % of members outside by the tolerance",
			"outside_points_share":      "mean over rows of the share of members outside by the tolerance",
			"max_out":                   "distance of the farthest member outside the box, per row (0 when all inside)",
			"kinematic_violation_share": "share of consecutive frame pairs whose drawn-box turn rate (modulo 180°) exceeds 0.25/m × speed + 10°/s",
			"rest_turn_share":           "share of pairs below 0.5 m/s whose drawn box turned by more than 1°",
			"jump_share":                "share of pairs whose drawn box turned by more than 30° in one frame",
			"sideslip":                  "angle between the drawn box's axis and the direction of travel, modulo 180°, rows at 3 m/s or more",
			"wiggle_deg_per_10m":        "per track, (Σ|Δψ| − |ΣΔψ|) per 10 m travelled: turning back and forth not explained by the net turn; tracks over 10 m",
			"length_cv":                 "per track, coefficient of variation of the drawn length",
			"overwide_share":            "share of scored rows whose drawn box is wider than 2.8 m on its shorter side",
			"area_efficiency":           "per row, the area of the members' minimum-area rectangle over the area of their bounding rectangle along the drawn heading: 1 when the box is turned to the points' own axis, lower the further it is turned off it; independent of the course and of the rectangle fit",
			"centre_slip":               "per consecutive pair, the drawn box centre's displacement less the tracked position's: how far the box moves on the track",
			"min_area_axis_error":       "per row with 30 or more members and a minimum-area rectangle of aspect 1.5 or more, the drawn axis's distance from that rectangle's axis, modulo 90°",
		}}
	for _, name := range s.names() {
		groups := map[string][]string{}
		for id := range s.agg[name] {
			g := group(s.class[id])
			groups[g] = append(groups[g], id)
			groups["all"] = append(groups["all"], id)
		}
		out.Variants[name] = map[string]*Measure{}
		for g, ids := range groups {
			sort.Strings(ids)
			m := &Measure{Tracks: len(ids)}
			var maxOut, side, wiggle, lengths, widths, cvs, eff, axisErr, slip []float64
			var share float64
			var estNanos int64
			var calls int
			for _, id := range ids {
				a := s.agg[name][id]
				m.Rows += a.rows
				m.OutsideAnyShare += float64(a.outsideAny)
				m.Outside2pctShare += float64(a.outside2pct)
				m.OverwideShare += float64(a.overwide)
				share += a.shareSum
				maxOut = append(maxOut, a.maxOut...)
				m.Pairs += a.pairs
				m.KinematicShare += float64(a.kinematic)
				m.RestPairs += a.restPairs
				m.RestTurnShare += float64(a.rest)
				m.JumpShare += float64(a.jump)
				side = append(side, a.sideslip...)
				eff = append(eff, a.efficiency...)
				axisErr = append(axisErr, a.axisErr...)
				slip = append(slip, a.slip...)
				if st := kept[name][id]; st != nil {
					if st.path >= 10 {
						wiggle = append(wiggle, (st.sumAbs-math.Abs(st.sumNet))*180/math.Pi/(st.path/10))
					}
					if len(st.lengths) > 0 {
						l := median(st.lengths)
						lengths = append(lengths, l)
						widths = append(widths, median(st.widths))
						if l > 0 {
							cvs = append(cvs, stddev(st.lengths)/mean(st.lengths))
						}
					}
					estNanos += st.estNanos
					calls += st.calls
				}
			}
			if m.Rows > 0 {
				m.OutsideAnyShare /= float64(m.Rows)
				m.Outside2pctShare /= float64(m.Rows)
				m.OverwideShare /= float64(m.Rows)
				m.OutsidePointsShare = share / float64(m.Rows)
			}
			if m.Pairs > 0 {
				m.KinematicShare /= float64(m.Pairs)
				m.JumpShare /= float64(m.Pairs)
			}
			if m.RestPairs > 0 {
				m.RestTurnShare /= float64(m.RestPairs)
			}
			m.MaxOutP50, m.MaxOutP90, m.MaxOutP99 = quantile(maxOut, .5), quantile(maxOut, .9), quantile(maxOut, .99)
			m.SideslipRows = len(side)
			m.SideslipP50, m.SideslipP90 = quantile(side, .5), quantile(side, .9)
			over := 0
			for _, v := range side {
				if v > 15 {
					over++
				}
			}
			if len(side) > 0 {
				m.SideslipOver15 = float64(over) / float64(len(side))
			}
			m.WigglePer10mP50, m.WigglePer10mP90 = quantile(wiggle, .5), quantile(wiggle, .9)
			m.EfficiencyP10, m.EfficiencyP50 = quantile(eff, .1), quantile(eff, .5)
			m.EfficiencyUnder80 = shareBelow(eff, 0.8)
			m.AxisRows = len(axisErr)
			m.SlipP50, m.SlipP90, m.SlipP99 = quantile(slip, .5), quantile(slip, .9), quantile(slip, .99)
			m.AxisErrP50, m.AxisErrP90 = quantile(axisErr, .5), quantile(axisErr, .9)
			m.AxisErrOver10 = 1 - shareBelow(axisErr, 10.000001)
			if len(axisErr) == 0 {
				m.AxisErrOver10 = 0
			}
			m.LengthP50, m.WidthP50, m.LengthCVP50 = quantile(lengths, .5), quantile(widths, .5), quantile(cvs, .5)
			if name != published && calls > 0 {
				m.MicrosPerUpdate = float64(estNanos) / 1e3 / float64(calls)
			}
			out.Variants[name][g] = m
		}
	}
	return out
}

// quantile is zero for no values; the counts beside each figure say when.
func quantile(v []float64, q float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	i := int(math.Ceil(q*float64(len(s)))) - 1
	if i < 0 {
		i = 0
	}
	return s[i]
}

func median(v []float64) float64 { return quantile(v, .5) }

func mean(v []float64) float64 {
	sum := 0.0
	for _, x := range v {
		sum += x
	}
	return sum / float64(len(v))
}

func stddev(v []float64) float64 {
	m := mean(v)
	sum := 0.0
	for _, x := range v {
		sum += (x - m) * (x - m)
	}
	return math.Sqrt(sum / float64(len(v)))
}

// areaRef is a frame's minimum-area rectangle of the members: its area, and
// its axis when the rectangle is elongated enough to have one.
type areaRef struct {
	ok, axisOK bool
	area, axis float64
}

const (
	refMinPoints = 30
	refMinAspect = 1.5
)

// minAreaRect is the members' minimum-area enclosing rectangle, by rotating
// an edge of their convex hull: the optimum has a side on a hull edge.
func minAreaRect(cluster *l5tracks.WorldCluster) areaRef {
	if cluster == nil {
		return areaRef{}
	}
	members := cluster.Members
	if len(members) == 0 {
		members = cluster.RetainedPoints
	}
	if len(members) < 3 {
		return areaRef{}
	}
	pts := make([][2]float64, len(members))
	for i, p := range members {
		pts[i] = [2]float64{p.X, p.Y}
	}
	hull := convexHull(pts)
	if len(hull) < 3 {
		return areaRef{}
	}
	best := areaRef{area: math.Inf(1)}
	var bestA, bestN float64
	for i := range hull {
		j := (i + 1) % len(hull)
		theta := math.Atan2(hull[j][1]-hull[i][1], hull[j][0]-hull[i][0])
		c, s := math.Cos(theta), math.Sin(theta)
		aLo, aHi, nLo, nHi := math.Inf(1), math.Inf(-1), math.Inf(1), math.Inf(-1)
		for _, p := range hull {
			a, n := p[0]*c+p[1]*s, -p[0]*s+p[1]*c
			aLo, aHi, nLo, nHi = math.Min(aLo, a), math.Max(aHi, a), math.Min(nLo, n), math.Max(nHi, n)
		}
		if area := (aHi - aLo) * (nHi - nLo); area < best.area {
			best.area, best.axis = area, theta
			bestA, bestN = aHi-aLo, nHi-nLo
		}
	}
	if best.area <= 0 || math.IsInf(best.area, 0) {
		return areaRef{}
	}
	best.ok = true
	long, short := math.Max(bestA, bestN), math.Min(bestA, bestN)
	best.axisOK = len(members) >= refMinPoints && short > 0 && long/short >= refMinAspect
	return best
}

// convexHull is Andrew's monotone chain, counter-clockwise.
func convexHull(pts [][2]float64) [][2]float64 {
	sort.Slice(pts, func(i, j int) bool {
		if pts[i][0] != pts[j][0] {
			return pts[i][0] < pts[j][0]
		}
		return pts[i][1] < pts[j][1]
	})
	cross := func(o, a, b [2]float64) float64 {
		return (a[0]-o[0])*(b[1]-o[1]) - (a[1]-o[1])*(b[0]-o[0])
	}
	hull := make([][2]float64, 0, 2*len(pts))
	for _, p := range pts {
		for len(hull) >= 2 && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	lower := len(hull) + 1
	for i := len(pts) - 2; i >= 0; i-- {
		p := pts[i]
		for len(hull) >= lower && cross(hull[len(hull)-2], hull[len(hull)-1], p) <= 0 {
			hull = hull[:len(hull)-1]
		}
		hull = append(hull, p)
	}
	return hull[:len(hull)-1]
}

func foldQuarterAbs(a float64) float64 {
	return math.Abs(0.25 * math.Atan2(math.Sin(4*a), math.Cos(4*a)))
}

func shareBelow(v []float64, x float64) float64 {
	if len(v) == 0 {
		return 0
	}
	n := 0
	for _, y := range v {
		if y < x {
			n++
		}
	}
	return float64(n) / float64(len(v))
}

// refAxis is the row's minimum-area axis in degrees, or empty without one.
func refAxis(ref areaRef) string {
	if !ref.axisOK {
		return ""
	}
	return f3(ref.axis * 180 / math.Pi)
}
