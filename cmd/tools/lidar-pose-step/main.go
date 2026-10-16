// Command lidar-pose-step asks whether a LiDAR's pose changed between two
// windows of a capture, from raw returns alone, before any background model.
//
// For each window it decodes every return with the pipeline's own Pandar40P
// parser and keeps, per background-grid cell (ring x 0.2-degree bin), the
// median range when the cell returns in most rotations with a small spread.
// For each pair of windows it fits a 6-DoF rigid motion between the two sets
// of static cells by robust point-to-plane least squares, and reports the
// motion, its uncertainty, the residual range change before and after, how
// the support spreads over azimuth sectors and rings, and whether it should
// abstain. A pose step is a global change the fit removes; a moved object or
// a local occlusion is a change confined to a few sectors that it does not.
//
// Windows are given in seconds relative to -anchor-ns, so a run around one
// event reads as offsets from it. Only the windows asked for are read: each
// capture is seeked to a window's start by packet time.
//
//	lidar-pose-step -pcap-root /Volumes/lidar/lidar/pcaps -case lombard-laguna \
//	  -anchor-ns 1788373950323299330 \
//	  -pair event=-11,-1,1,7 -pair null/q-600=-611,-601,-599,-593 \
//	  -json pose.json > pose.md
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l1packets/parse"
	"github.com/banshee-data/velocity.report/internal/version"
)

type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, " ") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

// pairSpec is NAME=A0,A1,B0,B1 in anchor-relative seconds; the group is the
// name's prefix up to its first slash.
type pairSpec struct {
	Name   string
	Group  string
	A0, A1 float64
	B0, B1 float64
}

func parsePairSpec(v string) (pairSpec, error) {
	name, rest, ok := strings.Cut(v, "=")
	if !ok || strings.TrimSpace(name) == "" {
		return pairSpec{}, fmt.Errorf("%q: want NAME=A0,A1,B0,B1", v)
	}
	parts := strings.Split(rest, ",")
	if len(parts) != 4 {
		return pairSpec{}, fmt.Errorf("%q: want four comma-separated seconds", v)
	}
	var x [4]float64
	for i, s := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return pairSpec{}, fmt.Errorf("%q: %w", v, err)
		}
		x[i] = f
	}
	if x[1] <= x[0] || x[3] <= x[2] {
		return pairSpec{}, fmt.Errorf("%q: each window must end after it starts", v)
	}
	group, _, _ := strings.Cut(name, "/")
	return pairSpec{Name: name, Group: group, A0: x[0], A1: x[1], B0: x[2], B1: x[3]}, nil
}

type indexEntry struct {
	ID       string   `json:"id"`
	Captures []string `json:"captures"`
}

type captureInfo struct {
	Path       string `json:"path"`
	SizeBytes  int64  `json:"size_bytes"`
	FirstNanos int64  `json:"first_unix_nanos"`
	LastNanos  int64  `json:"last_unix_nanos"`
	// FirstS and LastS are anchor-relative seconds.
	FirstS float64 `json:"first_s"`
	LastS  float64 `json:"last_s"`
	file   *ngFile
}

// anchorMapping places the anchor in the capture set.
type anchorMapping struct {
	UnixNanos int64  `json:"unix_nanos"`
	UTC       string `json:"utc"`
	// Capture is the file holding the anchor, or "" when it falls in a gap.
	Capture         string  `json:"capture"`
	OffsetInFileS   float64 `json:"offset_in_capture_s"`
	SinceFirstS     float64 `json:"since_first_packet_s"`
	SinceFirstClock string  `json:"since_first_packet_clock"`
	UntilFileEndS   float64 `json:"until_capture_end_s"`
	NextCaptureInS  float64 `json:"next_capture_starts_in_s,omitempty"`
}

type windowInfo struct {
	Key        string         `json:"key"`
	StartS     float64        `json:"start_s"`
	EndS       float64        `json:"end_s"`
	StartNanos int64          `json:"start_unix_nanos"`
	EndNanos   int64          `json:"end_unix_nanos"`
	Captures   []string       `json:"captures"`
	Packets    int            `json:"packets"`
	Returns    int            `json:"returns"`
	Rotations  int            `json:"rotations"`
	CoveredS   float64        `json:"covered_s"`
	Truncated  bool           `json:"truncated"`
	ReturnMode map[string]int `json:"return_modes"`
	MotorRPM   float64        `json:"motor_rpm_median"`
	SeenCells  int            `json:"seen_cells"`
	Static     int            `json:"static_cells"`
	cells      []cellStat
}

