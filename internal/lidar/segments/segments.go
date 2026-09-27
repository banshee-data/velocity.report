// Package segments ranks capture windows for annotation. Its output is a
// selection aid derived from tracker estimates, never reference truth.
package segments

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"slices"
	"sort"
)

const Version = 1

type Point struct {
	Track        string
	TimeNs       int64
	X, Y, VX, VY float64
	MaxSpeed     float64 // lifetime eligibility for the lateral finder
	SplitFlag    bool
}

type Capture struct {
	Path    string `json:"path"`
	FirstNs int64  `json:"first_ns"`
	LastNs  int64  `json:"last_ns"`
}

type Params struct {
	WindowSeconds     float64 `json:"window_seconds"`
	MinSpeed          float64 `json:"min_speed"`
	MaxHeadingDeg     float64 `json:"max_heading_deg"`
	MinGap            float64 `json:"min_gap"`
	MaxGap            float64 `json:"max_gap"`
	MaxLateral        float64 `json:"max_lateral"`
	JumpThreshold     float64 `json:"jump_threshold"`
	JumpMaxGapSeconds float64 `json:"jump_max_gap_seconds"`
	RandomSeed        int64   `json:"random_seed"`
}

func DefaultParams() Params { return Params{10, 3, 20, 3, 40, 2.5, 0.5, 0.3, 1} }

func (p Params) Validate() error {
	if !finite(p.WindowSeconds) || p.WindowSeconds < 0.1 || p.WindowSeconds > 3600 ||
		!finite(p.MinSpeed) || p.MinSpeed < 0 || !finite(p.MaxHeadingDeg) || p.MaxHeadingDeg < 0 || p.MaxHeadingDeg > 180 ||
		!finite(p.MinGap) || p.MinGap < 0 || !finite(p.MaxGap) || p.MaxGap < p.MinGap ||
		!finite(p.MaxLateral) || p.MaxLateral < 0 || !finite(p.JumpThreshold) || p.JumpThreshold < 0 ||
		!finite(p.JumpMaxGapSeconds) || p.JumpMaxGapSeconds <= 0 {
		return fmt.Errorf("invalid finder parameters")
	}
	return nil
}

func finite(f float64) bool { return !math.IsNaN(f) && !math.IsInf(f, 0) }

type Window struct {
	ID            string   `json:"id"`
	Finder        string   `json:"finder"`
	Version       int      `json:"version"`
	Source        string   `json:"source"`
	Role          string   `json:"role,omitempty"`
	StartNs       int64    `json:"window_start_unix_nanos"`
	EndNs         int64    `json:"window_end_unix_nanos"`
	PeakNs        int64    `json:"peak_timestamp_ns"`
	Score         float64  `json:"score"`
	PairFrames    int      `json:"pair_frames,omitempty"`
	PairSeconds   float64  `json:"pair_seconds,omitempty"`
	Pairs         int      `json:"pairs,omitempty"`
	Followers     int      `json:"followers,omitempty"`
	Leaders       int      `json:"leaders,omitempty"`
	LeaderChanges int      `json:"leader_changes,omitempty"`
	ClosestGapM   float64  `json:"closest_gap_m,omitempty"`
	FollowerIDs   []string `json:"follower_ids,omitempty"`
	LeaderIDs     []string `json:"leader_ids,omitempty"`
	TrackIDs      []string `json:"track_ids,omitempty"`
	Events        int      `json:"events,omitempty"`
	Capture       string   `json:"capture,omitempty"`
	OffsetSeconds float64  `json:"offset_seconds,omitempty"`
	Status        string   `json:"status"`
	ReplayCaseID  string   `json:"replay_case_id,omitempty"`
	JobID         string   `json:"job_id,omitempty"`
	PackDir       string   `json:"pack_dir,omitempty"`
}

// Identity binds a window to its source, role, finder and exact parameters.
func Identity(finder, source, role string, p Params, startNs int64) string {
	return identityAtVersion(Version, finder, source, role, p, startNs)
}

