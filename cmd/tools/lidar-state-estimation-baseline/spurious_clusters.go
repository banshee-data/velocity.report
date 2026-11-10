package main

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// spuriousClustersFile is the name of a case's spurious-cluster record in its
// first run's output directory, written under -spurious-clusters.
const spuriousClustersFile = "spurious_clusters.json"

// The windows l3bg0030's downstream clause compares, in seconds after the
// background settles: the first 30 s against the same site's 60 to 120 s.
const (
	spuriousEarlyFrom, spuriousEarlyTo   = 0.0, 30.0
	spuriousSteadyFrom, spuriousSteadyTo = 60.0, 120.0
	// spuriousQuietMomentRatio names a site whose early window runs this
	// many times its steady state or more: the criterion stopped on a quiet
	// moment, not a settled background.
	spuriousQuietMomentRatio = 1.5
)

// spuriousCollector records, for each tracker update, how many clusters the
// tracker was given and which tracks took them, and which tracks were ever
// confirmed. A spurious cluster is one no lasting track took: unassociated, or
// associated with a track that was never confirmed. A cluster that births a
// track is unassociated on its birth frame, so a lasting track's first
// cluster counts as spurious once. It reads the tap and changes nothing.
type spuriousCollector struct {
	frames    []spuriousTapFrame
	confirmed map[string]bool
}

type spuriousTapFrame struct {
	atNanos  int64
	clusters int
	takenBy  []string
}

func newSpuriousCollector() *spuriousCollector {
	return &spuriousCollector{confirmed: map[string]bool{}}
}

func (c *spuriousCollector) observe(f replayeval.TrackerTapFrame) {
	frame := spuriousTapFrame{atNanos: f.At.UnixNano(), clusters: len(f.Clusters)}
	for _, id := range f.Associations {
		if id != "" {
			frame.takenBy = append(frame.takenBy, id)
		}
	}
	c.frames = append(c.frames, frame)
	for _, track := range f.Tracks {
		if track != nil && track.TrackState == l5tracks.TrackConfirmed {
			c.confirmed[track.TrackID] = true
		}
	}
}

// spuriousFrame is one tracker update in the file: seconds since the first
// frame the background processed, clusters given, and those no lasting
// track took.
type spuriousFrame struct {
	ElapsedSeconds float64 `json:"t"`
	Clusters       int     `json:"clusters"`
	Spurious       int     `json:"spurious"`
}

// spuriousWindow sums a window of frames after the settle.
type spuriousWindow struct {
	FromSeconds      float64 `json:"from_s"`
	ToSeconds        float64 `json:"to_s"`
	Frames           int     `json:"frames"`
	Clusters         int     `json:"clusters"`
	Spurious         int     `json:"spurious"`
	SpuriousPerFrame float64 `json:"spurious_per_frame"`
	// Complete is false when the replay ended before the window did.
	Complete bool `json:"complete"`
}

type spuriousReport struct {
	Rule                 string          `json:"rule"`
	Settled              bool            `json:"settled"`
	SettledAfterSeconds  float64         `json:"settled_after_s"`
	SettledOnConvergence bool            `json:"settled_on_convergence"`
	LastingTracks        int             `json:"lasting_tracks"`
	Early                spuriousWindow  `json:"early"`
	Steady               spuriousWindow  `json:"steady"`
	EarlyOverSteady      *float64        `json:"early_over_steady"`
	QuietMoment          bool            `json:"quiet_moment"`
	Frames               []spuriousFrame `json:"frames"`
}

// report builds the record from the collected frames and the replay's
// settle. startNanos is the background's first frame, the clock the settle
// time is measured on.
func (c *spuriousCollector) report(startNanos int64, settled bool, settledAfter float64, onConvergence bool) spuriousReport {
	r := spuriousReport{
		Rule: "spurious: a cluster no lasting track took (unassociated, or taken by a track never confirmed); " +
			"windows in seconds after the settle, early 0-30 against steady 60-120; quiet moment when early/steady >= 1.5",
		Settled: settled, SettledAfterSeconds: settledAfter, SettledOnConvergence: onConvergence,
		LastingTracks: len(c.confirmed),
		Frames:        make([]spuriousFrame, 0, len(c.frames)),
	}
	for _, f := range c.frames {
		spurious := f.clusters
		for _, id := range f.takenBy {
			if c.confirmed[id] {
				spurious--
			}
		}
		r.Frames = append(r.Frames, spuriousFrame{
			ElapsedSeconds: float64(f.atNanos-startNanos) / 1e9, Clusters: f.clusters, Spurious: spurious,
		})
	}
	r.Early = spuriousWindowOf(r.Frames, settled, settledAfter, spuriousEarlyFrom, spuriousEarlyTo)
	r.Steady = spuriousWindowOf(r.Frames, settled, settledAfter, spuriousSteadyFrom, spuriousSteadyTo)
	if r.Early.Frames > 0 && r.Steady.Frames > 0 && r.Steady.SpuriousPerFrame > 0 {
		ratio := r.Early.SpuriousPerFrame / r.Steady.SpuriousPerFrame
		r.EarlyOverSteady = &ratio
		r.QuietMoment = ratio >= spuriousQuietMomentRatio
	}
	return r
}

// spuriousWindowOf sums the frames from settle+from (inclusive) to settle+to
// (exclusive). An unsettled replay has no windows.
func spuriousWindowOf(frames []spuriousFrame, settled bool, settledAfter, from, to float64) spuriousWindow {
	w := spuriousWindow{FromSeconds: from, ToSeconds: to}
	if !settled {
		return w
	}
	lo, hi := settledAfter+from, settledAfter+to
	for _, f := range frames {
		if f.ElapsedSeconds >= hi {
			w.Complete = true
		}
		if f.ElapsedSeconds < lo || f.ElapsedSeconds >= hi {
			continue
		}
		w.Frames++
		w.Clusters += f.Clusters
		w.Spurious += f.Spurious
	}
	if w.Frames > 0 {
		w.SpuriousPerFrame = float64(w.Spurious) / float64(w.Frames)
	}
	return w
}

func writeSpuriousClusters(dir string, r spuriousReport) error {
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, spuriousClustersFile), append(b, '\n'), 0o644)
}