type seriesRow struct {
	StartS       float64 `json:"start_s"`
	Returns      int     `json:"returns"`
	ChangedShare float64 `json:"changed_share"`
	MedianAbsM   float64 `json:"median_abs_m"`
	// SectorChanged is the changed share in each 30-degree sector, in scan
	// order from azimuth 0.
	SectorChanged []float64 `json:"sector_changed_share"`
}

type seriesResult struct {
	Name      string      `json:"name"`
	Reference windowRef   `json:"reference"`
	Span      windowRef   `json:"span"`
	TauM      float64     `json:"tau_m"`
	Rows      []seriesRow `json:"rotations"`
}

type output struct {
	Tool         string         `json:"tool"`
	BuildVersion string         `json:"build_version"`
	BuildGitSHA  string         `json:"build_git_sha"`
	BuildTime    string         `json:"build_time"`
	Case         string         `json:"case,omitempty"`
	UDPPort      int            `json:"udp_port"`
	Static       staticParams   `json:"static_params"`
	Fit          fitParams      `json:"fit_params"`
	Captures     []captureInfo  `json:"captures"`
	Anchor       anchorMapping  `json:"anchor"`
	Windows      []*windowInfo  `json:"windows"`
	Pairs        []pairResult   `json:"pairs"`
	Series       []seriesResult `json:"series,omitempty"`
	Groups       []groupSummary `json:"groups"`
}

