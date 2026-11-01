package main

import (
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"runtime/pprof"
	"sort"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

const (
	oracleRings = 40
	oracleBins  = 1800
	oracleModes = 3
)

// oracleCell is a cell's persistent surfaces over a whole capture, found
// without any L3 state: the ranges at which the cell returned in a large share
// of the sampled frames. Hits is the share of its returns inside them.
type oracleCell struct {
	Modes   [oracleModes]float32
	Support [oracleModes]float32
	Hits    float32
	Count   uint32
}

func modeWindow(r float32) float32 { return 0.2 + 0.005*r }

func cellOf(p *l2frames.Point) int {
	if p.Channel < 1 || p.Channel > oracleRings {
		return -1
	}
	az := math.Mod(p.Azimuth, 360)
	if az < 0 {
		az += 360
	}
	return (p.Channel-1)*oracleBins + int(az/360*oracleBins)%oracleBins
}

func findModes(ranges []float32, frames int, minSupport float64) oracleCell {
	c := oracleCell{Count: uint32(len(ranges))}
	if len(ranges) == 0 {
		return c
	}
	sort.Slice(ranges, func(i, j int) bool { return ranges[i] < ranges[j] })
	used := make([]bool, len(ranges))
	inModes := 0
	for m := 0; m < oracleModes; m++ {
		best, bestStart, bestEnd := 0, 0, 0
		end := 0
		for start := range ranges {
			if end < start {
				end = start
			}
			w := 2 * modeWindow(ranges[start])
			for end < len(ranges) && ranges[end]-ranges[start] <= w {
				end++
			}
			n := 0
			for k := start; k < end; k++ {
				if !used[k] {
					n++
				}
			}
			if n > best {
				best, bestStart, bestEnd = n, start, end
			}
		}
		support := float64(best) / float64(frames)
		if support < minSupport {
			break
		}
		mid := (bestStart + bestEnd) / 2
		c.Modes[m] = ranges[mid]
		c.Support[m] = float32(support)
		inModes += best
		for k := bestStart; k < bestEnd; k++ {
			used[k] = true
		}
	}
	c.Hits = float32(inModes) / float32(len(ranges))
	return c
}

func runOracle(args []string) error {
	fs := flag.NewFlagSet("oracle", flag.ExitOnError)
	pcap := fs.String("pcap", "", "capture")
	tuning := fs.String("tuning", "config/tuning.defaults.json", "tuning file; only frames are read")
	start := fs.Float64("start", 0, "capture second")
	duration := fs.Float64("duration", 0, "capture seconds; zero is all")
	every := fs.Int("every", 3, "sample every Nth frame")
	maxFrames := fs.Int("max-frames", 1200, "frames kept per cell")
	support := fs.Float64("min-support", 0.08, "a surface returns in at least this share of frames")
	replayOut := fs.String("replay-out", "", "new replay output directory")
	out := fs.String("out", "", "oracle file")
	fs.Parse(args)
	cells := oracleRings * oracleBins
	store := make([]float32, cells**maxFrames)
	counts := make([]int32, cells)
	frames, index := 0, 0
	seen := make([]uint32, cells)
	_, err := replayeval.Run(replayeval.Config{PCAPFile: *pcap, OutDir: *replayOut, TuningFile: *tuning, SensorID: "hesai-pandar40p",
		UDPPort: 2369, StartSeconds: *start, DurationSeconds: *duration, FrameTap: func(f replayeval.FrameTapFrame) {
			index++
			if index%*every != 0 || frames >= *maxFrames || !f.Frame.SpinComplete {
				return
			}
			frames++
			for i := range f.Frame.Points {
				p := &f.Frame.Points[i]
				c := cellOf(p)
				if c < 0 || seen[c] == uint32(frames) {
					continue
				}
				seen[c] = uint32(frames)
				store[c**maxFrames+int(counts[c])] = float32(p.Distance)
				counts[c]++
			}
		}})
	if err != nil {
		return err
	}
	result := make([]oracleCell, cells)
	for c := range result {
		result[c] = findModes(store[c**maxFrames:c**maxFrames+int(counts[c])], frames, *support)
	}
	w, err := os.Create(*out)
	if err != nil {
		return err
	}
	defer w.Close()
	if err := binary.Write(w, binary.LittleEndian, result); err != nil {
		return err
	}
	fmt.Printf("oracle frames=%d cells-with-returns=%d\n", frames, nonEmpty(result))
	return nil
}

func nonEmpty(cells []oracleCell) int {
	n := 0
	for i := range cells {
		if cells[i].Count > 0 {
			n++
		}
	}
	return n
}

func loadOracle(path string) ([]oracleCell, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	cells := make([]oracleCell, oracleRings*oracleBins)
	return cells, binary.Read(f, binary.LittleEndian, cells)
}

const (
	labelUnknown = iota
	labelStatic
	labelTransient
)

// oracleLabel: static within a persistent surface, transient in front of the
// farthest one in a cell whose returns are mostly persistent, unknown otherwise
// (vegetation, sky cells, returns beyond every surface).
func oracleLabel(c *oracleCell, r float32) uint8 {
	if c.Count == 0 || c.Modes[0] == 0 {
		return labelUnknown
	}
	far := float32(0)
	for m := 0; m < oracleModes; m++ {
		if c.Modes[m] == 0 {
			continue
		}
		d := r - c.Modes[m]
		if d < 0 {
			d = -d
		}
		if d <= 1.5*modeWindow(c.Modes[m]) {
			return labelStatic
		}
		if c.Modes[m] > far {
			far = c.Modes[m]
		}
	}
	if c.Hits < 0.6 || r > far {
		return labelUnknown
	}
	return labelTransient
}

// structureSupport is the share of frames a surface must return in to count as
// structure rather than something that stood for a while, such as a queue.
const structureSupport = 0.5

// onStructure reports whether r lies on a surface the cell returns from in most
// frames: a wall or the road, not a vehicle stopped at a light.
func onStructure(c *oracleCell, r float32) bool {
	for m := 0; m < oracleModes; m++ {
		if c.Modes[m] == 0 || c.Support[m] < structureSupport {
			continue
		}
		d := r - c.Modes[m]
		if d < 0 {
			d = -d
		}
		if d <= 1.5*modeWindow(c.Modes[m]) {
			return true
		}
	}
	return false
}

type sample struct {
	t            float64
	x, y         float32
	ox, oy       float32
	sx, sy       float32 // the track's filtered position
	mx, my       float32 // coordinate median
	tx, ty       float32 // trimmed mean, middle half of each axis
	points       int
	length, wide float32
}

// robustCentres is the cluster's coordinate median and the trimmed mean of the
// middle half of each axis, from its members; the medoid without them.
func robustCentres(c *l5tracks.WorldCluster) (mx, my, tx, ty float32) {
	n := len(c.Members)
	if n == 0 {
		return c.CentroidX, c.CentroidY, c.CentroidX, c.CentroidY
	}
	xs, ys := make([]float64, n), make([]float64, n)
	for i := range c.Members {
		xs[i], ys[i] = c.Members[i].X, c.Members[i].Y
	}
	sort.Float64s(xs)
	sort.Float64s(ys)
	trim := func(v []float64) float32 {
		a, b := len(v)/4, len(v)-len(v)/4
		if b <= a {
			return float32(v[len(v)/2])
		}
		sum := 0.0
		for _, x := range v[a:b] {
			sum += x
		}
		return float32(sum / float64(b-a))
	}
	return float32(xs[n/2]), float32(ys[n/2]), trim(xs), trim(ys)
}

type trackLog struct {
	id        string
	confirmed bool
	obs       []sample
}

type runScore struct {
	Arm                string  `json:"arm"`
	Frames             int     `json:"frames"`
	MeanForeground     float64 `json:"fg_points"`
	MeanL3Foreground   float64 `json:"l3_fg_points"`
	PaintedPerFrame    float64 `json:"painted_per_frame"`
	PaintedStructure   float64 `json:"painted_structure_per_frame"`
	PaintedShare       float64 `json:"painted_share"`
	TransientPerFrame  float64 `json:"transient_coherent_per_frame"`
	TransientRecall    float64 `json:"transient_recall_published"`
	TransientRecallL3  float64 `json:"transient_recall_l3"`
	ClustersPerFrame   float64 `json:"clusters_per_frame"`
	UnassociatedFrame  float64 `json:"unassociated_per_frame"`
	Tracks             int     `json:"tracks_created"`
	Confirmed          int     `json:"tracks_confirmed"`
	Moving             int     `json:"moving_tracks"`
	Stationary         int     `json:"stationary_tracks"`
	MovingSeconds      float64 `json:"moving_track_seconds"`
	MedianMovingLife   float64 `json:"median_moving_life_s"`
	P90MovingLife      float64 `json:"p90_moving_life_s"`
	SplitShare         float64 `json:"split_track_frame_share"`
	DuplicatePairs     int     `json:"duplicate_track_pairs"`
	JitterNear         float64 `json:"jitter_near_m"`
	JitterMid          float64 `json:"jitter_mid_m"`
	JitterFar          float64 `json:"jitter_far_m"`
	OBBJitterMedian    float64 `json:"obb_jitter_median_m"`
	StateJitter        float64 `json:"track_state_jitter_m"`
	SpeedJitter        float64 `json:"track_speed_jitter_mps"`
	MedianJitter       float64 `json:"median_centre_jitter_m"`
	TrimmedJitter      float64 `json:"trimmed_centre_jitter_m"`
	LongTracks         int     `json:"tracks_over_20m"`
	MedoidJitterMedian float64 `json:"medoid_jitter_median_m"`
	MedoidJitterP90    float64 `json:"medoid_jitter_p90_m"`
	CrossJitterMedian  float64 `json:"cross_jitter_median_m"`
	LengthJitterMedian float64 `json:"length_jitter_median_m"`
	MedianPoints       float64 `json:"median_points_per_obs"`
	ObservedShare      float64 `json:"observed_share"`
	BrokenLinks        int     `json:"broken_links"`
	ElapsedSeconds     float64 `json:"elapsed_s"`
	// Confusion is mean returns per frame by rule and oracle label: rule name,
	// then static, transient (coherent), lone transient, unknown.
	Confusion map[string][4]float64 `json:"confusion"`
}

var codeNames = []string{"invalid", "frozen_matches", "frozen_diverges", "bg_locked", "bg_base", "bg_widened",
	"bg_neighbour", "bg_deadlock", "bg_seed", "fg", "fg_froze", "bg_far", "fg_grown"}

const heldOffset = 13

func codeIndex(d l3grid.DecisionRecord) int {
	if d.Held {
		return int(d.Code) + heldOffset
	}
	return int(d.Code)
}

// jitter fits a quadratic to each observation's window of neighbours and returns
// the RMS of the middle sample's residual, overall and across the local heading.
func jitter(obs []sample) (all, cross float64, ok bool) {
	const half = 5
	if len(obs) < 2*half+1 {
		return 0, 0, false
	}
	var sum, sumCross float64
	n := 0
	for i := half; i < len(obs)-half; i++ {
		w := obs[i-half : i+half+1]
		if w[len(w)-1].t-w[0].t > 2.0 {
			continue
		}
		fx, sx := quadMiddle(w, func(s sample) float64 { return float64(s.x) })
		fy, sy := quadMiddle(w, func(s sample) float64 { return float64(s.y) })
		rx, ry := float64(obs[i].x)-fx, float64(obs[i].y)-fy
		speed := math.Hypot(sx, sy)
		sum += rx*rx + ry*ry
		if speed > 0.1 {
			c := (-sy*rx + sx*ry) / speed
			sumCross += c * c
		}
		n++
	}
	if n == 0 {
		return 0, 0, false
	}
	return math.Sqrt(sum / float64(n)), math.Sqrt(sumCross / float64(n)), true
}

// quadMiddle least-squares fits a quadratic through the window, excluding the
// middle sample, and returns its value and slope at the middle.
func quadMiddle(w []sample, get func(sample) float64) (value, slope float64) {
	t0 := w[len(w)/2].t
	var a [3][4]float64
	for i, s := range w {
		if i == len(w)/2 {
			continue
		}
		t := s.t - t0
		b := [3]float64{1, t, t * t}
		y := get(s)
		for r := 0; r < 3; r++ {
			for c := 0; c < 3; c++ {
				a[r][c] += b[r] * b[c]
			}
			a[r][3] += b[r] * y
		}
	}
	for c := 0; c < 3; c++ {
		p := c
		for r := c + 1; r < 3; r++ {
			if math.Abs(a[r][c]) > math.Abs(a[p][c]) {
				p = r
			}
		}
		a[c], a[p] = a[p], a[c]
		if a[c][c] == 0 {
			return get(w[len(w)/2]), 0
		}
		for r := 0; r < 3; r++ {
			if r != c {
				f := a[r][c] / a[c][c]
				for k := c; k < 4; k++ {
					a[r][k] -= f * a[c][k]
				}
			}
		}
	}
	return a[0][3] / a[0][0], a[1][3] / a[1][1]
}

func percentile(v []float64, p float64) float64 {
	if len(v) == 0 {
		return 0
	}
	s := append([]float64(nil), v...)
	sort.Float64s(s)
	return s[int(p*float64(len(s)-1))]
}

func runScoreCmd(args []string) error {
	fs := flag.NewFlagSet("score", flag.ExitOnError)
	pcap := fs.String("pcap", "", "capture")
	tuning := fs.String("tuning", "config/tuning.defaults.json", "tuning file")
	experiment := fs.String("experiment", "", "comma-separated replay experiments")
	arm := fs.String("arm", "", "arm name")
	start := fs.Float64("start", 0, "capture second")
	warmup := fs.Float64("warmup", 0, "warm-up seconds before start")
	duration := fs.Float64("duration", 0, "capture seconds; zero is all")
	skip := fs.Float64("skip", 8, "seconds from the first frame not scored")
	oraclePath := fs.String("oracle", "", "oracle file")
	replayOut := fs.String("replay-out", "", "new replay output directory")
	out := fs.String("json", "", "score JSON")
	tracksOut := fs.String("tracks", "", "per-track JSON")
	measurement := fs.String("measurement", "", "L5 measurement source (medoid_v0, obb_centre_v1)")
	cpuProfile := fs.String("cpuprofile", "", "write a CPU profile of the replay here")
	series := fs.String("series", "", "per-minute CSV: frames, foreground, structure paint, coherent transient and kept")
	fs.Parse(args)
	oracle, err := loadOracle(*oraclePath)
	if err != nil {
		return err
	}
	var first time.Time
	score := runScore{Arm: *arm}
	var paintedStructure float64
	var fg, l3, painted, transient, kept, keptL3, clusters, unassociated float64
	var confusion [26][4]float64
	labels := []uint8(nil)
	stamp := make([]int32, oracleRings*oracleBins)
	rangeAt := make([]float32, oracleRings*oracleBins)
	frameNo := int32(0)

	cfg := replayeval.Config{PCAPFile: *pcap, OutDir: *replayOut, TuningFile: *tuning, SensorID: "hesai-pandar40p", UDPPort: 2369,
		StartSeconds: *start, WarmupSeconds: *warmup, DurationSeconds: *duration, FrameTapDecisions: true, KeepClusterMembers: true}
	if *experiment != "" {
		cfg.Experiments = strings.Split(*experiment, ",")
	}
	if *measurement != "" {
		cfg.MeasurementSourceMode = l5tracks.MeasurementSource(*measurement)
	}
	perMinute := map[int]*[5]float64{}
	logs := map[string]*trackLog{}
	var trackFrames, splitFrames float64
	overlapFrames := map[[2]string]int{}
	var order []*trackLog
	cfg.FrameTap = func(f replayeval.FrameTapFrame) {
		if first.IsZero() {
			first = f.At
		}
		if f.At.Sub(first).Seconds() < *skip {
			return
		}
		score.Frames++
		frameNo++
		pts := f.Frame.Points
		minute := int(f.At.Sub(first).Minutes())
		tally := perMinute[minute]
		if tally == nil {
			tally = &[5]float64{}
			perMinute[minute] = tally
		}
		tally[0]++
		if cap(labels) < len(pts) {
			labels = make([]uint8, len(pts))
		}
		labels = labels[:len(pts)]
		for i := range pts {
			c := cellOf(&pts[i])
			if c < 0 {
				labels[i] = labelUnknown
				continue
			}
			labels[i] = oracleLabel(&oracle[c], float32(pts[i].Distance))
			if labels[i] == labelTransient {
				if stamp[c] != frameNo || float32(pts[i].Distance) < rangeAt[c] {
					rangeAt[c] = float32(pts[i].Distance)
				}
				stamp[c] = frameNo
			}
		}
		for i := range pts {
			if labels[i] == labelStatic {
				confusion[codeIndex(f.Decisions[i])][0]++
			} else if labels[i] == labelUnknown {
				confusion[codeIndex(f.Decisions[i])][3]++
			}
			if f.Mask[i] {
				fg++
				tally[1]++
			}
			if f.L3Mask[i] {
				l3++
			}
			if labels[i] == labelStatic && f.Mask[i] {
				painted++
				if c := cellOf(&pts[i]); c >= 0 && onStructure(&oracle[c], float32(pts[i].Distance)) {
					paintedStructure++
					tally[2]++
				}
			}
			if labels[i] != labelTransient {
				continue
			}
			c := cellOf(&pts[i])
			ring, bin := c/oracleBins, c%oracleBins
			n := 0
			r := float32(pts[i].Distance)
			for dr := -2; dr <= 2; dr++ {
				for db := -4; db <= 4; db++ {
					if dr == 0 && db == 0 || ring+dr < 0 || ring+dr >= oracleRings {
						continue
					}
					k := (ring+dr)*oracleBins + (bin+db+oracleBins)%oracleBins
					if stamp[k] == frameNo {
						d := rangeAt[k] - r
						if d < 0 {
							d = -d
						}
						if d < 1 {
							n++
						}
					}
				}
			}
			if n < 3 {
				confusion[codeIndex(f.Decisions[i])][2]++
				continue
			}
			confusion[codeIndex(f.Decisions[i])][1]++
			transient++
			tally[3]++
			if f.Mask[i] {
				kept++
				tally[4]++
			}
			if f.L3Mask[i] {
				keptL3++
			}
		}
	}
	cfg.TrackerTap = func(f replayeval.TrackerTapFrame) {
		if first.IsZero() {
			return
		}
		sec := f.At.Sub(first).Seconds()
		if sec < *skip {
			return
		}
		clusters += float64(len(f.Clusters))
		byID := make(map[string]*l5tracks.TrackedObject, len(f.Tracks))
		for _, t := range f.Tracks {
			byID[t.TrackID] = t
		}
		var moving []*l5tracks.TrackedObject
		for _, t := range f.Tracks {
			if t.TrackState == l5tracks.TrackConfirmed && t.Misses == 0 && t.Speed() >= 1 {
				moving = append(moving, t)
			}
		}
		for _, t := range moving {
			b := t.PublishedBox()
			sin, cos := math.Sincos(float64(b.HeadingRad))
			in := 0
			for i := range f.Clusters {
				dx, dy := float64(f.Clusters[i].CentroidX-b.X), float64(f.Clusters[i].CentroidY-b.Y)
				if math.Abs(dx*cos+dy*sin) <= float64(b.Length)/2+0.5 && math.Abs(-dx*sin+dy*cos) <= float64(b.Width)/2+0.5 {
					in++
				}
			}
			trackFrames++
			if in > 1 {
				splitFrames++
			}
			for _, o := range moving {
				if o.TrackID <= t.TrackID {
					continue
				}
				dx, dy := float64(o.X-b.X), float64(o.Y-b.Y)
				if math.Abs(dx*cos+dy*sin) <= float64(b.Length)/2 && math.Abs(-dx*sin+dy*cos) <= float64(b.Width)/2+0.3 {
					overlapFrames[[2]string{t.TrackID, o.TrackID}]++
				}
			}
		}
		for i, c := range f.Clusters {
			id := ""
			if i < len(f.Associations) {
				id = f.Associations[i]
			}
			if id == "" {
				unassociated++
				continue
			}
			l := logs[id]
			if l == nil {
				l = &trackLog{id: id}
				logs[id] = l
				order = append(order, l)
			}
			ox, oy := c.CentroidX, c.CentroidY
			if c.OBB != nil {
				ox, oy = c.OBB.CenterX, c.OBB.CenterY
			}
			mx, my, tx, ty := robustCentres(&f.Clusters[i])
			sx, sy := c.CentroidX, c.CentroidY
			if t := byID[id]; t != nil {
				sx, sy = t.X, t.Y
			}
			l.obs = append(l.obs, sample{t: sec, x: c.CentroidX, y: c.CentroidY, ox: ox, oy: oy, sx: sx, sy: sy, mx: mx, my: my, tx: tx, ty: ty,
				points: c.PointsCount, length: c.BoundingBoxLength, wide: c.BoundingBoxWidth})
		}
		for _, t := range f.Tracks {
			if t.TrackState == l5tracks.TrackConfirmed {
				if l := logs[t.TrackID]; l != nil {
					l.confirmed = true
				}
			}
		}
	}
	if *cpuProfile != "" {
		f, err := os.Create(*cpuProfile)
		if err != nil {
			return err
		}
		defer f.Close()
		if err := pprof.StartCPUProfile(f); err != nil {
			return err
		}
		defer pprof.StopCPUProfile()
	}
	began := time.Now()
	if _, err := replayeval.Run(cfg); err != nil {
		return err
	}
	score.ElapsedSeconds = time.Since(began).Seconds()
	frames := float64(score.Frames)
	score.MeanForeground = fg / frames
	score.MeanL3Foreground = l3 / frames
	score.PaintedPerFrame = painted / frames
	score.PaintedStructure = paintedStructure / frames
	if fg > 0 {
		score.PaintedShare = painted / fg
	}
	score.TransientPerFrame = transient / frames
	if transient > 0 {
		score.TransientRecall = kept / transient
		score.TransientRecallL3 = keptL3 / transient
	}
	score.Confusion = map[string][4]float64{}
	for code, row := range confusion {
		for k := range row {
			row[k] /= frames
		}
		name := codeNames[code%heldOffset]
		if code >= heldOffset {
			name += "+held"
		}
		score.Confusion[name] = row
	}
	if trackFrames > 0 {
		score.SplitShare = splitFrames / trackFrames
	}
	for _, n := range overlapFrames {
		if n >= 10 {
			score.DuplicatePairs++
		}
	}
	score.ClustersPerFrame = clusters / frames
	score.UnassociatedFrame = unassociated / frames

	score.Tracks = len(order)
	var lives, jits, cross, lengths, points, near, mid, far, obbs, medians, trimmed, states, speedJit []float64
	type span struct {
		first, last sample
	}
	var spans []span
	var observed, possible float64
	for _, l := range order {
		if !l.confirmed {
			continue
		}
		score.Confirmed++
		a, b := l.obs[0], l.obs[len(l.obs)-1]
		life := b.t - a.t
		net := math.Hypot(float64(b.x-a.x), float64(b.y-a.y))
		if net < 2 {
			score.Stationary++
			continue
		}
		if net < 5 || life < 1 {
			continue
		}
		score.Moving++
		score.MovingSeconds += life
		lives = append(lives, life)
		spans = append(spans, span{a, b})
		observed += float64(len(l.obs))
		possible += life*10 + 1
		if net >= 20 {
			score.LongTracks++
		}
		if j, c, ok := jitter(l.obs); ok {
			jits = append(jits, j)
			cross = append(cross, c)
			r := math.Hypot(float64(l.obs[len(l.obs)/2].x), float64(l.obs[len(l.obs)/2].y))
			switch {
			case r < 20:
				near = append(near, j)
			case r < 40:
				mid = append(mid, j)
			default:
				far = append(far, j)
			}
			centres := make([]sample, len(l.obs))
			for i, o := range l.obs {
				centres[i] = sample{t: o.t, x: o.ox, y: o.oy}
			}
			if o, _, ok := jitter(centres); ok {
				obbs = append(obbs, o)
			}
			for i, o := range l.obs {
				centres[i] = sample{t: o.t, x: o.sx, y: o.sy}
			}
			if o, _, ok := jitter(centres); ok {
				states = append(states, o)
			}
			var speeds []float64
			for i := 1; i+1 < len(l.obs); i++ {
				a, b, c := l.obs[i-1], l.obs[i], l.obs[i+1]
				if c.t-a.t > 0.5 {
					continue
				}
				v0 := math.Hypot(float64(b.sx-a.sx), float64(b.sy-a.sy)) / (b.t - a.t)
				v1 := math.Hypot(float64(c.sx-b.sx), float64(c.sy-b.sy)) / (c.t - b.t)
				speeds = append(speeds, math.Abs(v1-v0))
			}
			speedJit = append(speedJit, percentile(speeds, .5))
			for i, o := range l.obs {
				centres[i] = sample{t: o.t, x: o.mx, y: o.my}
			}
			if o, _, ok := jitter(centres); ok {
				medians = append(medians, o)
			}
			for i, o := range l.obs {
				centres[i] = sample{t: o.t, x: o.tx, y: o.ty}
			}
			if o, _, ok := jitter(centres); ok {
				trimmed = append(trimmed, o)
			}
		}
		var d []float64
		for i := 1; i < len(l.obs); i++ {
			d = append(d, math.Abs(float64(l.obs[i].length-l.obs[i-1].length)))
		}
		lengths = append(lengths, percentile(d, .5))
		var p []float64
		for _, o := range l.obs {
			p = append(p, float64(o.points))
		}
		points = append(points, percentile(p, .5))
	}
	for i := range spans {
		for j := range spans {
			if i == j {
				continue
			}
			dt := spans[j].first.t - spans[i].last.t
			if dt < -0.3 || dt > 2 {
				continue
			}
			gap := math.Hypot(float64(spans[j].first.x-spans[i].last.x), float64(spans[j].first.y-spans[i].last.y))
			if gap < 4+15*math.Max(dt, 0) {
				score.BrokenLinks++
				break
			}
		}
	}
	score.MedianMovingLife = percentile(lives, .5)
	score.P90MovingLife = percentile(lives, .9)
	score.JitterNear = percentile(near, .5)
	score.JitterMid = percentile(mid, .5)
	score.JitterFar = percentile(far, .5)
	score.OBBJitterMedian = percentile(obbs, .5)
	score.MedianJitter = percentile(medians, .5)
	score.StateJitter = percentile(states, .5)
	score.SpeedJitter = percentile(speedJit, .5)
	score.TrimmedJitter = percentile(trimmed, .5)
	score.MedoidJitterMedian = percentile(jits, .5)
	score.MedoidJitterP90 = percentile(jits, .9)
	score.CrossJitterMedian = percentile(cross, .5)
	score.LengthJitterMedian = percentile(lengths, .5)
	score.MedianPoints = percentile(points, .5)
	if possible > 0 {
		score.ObservedShare = observed / possible
	}
	if *series != "" {
		w, err := os.Create(*series)
		if err != nil {
			return err
		}
		defer w.Close()
		fmt.Fprintln(w, "minute,frames,foreground,structure_paint,transient,kept")
		minutes := make([]int, 0, len(perMinute))
		for m := range perMinute {
			minutes = append(minutes, m)
		}
		sort.Ints(minutes)
		for _, m := range minutes {
			b := perMinute[m]
			fmt.Fprintf(w, "%d,%.0f,%.0f,%.0f,%.0f,%.0f\n", m, b[0], b[1], b[2], b[3], b[4])
		}
	}
	if *tracksOut != "" {
		type trackRow struct {
			ID        string       `json:"id"`
			Confirmed bool         `json:"confirmed"`
			Samples   [][3]float64 `json:"samples"`
			Points    []int        `json:"points"`
		}
		rows := make([]trackRow, 0, len(order))
		for _, l := range order {
			r := trackRow{ID: l.id, Confirmed: l.confirmed}
			for _, o := range l.obs {
				r.Samples = append(r.Samples, [3]float64{o.t, float64(o.x), float64(o.y)})
				r.Points = append(r.Points, o.points)
			}
			rows = append(rows, r)
		}
		b, _ := json.Marshal(map[string]any{"first_unix_ns": first.UnixNano(), "tracks": rows})
		if err := os.WriteFile(*tracksOut, b, 0o644); err != nil {
			return err
		}
	}
	b, _ := json.MarshalIndent(score, "", " ")
	if *out != "" {
		if err := os.WriteFile(*out, b, 0o644); err != nil {
			return err
		}
	}
	fmt.Println(string(b))
	return nil
}
