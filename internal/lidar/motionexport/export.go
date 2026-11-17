package motionexport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/l2frames"
)

// ManifestSchema names the provisional run manifest. Gerardus plans run.json
// as a proto message; this JSON carries the same facts until that lands.
const ManifestSchema = "velocity.report/motion-segment/v0"

// Time sources.
const (
	TimeAzimuthClock = "azimuth_clock_v1"
	TimePacket       = "packet_firetime"
)

// Config sets one export.
type Config struct {
	OutDir string
	// Epoch is time zero for every point time. Zero means the first exported
	// frame's start, truncated to the second.
	Epoch time.Time
	// From and To bound the exported frames by start time; zero is open.
	From, To time.Time
	// TimeSource is TimeAzimuthClock (default) or TimePacket.
	TimeSource string
	// SplitSeconds starts a new file after this much capture time. Default 60.
	SplitSeconds float64
	// AzimuthStride keeps every n-th firing block of a rotation (1 keeps all).
	AzimuthStride int
	// MaxRangeM drops returns beyond it; zero keeps every range.
	MaxRangeM float64
	// KeepEveryRotation exports one rotation in n (1 keeps all). It is the
	// temporal-only rate ablation: a 10 Hz drive at 2 reads as 5 Hz with 10 Hz
	// angular density, and a 20 Hz drive at 2 as 10 Hz with 20 Hz density. It
	// does not reproduce a sensor running at the lower rate.
	KeepEveryRotation int
	// IncludeIncomplete exports rotations the frame builder marked incomplete.
	IncludeIncomplete bool
	// MaxFlaggedPeriodDeviation flags a rotation whose period differs from
	// the segment median by more than this fraction. Default 0.02.
	MaxFlaggedPeriodDeviation float64
	Software                  string
}

// Manifest describes an export. Fields set by the caller (captures, tuning,
// sites, mount, build) are recorded verbatim.
type Manifest struct {
	Schema      string            `json:"schema"`
	Note        string            `json:"note"`
	Build       map[string]string `json:"build"`
	Sensor      map[string]string `json:"sensor"`
	Captures    []FileDigest      `json:"captures"`
	Tuning      *FileDigest       `json:"tuning,omitempty"`
	Segment     Segment           `json:"segment"`
	Mount       Mount             `json:"mount"`
	Time        TimeInfo          `json:"time"`
	Coordinates map[string]string `json:"coordinates"`
	Selection   Selection         `json:"selection"`
	Files       []PointFile       `json:"files"`
	Frames      FrameStats        `json:"frames"`
}

// FileDigest names an input or output file by base name and content.
type FileDigest struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

// Segment says what the export covers and which known places bound it.
type Segment struct {
	Kind      string `json:"kind"`
	FromUTC   string `json:"from_utc,omitempty"`
	ToUTC     string `json:"to_utc,omitempty"`
	StartSite string `json:"start_site,omitempty"`
	EndSite   string `json:"end_site,omitempty"`
}

// Mount is the operator's record of the sensor mount; nil means not recorded.
type Mount struct {
	Platform string   `json:"platform,omitempty"`
	HeightM  *float64 `json:"height_m,omitempty"`
	RollDeg  *float64 `json:"roll_deg,omitempty"`
	PitchDeg *float64 `json:"pitch_deg,omitempty"`
	Notes    string   `json:"notes,omitempty"`
}

// TimeInfo states the point-time convention and its measured quality.
type TimeInfo struct {
	EpochUTC string `json:"epoch_utc"`
	// RotationHz is the source's measured rotation rate, 1 / MedianPeriodS.
	RotationHz float64 `json:"rotation_hz"`
	Source     string  `json:"source"`
	Convention string  `json:"convention"`
	// MedianPeriodS and the residual percentiles are over exported rotations.
	MedianPeriodS   float64 `json:"median_period_s"`
	MedianResidUS   float64 `json:"median_resid_us"`
	P95ResidUS      float64 `json:"p95_resid_us"`
	ClockFitFailed  int     `json:"clock_fit_failed"`
	PeriodOutliers  int     `json:"period_outliers"`
	MaxPeriodDevPct float64 `json:"max_period_deviation_pct"`
}