func main() {
	var (
		indexPath  = flag.String("index", "tools/s2-archive/site-index.json", "capture archive index JSON")
		caseID     = flag.String("case", "", "index entry whose captures to read, in order; or give capture paths as arguments")
		pcapRoot   = flag.String("pcap-root", "", "directory holding the archive's capture subdirectories (with -case)")
		pcapSubdir = flag.String("pcap-subdir", "s2", "archive capture subdirectory")
		udpPort    = flag.Int("udp-port", 2369, "LiDAR data UDP destination port")
		anchorNs   = flag.Int64("anchor-ns", 0, "anchor capture time, unix nanoseconds; window seconds are relative to it (required)")
		jsonPath   = flag.String("json", "", "write the full result as JSON here")
		seriesTau  = flag.Float64("series-tau", 0.15, "|range - reference median| above which a return counts as changed in -series")
		pairs      multiFlag
		series     multiFlag
	)
	sp := staticParams{}
	flag.Float64Var(&sp.MinCoverage, "min-coverage", 0.8, "share of a window's rotations a static cell must return in")
	flag.Float64Var(&sp.MaxIQR, "max-iqr", 0.08, "static cell range IQR bound, metres (or -max-iqr-fraction of range, if larger)")
	flag.Float64Var(&sp.MaxIQRFraction, "max-iqr-fraction", 0.004, "static cell range IQR bound as a fraction of range")
	flag.Float64Var(&sp.MinRange, "min-range", 2.5, "exclude cells nearer than this, metres (the mount moves with the sensor)")
	flag.Float64Var(&sp.MaxRange, "max-range", 120, "exclude cells farther than this, metres")
	flag.Var(&pairs, "pair", "NAME=A0,A1,B0,B1: compare before window [A0,A1] with after window [B0,B1], anchor-relative seconds; a NAME prefix up to '/' groups pairs (repeatable)")
	flag.Var(&series, "series", "NAME=R0,R1,S0,S1: per rotation in [S0,S1], the raw departure of returns from window [R0,R1]'s static medians (repeatable)")
	flag.Parse()

	if *anchorNs == 0 {
		fatal(fmt.Errorf("-anchor-ns is required"))
	}
	if len(pairs) == 0 && len(series) == 0 {
		fatal(fmt.Errorf("give at least one -pair or -series"))
	}
	var specs, seriesSpecs []pairSpec
	for _, v := range pairs {
		s, err := parsePairSpec(v)
		if err != nil {
			fatal(fmt.Errorf("-pair %w", err))
		}
		specs = append(specs, s)
	}
	for _, v := range series {
		s, err := parsePairSpec(v)
		if err != nil {
			fatal(fmt.Errorf("-series %w", err))
		}
		seriesSpecs = append(seriesSpecs, s)
	}
	paths, err := resolveCaptures(*caseID, *indexPath, *pcapRoot, *pcapSubdir, flag.Args())
	if err != nil {
		fatal(err)
	}
	cfg, err := parse.LoadEmbeddedPandar40PConfig()
	if err != nil {
		fatal(err)
	}
	geom, err := newGeometry(parse.ElevationsFromConfig(cfg))
	if err != nil {
		fatal(err)
	}

	out := output{
		Tool: "lidar-pose-step", BuildVersion: version.Version, BuildGitSHA: version.GitSHA, BuildTime: version.BuildTime,
		Case: *caseID, UDPPort: *udpPort, Static: sp, Fit: defaultFitParams(),
	}
	for _, p := range paths {
		f, err := openPcapng(p)
		if err != nil {
			fatal(err)
		}
		defer f.Close()
		first, last, err := f.extent()
		if err != nil {
			fatal(err)
		}
		out.Captures = append(out.Captures, captureInfo{Path: p, SizeBytes: f.size, FirstNanos: first, LastNanos: last,
			FirstS: rel(first, *anchorNs), LastS: rel(last, *anchorNs), file: f})
	}
	out.Anchor = mapAnchor(out.Captures, *anchorNs)
	fmt.Fprintf(os.Stderr, "anchor %s: %s +%.3f s (placement %s)\n", out.Anchor.UTC,
		filepath.Base(out.Anchor.Capture), out.Anchor.OffsetInFileS, out.Anchor.SinceFirstClock)

	windows := map[string]*windowInfo{}
	window := func(s0, s1 float64) (*windowInfo, error) {
		key := windowKey(s0, s1)
		if w, ok := windows[key]; ok {
			return w, nil
		}
		start, end := abs(*anchorNs, s0), abs(*anchorNs, s1)
		began := time.Now()
		w, err := decodeWindow(out.Captures, start, end, uint16(*udpPort), cfg, sp)
		if err != nil {
			return nil, fmt.Errorf("window %s: %w", key, err)
		}
		w.Key, w.StartS, w.EndS = key, s0, s1
		fmt.Fprintf(os.Stderr, "window %s: %d packets, %d rotations, %d static cells (%.1f s)\n",
			key, w.Packets, w.Rotations, w.Static, time.Since(began).Seconds())
		windows[key] = w
		out.Windows = append(out.Windows, w)
		return w, nil
	}

	fp := defaultFitParams()
	for _, s := range specs {
		a, err := window(s.A0, s.A1)
		if err != nil {
			fatal(err)
		}
		b, err := window(s.B0, s.B1)
		if err != nil {
			fatal(err)
		}
		r := comparePair(geom, a.cells, b.cells, fp)
		r.Name, r.Group = s.Name, s.Group
		r.A = windowRef{StartS: s.A0, EndS: s.A1, StartNanos: a.StartNanos, EndNanos: a.EndNanos}
		r.B = windowRef{StartS: s.B0, EndS: s.B1, StartNanos: b.StartNanos, EndNanos: b.EndNanos}
		if a.Truncated || b.Truncated {
			r.Reasons = append(r.Reasons, "a window is truncated by a capture gap (reported, not abstained)")
		}
		out.Pairs = append(out.Pairs, r)
	}
	for _, s := range seriesSpecs {
		ref, err := window(s.A0, s.A1)
		if err != nil {
			fatal(err)
		}
		start, end := abs(*anchorNs, s.B0), abs(*anchorNs, s.B1)
		acc, err := decodeSeries(out.Captures, start, end, uint16(*udpPort), cfg, ref.cells, *seriesTau)
		if err != nil {
			fatal(fmt.Errorf("series %s: %w", s.Name, err))
		}
		sr := seriesResult{Name: s.Name, TauM: *seriesTau,
			Reference: windowRef{StartS: s.A0, EndS: s.A1, StartNanos: ref.StartNanos, EndNanos: ref.EndNanos},
			Span:      windowRef{StartS: s.B0, EndS: s.B1, StartNanos: start, EndNanos: end}}
		for _, row := range acc {
			r := seriesRow{StartS: rel(row.StartNanos, *anchorNs), Returns: row.Returns,
				ChangedShare: share(row.Changed, row.Returns), MedianAbsM: row.MedianAbsM}
			for k := range row.SectorReturns {
				r.SectorChanged = append(r.SectorChanged, share(row.SectorChanged[k], row.SectorReturns[k]))
			}
			sr.Rows = append(sr.Rows, r)
		}
		out.Series = append(out.Series, sr)
	}
	out.Groups = summariseGroups(out.Pairs)

	if *jsonPath != "" {
		b, err := json.MarshalIndent(out, "", "  ")
		if err != nil {
			fatal(err)
		}
		if err := os.WriteFile(*jsonPath, append(b, '\n'), 0o644); err != nil {
			fatal(err)
		}
	}
	writeMarkdown(os.Stdout, out)
}

