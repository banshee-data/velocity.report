// lidar-fg-diag dumps foreground classification, frame by frame, from a VRLOG
// or from a fresh replay of a capture, so what a recording shows can be checked
// against the rule that decided each return.
//
//	lidar-fg-diag vrlog  -vrlog DIR -from N -to N -out DIR
//	lidar-fg-diag replay -pcap FILE -tuning FILE [-experiment a,b] -from SEC -to SEC [-every N] -out DIR
//	lidar-fg-diag oracle -pcap FILE -replay-out DIR -out FILE
//	lidar-fg-diag score  -pcap FILE -oracle FILE -tuning FILE [-experiment a,b] -replay-out DIR -json FILE
//
// oracle finds each cell's persistent surfaces over a whole capture with no L3
// state; score replays an arm and measures its masks against them, with its
// clusters and tracks: track lives, medoid jitter, broken links.
//
// Each frame is a little-endian packed record file plus a JSON sidecar. The
// record layout is in pointRecord below; tools/lidar-fg-diag/render_frames.py
// draws them.
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l9endpoints/recorder"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// pointRecord is one return. Fields a VRLOG cannot supply are zero.
type pointRecord struct {
	X, Y, Z, Range, Azimuth          float32
	Average, Spread, Window, BaseWin float32
	Locked                           float32
	Seen                             uint32
	Ring, Intensity, Published, L3   uint8
	Code                             uint8
	Label                            uint8
	_                                [2]uint8
}

type trackMeta struct {
	ID     string  `json:"id"`
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	VX     float32 `json:"vx"`
	VY     float32 `json:"vy"`
	Length float32 `json:"length"`
	Width  float32 `json:"width"`
	Height float32 `json:"height"`
	Yaw    float32 `json:"yaw"`
	Hits   int     `json:"hits"`
	Misses int     `json:"misses"`
}

type clusterMeta struct {
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Length float32 `json:"length"`
	Width  float32 `json:"width"`
	Points int     `json:"points"`
	Track  string  `json:"track"`
}

type frameMeta struct {
	Clusters []clusterMeta `json:"clusters,omitempty"`
	Index    int           `json:"index"`
	Seconds  float64       `json:"seconds"`
	UnixNs   int64         `json:"unix_ns"`
	Points   int           `json:"points"`
	Fg       int           `json:"fg"`
	Tracks   []trackMeta   `json:"tracks"`
	Type     string        `json:"type,omitempty"`
}