func identityAtVersion(version int, finder, source, role string, p Params, startNs int64) string {
	b, _ := json.Marshal(struct {
		Finder  string
		Version int
		Source  string
		Role    string
		Params  Params
		StartNs int64
	}{finder, version, source, role, p, startNs})
	sum := sha256.Sum256(b)
	return "seg-" + hex.EncodeToString(sum[:12])
}

type FinderInfo struct {
	Name    string `json:"name"`
	Version int    `json:"version"`
	HeldOut bool   `json:"held_out"`
}

func Finders() []FinderInfo {
	return []FinderInfo{{"following", Version, true}, {"leader_changes", Version, false}, {"lateral_jump", Version, false}, {"split_flags", Version, false}, {"exposure", Version, true}, {"random", Version, true}}
}

func Allowed(finder, role string) bool {
	if role != "tuning" && role != "held_out" {
		return false
	}
	if role == "held_out" && finder != "following" && finder != "exposure" && finder != "random" {
		return false
	}
	for _, f := range Finders() {
		if f.Name == finder {
			return true
		}
	}
	return false
}

type pair struct {
	follower, leader string
	gap              float64
}
type accumulator struct {
	w                          Window
	pairs                      map[string]bool
	followers, leaders, tracks map[string]bool
	nearest                    map[string][]pairFrame
	lastSeen                   map[string]int64
	maxEvent                   float64
	framePairCounts            map[int64]int
	peakPairCount              int
}
type pairFrame struct {
	time   int64
	leader string
	gap    float64
}

func newAccumulator(start, width int64) *accumulator {
	return &accumulator{w: Window{StartNs: start, EndNs: start + width, Status: "candidate"}, pairs: map[string]bool{}, followers: map[string]bool{}, leaders: map[string]bool{}, tracks: map[string]bool{}, nearest: map[string][]pairFrame{}, lastSeen: map[string]int64{}, framePairCounts: map[int64]int{}}
}