// Selection records the decimation applied.
type Selection struct {
	AzimuthStride     int     `json:"azimuth_stride"`
	KeepEveryRotation int     `json:"keep_every_rotation"`
	MaxRangeM         float64 `json:"max_range_m,omitempty"`
	IncludeIncomplete bool    `json:"include_incomplete"`
	SplitSeconds      float64 `json:"split_seconds"`
}

// PointFile is one LAS file of the export.
type PointFile struct {
	FileDigest
	Points     uint64  `json:"points"`
	FirstFrame uint32  `json:"first_frame"`
	LastFrame  uint32  `json:"last_frame"`
	TimeMinS   float64 `json:"time_min_s"`
	TimeMaxS   float64 `json:"time_max_s"`
}

// FrameStats counts what happened to every rotation in the window.
type FrameStats struct {
	Seen       int `json:"seen"`
	Exported   int `json:"exported"`
	Incomplete int `json:"skipped_incomplete"`
	Decimated  int `json:"skipped_decimation"`
	OutOfRange int `json:"outside_window"`
	PacketGaps int `json:"packet_gaps"`
}

// FrameRecord is one rotation's line in frames.csv.
type FrameRecord struct {
	Frame      uint32
	Rotation   int // index among complete rotations in the window, before decimation
	StartS     float64
	Points     int
	Complete   bool
	PacketGaps int
	Clock      ClockFit
}

// Exporter receives rotations in capture order and writes the files.
type Exporter struct {
	cfg      Config
	man      Manifest
	w        *LASWriter
	file     PointFile
	fileT0   time.Time
	frame    uint32
	rotation int
	records  []FrameRecord
	csv      *os.File
	err      error
}