func writeFrame(dir string, meta frameMeta, records []pointRecord) error {
	base := filepath.Join(dir, fmt.Sprintf("frame_%06d", meta.Index))
	f, err := os.Create(base + ".bin")
	if err != nil {
		return err
	}
	w := bufio.NewWriterSize(f, 1<<20)
	if err := binary.Write(w, binary.LittleEndian, records); err != nil {
		f.Close()
		return err
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	b, err := json.Marshal(meta)
	if err != nil {
		return err
	}
	return os.WriteFile(base+".json", b, 0o644)
}

func runVRLOG(args []string) error {
	fs := flag.NewFlagSet("vrlog", flag.ExitOnError)
	path := fs.String("vrlog", "", "VRLOG directory")
	from := fs.Int("from", 0, "first rotation frame")
	to := fs.Int("to", 0, "last rotation frame")
	out := fs.String("out", "", "output directory")
	fs.Parse(args)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	r, err := recorder.NewReplayer(*path)
	if err != nil {
		return err
	}
	defer r.Close()
	start := r.Header().StartNs
	rotation := -1
	for {
		b, err := r.ReadFrame()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if b.PointCloud == nil || b.Background != nil {
			continue
		}
		rotation++
		if rotation < *from {
			continue
		}
		if rotation > *to {
			return nil
		}
		pc := b.PointCloud
		n := len(pc.X)
		records := make([]pointRecord, n)
		fg := 0
		for i := 0; i < n; i++ {
			records[i] = pointRecord{X: pc.X[i], Y: pc.Y[i], Z: pc.Z[i], Published: pc.Classification[i],
				L3: pc.Classification[i], Intensity: pc.Intensity[i]}
			if pc.Classification[i] != 0 {
				fg++
			}
		}
		meta := frameMeta{Index: rotation, Seconds: float64(b.TimestampNanos-start) / 1e9, UnixNs: b.TimestampNanos,
			Points: n, Fg: fg, Type: fmt.Sprint(b.FrameType)}
		if b.Tracks != nil {
			for _, t := range b.Tracks.Tracks {
				meta.Tracks = append(meta.Tracks, trackMeta{ID: t.TrackID, X: t.X, Y: t.Y, VX: t.VX, VY: t.VY, Hits: t.Hits, Misses: t.Misses,
					Length: t.BBoxLength, Width: t.BBoxWidth, Height: t.BBoxHeight, Yaw: t.HeadingRad})
			}
		}
		if err := writeFrame(*out, meta, records); err != nil {
			return err
		}
	}
}

func runReplay(args []string) error {
	fs := flag.NewFlagSet("replay", flag.ExitOnError)
	pcap := fs.String("pcap", "", "capture")
	tuning := fs.String("tuning", "config/tuning.defaults.json", "tuning file")
	experiment := fs.String("experiment", "", "comma-separated replay experiments")
	from := fs.Float64("from", 0, "first capture second to dump")
	to := fs.Float64("to", 0, "last capture second to dump")
	every := fs.Int("every", 1, "dump every Nth frame in the window")
	start := fs.Float64("start", 0, "start replaying at this capture second")
	warmup := fs.Float64("warmup", 0, "warm-up seconds before start")
	duration := fs.Float64("duration", 0, "replay this many capture seconds; zero is all of it")
	replayOut := fs.String("replay-out", "", "new replay output directory")
	out := fs.String("out", "", "output directory")
	summary := fs.String("summary", "", "per-frame decision count CSV")
	oraclePath := fs.String("oracle", "", "oracle file, to label dumped returns")
	fs.Parse(args)
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return err
	}
	var oracle []oracleCell
	if *oraclePath != "" {
		var err error
		if oracle, err = loadOracle(*oraclePath); err != nil {
			return err
		}
	}
	var csv *bufio.Writer
	if *summary != "" {
		f, err := os.Create(*summary)
		if err != nil {
			return err
		}
		defer f.Close()
		csv = bufio.NewWriter(f)
		defer csv.Flush()
		fmt.Fprintln(csv, "frame,seconds,points,published,l3,code0,code1,code2,code3,code4,code5,code6,code7,code8,code9,code10,code11,code12")
	}
	var first time.Time
	index := -1
	var tapErr error
	var lastClusters []clusterMeta
	cfg := replayeval.Config{PCAPFile: *pcap, OutDir: *replayOut, TuningFile: *tuning, SensorID: "hesai-pandar40p",
		UDPPort: 2369, StartSeconds: *start, WarmupSeconds: *warmup, DurationSeconds: *duration, FrameTapDecisions: true, FrameTap: func(f replayeval.FrameTapFrame) {
			if tapErr != nil {
				return
			}
			if first.IsZero() {
				first = f.At
			}
			index++
			sec := f.At.Sub(first).Seconds()
			if len(f.Decisions) != len(f.Frame.Points) {
				tapErr = fmt.Errorf("frame %d: %d decisions for %d points", index, len(f.Decisions), len(f.Frame.Points))
				return
			}
			if csv != nil {
				var counts [13]int
				published, l3 := 0, 0
				for i := range f.Decisions {
					counts[f.Decisions[i].Code]++
					if f.Mask[i] {
						published++
					}
					if f.L3Mask[i] {
						l3++
					}
				}
				fmt.Fprintf(csv, "%d,%.3f,%d,%d,%d", index, sec, len(f.Decisions), published, l3)
				for _, c := range counts {
					fmt.Fprintf(csv, ",%d", c)
				}
				fmt.Fprintln(csv)
			}
			if sec < *from || sec > *to || index%*every != 0 {
				return
			}
			records := make([]pointRecord, len(f.Frame.Points))
			fg := 0
			for i, p := range f.Frame.Points {
				d := f.Decisions[i]
				r := pointRecord{X: float32(p.X), Y: float32(p.Y), Z: float32(p.Z), Range: float32(p.Distance),
					Azimuth: float32(p.Azimuth), Average: d.Average, Spread: d.Spread, Window: d.Window, BaseWin: d.BaseWindow,
					Locked: d.Locked, Seen: d.Seen, Ring: uint8(p.Channel), Intensity: p.Intensity, Code: uint8(d.Code)}
				if oracle != nil {
					if c := cellOf(&f.Frame.Points[i]); c >= 0 {
						r.Label = oracleLabel(&oracle[c], r.Range)
					}
				}
				if d.Held {
					r.Code |= 0x80
				}
				if f.Mask[i] {
					r.Published = 1
					fg++
				}
				if f.L3Mask[i] {
					r.L3 = 1
				}
				records[i] = r
			}
			meta := frameMeta{Index: index, Seconds: sec, UnixNs: f.At.UnixNano(), Points: len(records), Fg: fg, Clusters: lastClusters}
			for _, t := range f.Tracks {
				meta.Tracks = append(meta.Tracks, trackMeta{ID: t.TrackID, X: t.X, Y: t.Y, VX: t.VX, VY: t.VY, Hits: t.Hits, Misses: t.Misses,
					Length: t.BoundingBoxLengthAvg, Width: t.BoundingBoxWidthAvg, Height: t.BoundingBoxHeightAvg,
					Yaw: float32(math.Atan2(float64(t.VY), float64(t.VX)))})
			}
			tapErr = writeFrame(*out, meta, records)
		}}
	if *experiment != "" {
		cfg.Experiments = strings.Split(*experiment, ",")
	}
	cfg.TrackerTap = func(f replayeval.TrackerTapFrame) {
		lastClusters = lastClusters[:0:0]
		for i, c := range f.Clusters {
			m := clusterMeta{X: c.CentroidX, Y: c.CentroidY, Length: c.BoundingBoxLength, Width: c.BoundingBoxWidth, Points: c.PointsCount}
			if i < len(f.Associations) {
				m.Track = f.Associations[i]
			}
			lastClusters = append(lastClusters, m)
		}
	}
	if _, err := replayeval.Run(cfg); err != nil {
		return err
	}
	return tapErr
}

func main() {
	var err error
	switch {
	case len(os.Args) > 1 && os.Args[1] == "vrlog":
		err = runVRLOG(os.Args[2:])
	case len(os.Args) > 1 && os.Args[1] == "replay":
		err = runReplay(os.Args[2:])
	case len(os.Args) > 1 && os.Args[1] == "oracle":
		err = runOracle(os.Args[2:])
	case len(os.Args) > 1 && os.Args[1] == "score":
		err = runScoreCmd(os.Args[2:])
	default:
		err = fmt.Errorf("usage: lidar-fg-diag vrlog|replay|oracle|score ...")
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