// Find computes a complete window ranking. Every result has a stable identity
// that binds its source, finder, parameters and time, including random seed.
func Find(points []Point, finder, source, role string, p Params, captures []Capture) ([]Window, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	if !Allowed(finder, role) {
		return nil, fmt.Errorf("finder %q is not allowed for role %q", finder, role)
	}
	if source == "" {
		return nil, fmt.Errorf("source is required")
	}
	if finder == "random" && role == "held_out" && len(captures) == 0 {
		return nil, fmt.Errorf("held-out random selection requires indexed captures")
	}
	width := int64(p.WindowSeconds * 1e9)
	points = slices.Clone(points)
	sort.Slice(points, func(i, j int) bool {
		if points[i].TimeNs != points[j].TimeNs {
			return points[i].TimeNs < points[j].TimeNs
		}
		return points[i].Track < points[j].Track
	})
	byTime := map[int64][]Point{}
	frameTimes := []int64{}
	for _, pt := range points {
		if pt.Track == "" || !finite(pt.X) || !finite(pt.Y) || !finite(pt.VX) || !finite(pt.VY) {
			continue
		}
		if _, ok := byTime[pt.TimeNs]; !ok {
			frameTimes = append(frameTimes, pt.TimeNs)
		}
		byTime[pt.TimeNs] = append(byTime[pt.TimeNs], pt)
	}
	period := 0.1
	if len(frameTimes) > 1 {
		gaps := make([]int64, 0, len(frameTimes)-1)
		for i := 1; i < len(frameTimes); i++ {
			gaps = append(gaps, frameTimes[i]-frameTimes[i-1])
		}
		slices.Sort(gaps)
		if len(gaps)%2 == 1 {
			period = float64(gaps[len(gaps)/2]) / 1e9
		} else {
			period = float64(gaps[len(gaps)/2-1]+gaps[len(gaps)/2]) / 2e9
		}
	}
	windows := map[int64]*accumulator{}
	get := func(t int64) *accumulator {
		start := floor(t, width) * width
		if windows[start] == nil {
			windows[start] = newAccumulator(start, width)
		}
		return windows[start]
	}
	if finder == "following" || finder == "leader_changes" {
		cosLimit := math.Cos(p.MaxHeadingDeg * math.Pi / 180)
		for _, t := range frameTimes {
			frame := byTime[t]
			for _, f := range frame {
				fs := math.Hypot(f.VX, f.VY)
				if fs < p.MinSpeed || fs == 0 {
					continue
				}
				fx, fy := f.VX/fs, f.VY/fs
				for _, l := range frame {
					if l.Track == f.Track {
						continue
					}
					ls := math.Hypot(l.VX, l.VY)
					if ls < p.MinSpeed || ls == 0 {
						continue
					}
					if fx*l.VX/ls+fy*l.VY/ls < cosLimit {
						continue
					}
					dx, dy := l.X-f.X, l.Y-f.Y
					along := dx*fx + dy*fy
					across := math.Abs(dx*fy - dy*fx)
					if along < p.MinGap || along > p.MaxGap || across > p.MaxLateral {
						continue
					}
					a := get(t)
					a.w.PairFrames++
					a.framePairCounts[t]++
					if a.framePairCounts[t] > a.peakPairCount {
						a.peakPairCount = a.framePairCounts[t]
						a.w.PeakNs = t
					}
					a.pairs[f.Track+"\x00"+l.Track] = true
					a.followers[f.Track] = true
					a.leaders[l.Track] = true
					a.tracks[f.Track] = true
					a.tracks[l.Track] = true
					if a.w.ClosestGapM == 0 || along < a.w.ClosestGapM {
						a.w.ClosestGapM = along
					}
					frames := a.nearest[f.Track]
					if len(frames) > 0 && frames[len(frames)-1].time == t {
						if along < frames[len(frames)-1].gap {
							frames[len(frames)-1] = pairFrame{t, l.Track, along}
							a.nearest[f.Track] = frames
						}
					} else {
						a.nearest[f.Track] = append(frames, pairFrame{t, l.Track, along})
					}
				}
			}
		}
		for _, a := range windows {
			for _, frames := range a.nearest {
				for i := 1; i < len(frames); i++ {
					if frames[i].leader != frames[i-1].leader {
						a.w.LeaderChanges++
						a.w.PeakNs = frames[i].time
					}
				}
			}
			a.w.PairSeconds = round2(float64(a.w.PairFrames) * period)
			a.w.Pairs = len(a.pairs)
			a.w.Followers = len(a.followers)
			a.w.Leaders = len(a.leaders)
			a.w.ClosestGapM = round2(a.w.ClosestGapM)
		}
	} else if finder == "lateral_jump" {
		byTrack := map[string][]Point{}
		for _, pt := range points {
			if pt.Track != "" && finite(pt.X) && finite(pt.Y) && (pt.MaxSpeed >= 6 || pt.MaxSpeed == 0) {
				byTrack[pt.Track] = append(byTrack[pt.Track], pt)
			}
		}
		for id, series := range byTrack {
			for i := 4; i < len(series); i++ {
				five := series[i-4 : i+1]
				contiguous := true
				for j := 1; j < 5; j++ {
					gap := float64(five[j].TimeNs-five[j-1].TimeNs) / 1e9
					if gap <= 0 || gap > p.JumpMaxGapSeconds {
						contiguous = false
						break
					}
				}
				if !contiguous {
					continue
				}
				residual, ok := lateralResidual(five)
				if !ok || residual <= p.JumpThreshold {
					continue
				}
				t := five[2].TimeNs
				a := get(t)
				a.w.Events++
				a.tracks[id] = true
				if residual > a.maxEvent {
					a.maxEvent = residual
					a.w.PeakNs = t
				}
				a.w.Score = math.Max(a.w.Score, residual)
			}
		}
	} else if finder == "random" && len(captures) > 0 {
		// Draw one uniformly seeded candidate per capture from all time
		// windows, including quiet road. A tracker failure or even a vehicle's
		// presence must not decide whether random held-out evidence can enter.
		for _, c := range captures {
			if c.LastNs <= c.FirstNs || (c.LastNs-c.FirstNs)/width > 100000 {
				return nil, fmt.Errorf("capture %q has invalid or excessive random windows", c.Path)
			}
			for start := c.FirstNs + 35_000_000_000; start+width <= c.LastNs; start += width {
				windows[start] = newAccumulator(start, width)
			}
		}
	} else if finder == "exposure" || finder == "random" || finder == "split_flags" {
		for _, t := range frameTimes {
			for _, pt := range byTime[t] {
				if finder == "split_flags" && !pt.SplitFlag {
					continue
				}
				if finder != "split_flags" && math.Hypot(pt.VX, pt.VY) < p.MinSpeed {
					continue
				}
				a := get(t)
				a.tracks[pt.Track] = true
				a.w.Events++
				a.w.PeakNs = t
			}
		}
	}
	result := make([]Window, 0, len(windows))
	for _, a := range windows {
		w := a.w
		w.Finder = finder
		w.Version = Version
		w.Source = source
		w.Role = role
		w.TrackIDs = keys(a.tracks)
		w.FollowerIDs = keys(a.followers)
		w.LeaderIDs = keys(a.leaders)
		switch finder {
		case "following":
			w.Score = float64(w.PairFrames)
		case "leader_changes":
			w.Score = float64(w.LeaderChanges)
		case "lateral_jump":
			w.Score = round2(w.Score)
		case "exposure", "split_flags":
			w.Score = float64(w.Events)
		case "random":
			rng := rand.New(rand.NewSource(p.RandomSeed ^ w.StartNs))
			w.Score = rng.Float64()
		}
		if (finder == "leader_changes" && w.LeaderChanges == 0) || (finder == "exposure" && w.Events == 0) || (finder == "random" && len(captures) == 0 && w.Events == 0) {
			continue
		}
		for _, c := range captures {
			if c.FirstNs <= w.StartNs && w.StartNs <= c.LastNs {
				w.Capture = c.Path
				w.OffsetSeconds = round1(float64(w.StartNs-c.FirstNs) / 1e9)
				break
			}
		}
		w.ID = Identity(finder, source, role, p, w.StartNs)
		result = append(result, w)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Score != result[j].Score {
			return result[i].Score > result[j].Score
		}
		return result[i].StartNs < result[j].StartNs
	})
	if finder == "random" && len(captures) > 0 {
		one := make([]Window, 0, len(captures))
		seen := map[string]bool{}
		for _, w := range result {
			if !seen[w.Capture] {
				one = append(one, w)
				seen[w.Capture] = true
			}
		}
		result = one
	}
	return result, nil
}