func resolveCaptures(caseID, indexPath, root, subdir string, args []string) ([]string, error) {
	if caseID == "" {
		if len(args) == 0 {
			return nil, fmt.Errorf("give -case or capture paths")
		}
		return args, nil
	}
	if len(args) > 0 {
		return nil, fmt.Errorf("-case and capture paths are mutually exclusive")
	}
	if root == "" {
		return nil, fmt.Errorf("-case needs -pcap-root")
	}
	b, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, fmt.Errorf("read index: %w", err)
	}
	var entries []indexEntry
	if err := json.Unmarshal(b, &entries); err != nil {
		return nil, fmt.Errorf("decode index: %w", err)
	}
	for _, e := range entries {
		if e.ID != caseID {
			continue
		}
		if len(e.Captures) == 0 {
			return nil, fmt.Errorf("index entry %s lists no captures", caseID)
		}
		var paths []string
		for _, c := range e.Captures {
			paths = append(paths, filepath.Join(root, subdir, c))
		}
		return paths, nil
	}
	return nil, fmt.Errorf("case %q is not in %s", caseID, indexPath)
}

// decodeWindow reads every capture overlapping [start, end] from its first
// packet at or after start, and summarises the window's cells.
func decodeWindow(caps []captureInfo, start, end int64, port uint16, cfg *parse.Pandar40PConfig,
	sp staticParams) (*windowInfo, error) {
	acc := newWindowAccum()
	w := &windowInfo{StartNanos: start, EndNanos: end, ReturnMode: map[string]int{}}
	if err := readSpan(caps, start, end, port, cfg, acc, &w.Captures); err != nil {
		return nil, err
	}
	w.Packets, w.Returns, w.Rotations = acc.packets, acc.points, acc.rotations()
	if acc.packets > 0 {
		w.CoveredS = float64(acc.lastNanos-acc.firstNanos) / 1e9
	}
	w.Truncated = w.CoveredS < 0.9*float64(end-start)/1e9
	for m, n := range acc.returnModes {
		w.ReturnMode[fmt.Sprintf("0x%02x", m)] = n
	}
	if len(acc.motorRPM) > 0 {
		w.MotorRPM = median(acc.motorRPM)
	}
	if acc.packets == 0 {
		return nil, fmt.Errorf("no packets on port %d in the window", port)
	}
	w.cells = acc.finish(sp)
	w.SeenCells, w.Static = countStatic(w.cells)
	return w, nil
}

// decodeSeries reads [start, end] rotation by rotation against ref's static
// medians. It holds one rotation's returns at a time, so a long span costs a
// fixed-size row per rotation, not memory per return.
func decodeSeries(caps []captureInfo, start, end int64, port uint16, cfg *parse.Pandar40PConfig,
	ref []cellStat, tau float64) ([]rotationSample, error) {
	acc := newWindowAccum()
	acc.seriesRef, acc.seriesTau = ref, tau
	var used []string
	if err := readSpan(caps, start, end, port, cfg, acc, &used); err != nil {
		return nil, err
	}
	acc.closeRotation()
	return acc.series, nil
}

func readSpan(caps []captureInfo, start, end int64, port uint16, cfg *parse.Pandar40PConfig, acc *windowAccum, used *[]string) error {
	parser := parse.NewPandar40PParser(*cfg)
	for _, c := range caps {
		if c.LastNanos < start || c.FirstNanos > end {
			continue
		}
		*used = append(*used, filepath.Base(c.Path))
		off, err := c.file.seekOffset(start)
		if err != nil {
			return err
		}
		var decodeErr error
		decodeFailures := 0
		err = c.file.readFrom(off, func(p ngPacket) bool {
			if p.UnixNanos < start {
				return true
			}
			if p.UnixNanos > end {
				return false
			}
			payload := udpPayload(c.file.ifaces[p.Iface].linkType, p.Data, port)
			if payload == nil {
				return true
			}
			if err := acc.addPacket(parser, p.UnixNanos, payload); err != nil {
				decodeFailures++
				decodeErr = err
			}
			return true
		})
		if err != nil {
			return err
		}
		if decodeFailures > 0 {
			fmt.Fprintf(os.Stderr, "%s: %d packets on port %d failed to decode (last: %v)\n",
				filepath.Base(c.Path), decodeFailures, port, decodeErr)
		}
	}
	return nil
}