// NewExporter prepares OutDir, which must not exist or must be empty.
func NewExporter(cfg Config, man Manifest) (*Exporter, error) {
	if cfg.TimeSource == "" {
		cfg.TimeSource = TimeAzimuthClock
	}
	if cfg.TimeSource != TimeAzimuthClock && cfg.TimeSource != TimePacket {
		return nil, fmt.Errorf("time source %q", cfg.TimeSource)
	}
	if cfg.SplitSeconds <= 0 {
		cfg.SplitSeconds = 60
	}
	if cfg.AzimuthStride <= 0 {
		cfg.AzimuthStride = 1
	}
	if cfg.KeepEveryRotation <= 0 {
		cfg.KeepEveryRotation = 1
	}
	if cfg.MaxFlaggedPeriodDeviation <= 0 {
		cfg.MaxFlaggedPeriodDeviation = 0.02
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return nil, err
	}
	if ents, err := os.ReadDir(cfg.OutDir); err != nil || len(ents) > 0 {
		if err == nil {
			err = fmt.Errorf("output directory %s is not empty", cfg.OutDir)
		}
		return nil, err
	}
	csv, err := os.OpenFile(filepath.Join(cfg.OutDir, "frames.csv"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	fmt.Fprintln(csv, "frame,rotation,start_s,points,complete,packet_gaps,clock_ok,period_s,rms_resid_us,max_resid_us,blocks,rejected")
	man.Schema = ManifestSchema
	man.Note = "provisional: Gerardus plans run.json as a proto message; point time is NOT GPS time"
	man.Time.Source = cfg.TimeSource
	man.Time.Convention = map[string]string{
		TimeAzimuthClock: "seconds since epoch_utc; each rotation's block time is a least-squares line in unwrapped block azimuth fitted to the recorded block times (PCAP arrival time + channel firetime during replay), plus the point's channel offset within its block",
		TimePacket:       "seconds since epoch_utc of the recorded point time: PCAP arrival time + channel firetime during replay; per-block offsets are absent",
	}[cfg.TimeSource]
	man.Coordinates = map[string]string{
		"frame": "sensor, before any motion correction",
		"axes":  "x = r cos(el) sin(az), y = r cos(el) cos(az), z = r sin(el); l2frames.SphericalToCartesian",
		"units": "metres; LAS grid 0.0005 m",
	}
	man.Selection = Selection{AzimuthStride: cfg.AzimuthStride, KeepEveryRotation: cfg.KeepEveryRotation, MaxRangeM: cfg.MaxRangeM, IncludeIncomplete: cfg.IncludeIncomplete, SplitSeconds: cfg.SplitSeconds}
	if man.Segment.Kind == "" {
		man.Segment.Kind = "motion"
	}
	if !cfg.From.IsZero() {
		man.Segment.FromUTC = cfg.From.UTC().Format(time.RFC3339Nano)
	}
	if !cfg.To.IsZero() {
		man.Segment.ToUTC = cfg.To.UTC().Format(time.RFC3339Nano)
	}
	return &Exporter{cfg: cfg, man: man, csv: csv}, nil
}

// AddFrame exports one rotation. Errors are sticky; Close reports them.
func (e *Exporter) AddFrame(f *l2frames.LiDARFrame) error {
	if e.err != nil || f == nil {
		return e.err
	}
	start := f.StartTimestamp
	if (!e.cfg.From.IsZero() && start.Before(e.cfg.From)) || (!e.cfg.To.IsZero() && start.After(e.cfg.To)) {
		e.man.Frames.OutOfRange++
		return nil
	}
	e.man.Frames.Seen++
	e.man.Frames.PacketGaps += f.PacketGaps
	if !f.SpinComplete && !e.cfg.IncludeIncomplete {
		e.man.Frames.Incomplete++
		return nil
	}
	rotation := e.rotation
	e.rotation++
	if rotation%e.cfg.KeepEveryRotation != 0 {
		e.man.Frames.Decimated++
		return nil
	}
	if e.cfg.Epoch.IsZero() {
		e.cfg.Epoch = start.Truncate(time.Second)
	}
	if e.w == nil || start.Sub(e.fileT0).Seconds() >= e.cfg.SplitSeconds {
		if e.err = e.closeFile(); e.err != nil {
			return e.err
		}
		if e.err = e.openFile(start); e.err != nil {
			return e.err
		}
	}
	times, fit := PointTimes(f.Points, e.cfg.Epoch, e.cfg.TimeSource == TimeAzimuthClock)
	keep := e.blockStride(f.Points)
	n := 0
	for i, p := range f.Points {
		if p.Distance <= 0 || math.IsNaN(p.Distance) || (e.cfg.MaxRangeM > 0 && p.Distance > e.cfg.MaxRangeM) || !keep[i] {
			continue
		}
		if p.Channel < 1 || p.Channel > 255 {
			continue
		}
		if e.err = e.w.Write(LASPoint{X: p.X, Y: p.Y, Z: p.Z, Intensity: p.Intensity, Ring: uint8(p.Channel), AzimuthDeg: float32(p.Azimuth), Frame: e.frame, TimeS: times[i]}); e.err != nil {
			return e.err
		}
		n++
	}
	if e.file.Points == 0 {
		e.file.FirstFrame = e.frame
	}
	e.file.LastFrame = e.frame
	e.file.Points = e.w.Count()
	rec := FrameRecord{Frame: e.frame, Rotation: rotation, StartS: start.Sub(e.cfg.Epoch).Seconds(), Points: n, Complete: f.SpinComplete, PacketGaps: f.PacketGaps, Clock: fit}
	e.records = append(e.records, rec)
	fmt.Fprintf(e.csv, "%d,%d,%.6f,%d,%t,%d,%t,%.6f,%.1f,%.1f,%d,%d\n", rec.Frame, rec.Rotation, rec.StartS, rec.Points, rec.Complete, rec.PacketGaps, fit.OK, fit.PeriodS, fit.RMSResidUS, fit.MaxResidUS, fit.Blocks, fit.Rejected)
	e.man.Frames.Exported++
	e.frame++
	return nil
}

// blockStride keeps every AzimuthStride-th block of the rotation, in time
// order, so decimation removes whole azimuth columns rather than rings.
func (e *Exporter) blockStride(points []l2frames.Point) []bool {
	keep := make([]bool, len(points))
	if e.cfg.AzimuthStride == 1 {
		for i := range keep {
			keep[i] = true
		}
		return keep
	}
	type first struct {
		t time.Time
		i int
	}
	seen := map[blockKey]int{}
	var order []first
	// Blocks of one packet share a recorded time, so the stable sort keeps
	// their packet order.
	for _, p := range points {
		k := blockKey{p.PacketKey, p.UDPSequence, p.BlockID}
		if _, ok := seen[k]; !ok {
			seen[k] = len(order)
			order = append(order, first{p.Timestamp, len(order)})
		}
	}
	sort.SliceStable(order, func(a, b int) bool { return order[a].t.Before(order[b].t) })
	rank := make([]int, len(order))
	for r, o := range order {
		rank[o.i] = r
	}
	for i, p := range points {
		keep[i] = rank[seen[blockKey{p.PacketKey, p.UDPSequence, p.BlockID}]]%e.cfg.AzimuthStride == 0
	}
	return keep
}

func (e *Exporter) openFile(start time.Time) error {
	name := fmt.Sprintf("segment-%04d.las", len(e.man.Files))
	w, err := CreateLAS(filepath.Join(e.cfg.OutDir, name), e.cfg.Software, time.Now())
	if err != nil {
		return err
	}
	e.w, e.fileT0 = w, start
	e.file = PointFile{FileDigest: FileDigest{Path: name}}
	return nil
}

func (e *Exporter) closeFile() error {
	if e.w == nil {
		return nil
	}
	e.file.TimeMinS, e.file.TimeMaxS = e.w.TimeRange()
	if err := e.w.Close(); err != nil {
		return err
	}
	d, err := Digest(filepath.Join(e.cfg.OutDir, e.file.Path))
	if err != nil {
		return err
	}
	e.file.SHA256, e.file.Bytes = d.SHA256, d.Bytes
	e.man.Files = append(e.man.Files, e.file)
	e.w = nil
	return nil
}

// Close finishes the last file, summarises timing and writes run.json.
func (e *Exporter) Close() (Manifest, error) {
	err := e.err
	if cerr := e.closeFile(); err == nil {
		err = cerr
	}
	if cerr := e.csv.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return e.man, err
	}
	if e.man.Frames.Exported == 0 {
		return e.man, errors.New("no rotation was exported")
	}
	e.summariseTime()
	b, err := json.MarshalIndent(e.man, "", "  ")
	if err != nil {
		return e.man, err
	}
	f, err := os.OpenFile(filepath.Join(e.cfg.OutDir, "run.json"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return e.man, err
	}
	_, err = f.Write(append(b, '\n'))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return e.man, err
}

func (e *Exporter) summariseTime() {
	t := &e.man.Time
	t.EpochUTC = e.cfg.Epoch.UTC().Format(time.RFC3339Nano)
	var periods, resid []float64
	for _, r := range e.records {
		if !r.Clock.OK {
			t.ClockFitFailed++
			continue
		}
		periods = append(periods, r.Clock.PeriodS)
		resid = append(resid, r.Clock.RMSResidUS)
	}
	if len(periods) == 0 {
		return
	}
	sort.Float64s(resid)
	t.MedianResidUS = resid[len(resid)/2]
	t.P95ResidUS = resid[int(0.95*float64(len(resid)-1))]
	med := append([]float64(nil), periods...)
	sort.Float64s(med)
	t.MedianPeriodS = med[len(med)/2]
	t.RotationHz = 1 / t.MedianPeriodS
	for _, p := range periods {
		dev := math.Abs(p-t.MedianPeriodS) / t.MedianPeriodS
		t.MaxPeriodDevPct = math.Max(t.MaxPeriodDevPct, 100*dev)
		if dev > e.cfg.MaxFlaggedPeriodDeviation {
			t.PeriodOutliers++
		}
	}
}

// Digest hashes a file.
func Digest(path string) (FileDigest, error) {
	f, err := os.Open(path)
	if err != nil {
		return FileDigest{}, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return FileDigest{}, err
	}
	return FileDigest{Path: filepath.Base(path), SHA256: hex.EncodeToString(h.Sum(nil)), Bytes: n}, nil
}