func floor(t, width int64) int64 {
	q := t / width
	if t%width < 0 {
		q--
	}
	return q
}
func round1(x float64) float64 { return math.Round(x*10) / 10 }
func round2(x float64) float64 { return math.Round(x*100) / 100 }
func keys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// lateralResidual is the script's five point time domain XY fit. The centre
// contributes to the fit; the result is an anomaly proxy, not a measured jump.
func lateralResidual(s []Point) (float64, bool) {
	centre := s[2].TimeNs
	var mt, mx, my float64
	for _, p := range s {
		mt += float64(p.TimeNs-centre) / 1e9
		mx += p.X
		my += p.Y
	}
	mt /= 5
	mx /= 5
	my /= 5
	var denom, nx, ny float64
	for _, p := range s {
		t := float64(p.TimeNs-centre)/1e9 - mt
		denom += t * t
		nx += t * (p.X - mx)
		ny += t * (p.Y - my)
	}
	if denom == 0 {
		return 0, false
	}
	vx, vy := nx/denom, ny/denom
	speed := math.Hypot(vx, vy)
	if speed < 2 {
		return 0, false
	}
	dx, dy := s[2].X-(mx-vx*mt), s[2].Y-(my-vy*mt)
	return math.Abs((-vy*dx + vx*dy) / speed), true
}