func mapAnchor(caps []captureInfo, anchor int64) anchorMapping {
	m := anchorMapping{UnixNanos: anchor, UTC: timeFromNanos(anchor).UTC().Format("2006-01-02T15:04:05.000000000Z")}
	if len(caps) == 0 {
		return m
	}
	m.SinceFirstS = rel(anchor, caps[0].FirstNanos)
	m.SinceFirstClock = clock(m.SinceFirstS)
	for i, c := range caps {
		if anchor >= c.FirstNanos && anchor <= c.LastNanos {
			m.Capture = c.Path
			m.OffsetInFileS = rel(anchor, c.FirstNanos)
			m.UntilFileEndS = rel(c.LastNanos, anchor)
			if i+1 < len(caps) {
				m.NextCaptureInS = rel(caps[i+1].FirstNanos, anchor)
			}
		}
	}
	return m
}

func clock(s float64) string {
	sign := ""
	if s < 0 {
		sign, s = "-", -s
	}
	m := math.Floor(s / 60)
	return fmt.Sprintf("%s%d:%06.3f", sign, int(m), s-60*m)
}

func windowKey(s0, s1 float64) string { return fmt.Sprintf("[%+.1f,%+.1f]", s0, s1) }

func abs(anchor int64, s float64) int64 { return anchor + int64(math.Round(s*1e9)) }

func rel(t, anchor int64) float64 { return float64(t-anchor) / 1e9 }

func timeFromNanos(n int64) time.Time { return time.Unix(0, n) }

func share(n, d int) float64 {
	if d == 0 {
		return math.NaN()
	}
	return float64(n) / float64(d)
}

// groupSummary is the spread of one group of pairs, the null distribution
// when the group holds quiet pairs.
type groupSummary struct {
	Group string `json:"group"`
	Pairs int    `json:"pairs"`
	// Abstained pairs are counted but left out of the spreads.
	Abstained       int     `json:"abstained"`
	RotationMagDeg  spread3 `json:"rotation_magnitude_deg"`
	TiltDeg         spread3 `json:"tilt_deg"`
	YawDeg          spread3 `json:"abs_yaw_deg"`
	TranslationMagM spread3 `json:"translation_magnitude_m"`
	BeforeMedianM   spread3 `json:"before_median_abs_m"`
	AfterMedianM    spread3 `json:"after_median_abs_m"`
	// Reduction is 1 - after/before of the median |delta range|.
	Reduction    spread3 `json:"median_reduction"`
	BeforeChange spread3 `json:"before_changed_share"`
	AfterChange  spread3 `json:"after_changed_share"`
}

type spread3 struct {
	Min    float64 `json:"min"`
	Median float64 `json:"median"`
	Max    float64 `json:"max"`
}

func spreadOf(xs []float64) spread3 {
	if len(xs) == 0 {
		return spread3{math.NaN(), math.NaN(), math.NaN()}
	}
	s := append([]float64(nil), xs...)
	slices.Sort(s)
	return spread3{s[0], sortedQuantile(s, 0.5), s[len(s)-1]}
}

func summariseGroups(pairs []pairResult) []groupSummary {
	var order []string
	byGroup := map[string][]pairResult{}
	for _, p := range pairs {
		if _, ok := byGroup[p.Group]; !ok {
			order = append(order, p.Group)
		}
		byGroup[p.Group] = append(byGroup[p.Group], p)
	}
	var out []groupSummary
	for _, g := range order {
		gs := groupSummary{Group: g, Pairs: len(byGroup[g])}
		var rot, tilt, yaw, tr, bm, am, red, bc, ac []float64
		for _, p := range byGroup[g] {
			if p.Abstain {
				gs.Abstained++
				continue
			}
			rot = append(rot, p.Transform.RotationMagDeg)
			tilt = append(tilt, p.Transform.TiltDeg)
			yaw = append(yaw, math.Abs(p.Transform.RotationDeg[2]))
			tr = append(tr, p.Transform.TranslationMagM)
			bm = append(bm, p.Before.MedianAbs)
			am = append(am, p.After.MedianAbs)
			if p.Before.MedianAbs > 0 {
				red = append(red, 1-p.After.MedianAbs/p.Before.MedianAbs)
			}
			bc = append(bc, p.Before.ChangedShare)
			ac = append(ac, p.After.ChangedShare)
		}
		gs.RotationMagDeg, gs.TiltDeg, gs.YawDeg, gs.TranslationMagM = spreadOf(rot), spreadOf(tilt), spreadOf(yaw), spreadOf(tr)
		gs.BeforeMedianM, gs.AfterMedianM, gs.Reduction = spreadOf(bm), spreadOf(am), spreadOf(red)
		gs.BeforeChange, gs.AfterChange = spreadOf(bc), spreadOf(ac)
		out = append(out, gs)
	}
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "lidar-pose-step:", err)
	os.Exit(1)
}
