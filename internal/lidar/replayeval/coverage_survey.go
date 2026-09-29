package replayeval

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	observationsqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

// Coverage survey: a ContinuityCoverage declaration measured from a replay.
//
// kirk0's declaration was read off a replay by hand: its online estimates
// reached 91.7 m, so it declared 92 m, and it assumed the full circle.
// SurveyContinuityCoverage takes that measurement from any default replay
// that wrote an evidence database, so each corpus site's declaration can be
// reproduced and says how it was made.
//
// Range. The horizontal range, from the sensor origin, of every online
// estimate the replay persisted: confirmed tracks associated in their frame.
// max_range_m is the chosen percentile of those ranges, by nearest rank,
// rounded up to the next whole metre. The default, 100, is the maximum and
// reproduces kirk0's hand reading. min_range_m stays 0: the near blind zone
// is not measured here.
//
// Azimuth. The same estimates, binned by whole degree anticlockwise from +X.
// When one arc of at least MinSectorGapDeg never holds an estimate, the
// declaration is the sector outside it; otherwise it is the full circle. An
// arc no road user ever occupied in the whole replay is one the continuity
// experiments may then call out of view, so the per-10-degree counts and the
// empty arc are kept in the statistics, to be read before the declaration is
// committed.
//
// Origin. The replay tracks in the sensor frame with no pose, so the origin
// is (0, 0), derived from the identity tracking transform exactly as the
// solid body's is (OriginTrackingTransformIdentity).
//
// Only a default replay is surveyed: no experiments, the production
// measurement model, and one online estimate version, the replay's own.
// Surface-relative ground clipping is not in the replay manifest, so the
// caller refuses it.

// DefaultCoverageRangePercentile declares the maximum observed range.
const DefaultCoverageRangePercentile = 100

// DefaultCoverageMinSectorGapDeg is the narrowest empty arc that makes a
// declaration a sector: a quarter of the circle with no road user in it.
const DefaultCoverageMinSectorGapDeg = 90

// CoverageSurveyOptions chooses how a declaration is read from a replay.
type CoverageSurveyOptions struct {
	// CaseID and Tool name the case and the command that surveyed it.
	CaseID, Tool string
	// RangePercentile is in (0, 100]; MinSectorGapDeg in (0, 360].
	RangePercentile float64
	MinSectorGapDeg float64
}

// Validate reports the first way the options cannot run a survey.
func (o CoverageSurveyOptions) Validate() error {
	if strings.TrimSpace(o.CaseID) == "" || strings.TrimSpace(o.Tool) == "" {
		return errors.New("coverage survey needs a case ID and the tool that ran it")
	}
	if !(o.RangePercentile > 0 && o.RangePercentile <= 100) {
		return fmt.Errorf("coverage survey range percentile %g must be in (0, 100]", o.RangePercentile)
	}
	if !(o.MinSectorGapDeg > 0 && o.MinSectorGapDeg <= 360) {
		return fmt.Errorf("coverage survey minimum sector gap %g must be in (0, 360] degrees", o.MinSectorGapDeg)
	}
	return nil
}

// CoverageSurvey is a surveyed declaration and what it was measured from.
type CoverageSurvey struct {
	Declaration ContinuityCoverage  `json:"declaration"`
	Stats       CoverageSurveyStats `json:"stats"`
}

// CoverageSurveyStats is what a survey measured, kept beside the declaration.
type CoverageSurveyStats struct {
	CaseID        string `json:"case_id"`
	DeclarationID string `json:"declaration_id"`
	Tool          string `json:"tool"`
	BuildVersion  string `json:"build_version"`
	BuildGitSHA   string `json:"build_git_sha"`
	BuildStamped  bool   `json:"build_stamped"`
	// CaptureSetSHA256 is the SHA-256 of the case's capture digests, in
	// order, one per line: the case's source, independent of where it lives.
	CaptureSHA256s      []string `json:"capture_sha256s"`
	CaptureSetSHA256    string   `json:"capture_set_sha256"`
	ParamsSHA256        string   `json:"params_sha256"`
	ObservationSourceID string   `json:"observation_source_id"`
	FramesRecorded      int      `json:"frames_recorded"`
	OnlineEstimates     int      `json:"online_estimates"`
	Tracks              int      `json:"tracks"`
	SensorOrigin        struct {
		XMetres float64 `json:"x_m"`
		YMetres float64 `json:"y_m"`
		Source  string  `json:"source"`
	} `json:"sensor_origin"`
	RangeMetres struct {
		Min float64 `json:"min"`
		P50 float64 `json:"p50"`
		P90 float64 `json:"p90"`
		P95 float64 `json:"p95"`
		P99 float64 `json:"p99"`
		Max float64 `json:"max"`
	} `json:"horizontal_range_m"`
	RangePercentile   float64       `json:"range_percentile"`
	RangeAtPercentile float64       `json:"range_at_percentile_m"`
	Azimuth           SurveyAzimuth `json:"azimuth"`
}

// SurveyAzimuth is the azimuth measurement: whole-degree bins, the largest
// arc without an estimate, and the counts by 10 degrees from +X.
type SurveyAzimuth struct {
	OccupiedDegrees        int     `json:"occupied_degrees"`
	LargestEmptyArcFromDeg int     `json:"largest_empty_arc_from_deg"`
	LargestEmptyArcDeg     int     `json:"largest_empty_arc_deg"`
	MinSectorGapDeg        float64 `json:"min_sector_gap_deg"`
	Sector                 bool    `json:"sector"`
	EstimatesPer10Deg      [36]int `json:"estimates_per_10_deg"`
}

// surveyManifest is the part of replay_manifest.json a survey depends on.
type surveyManifest struct {
	SourceSHA256s         []string `json:"source_sha256s"`
	ParamsSHA256          string   `json:"params_sha256"`
	BuildVersion          string   `json:"build_version"`
	BuildGitSHA           string   `json:"build_git_sha"`
	BuildStamped          bool     `json:"build_stamped"`
	Experiments           []string `json:"experiments"`
	MeasurementSourceMode string   `json:"measurement_source_mode"`
	ObservationSourceID   string   `json:"observation_source_id"`
	FramesRecorded        int      `json:"frames_recorded"`
}

// SurveyContinuityCoverage measures a declaration from a replay: replayDir is
// the replay's output directory, whose replay_manifest.json says what ran,
// and database the evidence database it wrote.
func SurveyContinuityCoverage(replayDir string, database observationsqlite.DBClient, opts CoverageSurveyOptions) (*CoverageSurvey, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	m, err := readSurveyManifest(replayDir)
	if err != nil {
		return nil, err
	}
	estimates, err := observationsqlite.NewStateEstimateStore(database).ListBySource(m.ObservationSourceID)
	if err != nil {
		return nil, err
	}
	return measureCoverage(opts, m, estimates)
}

func readSurveyManifest(replayDir string) (surveyManifest, error) {
	var m surveyManifest
	b, err := os.ReadFile(filepath.Join(replayDir, "replay_manifest.json"))
	if err != nil {
		return m, fmt.Errorf("coverage survey: %w", err)
	}
	if err := json.Unmarshal(b, &m); err != nil {
		return m, fmt.Errorf("coverage survey: parse replay manifest: %w", err)
	}
	switch {
	case len(m.Experiments) > 0:
		return m, fmt.Errorf("coverage survey reads the default replay; this one ran experiments %v", m.Experiments)
	case m.MeasurementSourceMode != string(l5tracks.MeasurementMedoidV0):
		return m, fmt.Errorf("coverage survey reads the default replay; this one measured %q", m.MeasurementSourceMode)
	case m.ObservationSourceID == "":
		return m, errors.New("coverage survey needs the replay's online estimates, and this replay wrote no evidence database")
	case len(m.SourceSHA256s) == 0:
		return m, errors.New("coverage survey: the replay manifest names no capture digests")
	}
	return m, nil
}

// measureCoverage is the survey proper, over a replay's online estimates.
func measureCoverage(opts CoverageSurveyOptions, m surveyManifest, estimates []observationsqlite.TrackEstimate) (*CoverageSurvey, error) {
	if len(estimates) == 0 {
		return nil, fmt.Errorf("coverage survey: source %s has no online estimates to measure", m.ObservationSourceID)
	}
	var s CoverageSurveyStats
	s.CaseID, s.Tool, s.ParamsSHA256, s.ObservationSourceID = opts.CaseID, opts.Tool, m.ParamsSHA256, m.ObservationSourceID
	s.BuildVersion, s.BuildGitSHA, s.BuildStamped = m.BuildVersion, m.BuildGitSHA, m.BuildStamped
	s.CaptureSHA256s, s.FramesRecorded, s.OnlineEstimates = m.SourceSHA256s, m.FramesRecorded, len(estimates)
	s.CaptureSetSHA256 = "sha256:" + hex.EncodeToString(sha256Sum([]byte(strings.Join(m.SourceSHA256s, "\n"))))
	s.SensorOrigin.Source = OriginTrackingTransformIdentity

	ranges := make([]float64, len(estimates))
	var bins [360]int
	tracks := map[int64]bool{}
	for i, e := range estimates {
		// One version only, and the replay's own: another would measure some
		// other configuration's envelope under this replay's name.
		if e.ParamHash != m.ParamsSHA256 {
			return nil, fmt.Errorf("coverage survey: estimate %s has parameter hash %s; the replay's is %s",
				e.EstimateID, e.ParamHash, m.ParamsSHA256)
		}
		x, y := float64(e.X)-s.SensorOrigin.XMetres, float64(e.Y)-s.SensorOrigin.YMetres
		ranges[i] = math.Hypot(x, y)
		deg := math.Atan2(y, x) * 180 / math.Pi
		if deg < 0 {
			deg += 360
		}
		bin := int(deg) % 360
		bins[bin]++
		s.Azimuth.EstimatesPer10Deg[bin/10]++
		tracks[e.CreationSequence] = true
	}
	s.Tracks = len(tracks)
	sort.Float64s(ranges)
	rank := func(p float64) float64 { return ranges[int(math.Ceil(p/100*float64(len(ranges))))-1] }
	centimetres := func(v float64) float64 { return math.Round(v*100) / 100 }
	r := &s.RangeMetres
	r.Min, r.P50, r.P90 = centimetres(ranges[0]), centimetres(rank(50)), centimetres(rank(90))
	r.P95, r.P99, r.Max = centimetres(rank(95)), centimetres(rank(99)), centimetres(ranges[len(ranges)-1])
	s.RangePercentile = opts.RangePercentile
	// Rounded to the centimetre before rounding up, so a float32 position a
	// hair beyond a whole metre does not add a metre to the envelope.
	atPercentile := centimetres(rank(opts.RangePercentile))
	s.RangeAtPercentile = atPercentile

	from, width := largestEmptyArc(bins)
	for _, n := range bins {
		if n > 0 {
			s.Azimuth.OccupiedDegrees++
		}
	}
	s.Azimuth.LargestEmptyArcFromDeg, s.Azimuth.LargestEmptyArcDeg = from, width
	s.Azimuth.MinSectorGapDeg = opts.MinSectorGapDeg
	s.Azimuth.Sector = float64(width) >= opts.MinSectorGapDeg

	d := ContinuityCoverage{MaxRangeMetres: float32(math.Ceil(atPercentile)), AzimuthHalfWidthDeg: 180}
	rangeWords := fmt.Sprintf("the p%g online-estimate range", opts.RangePercentile)
	if opts.RangePercentile == 100 {
		rangeWords = "the maximum online-estimate range"
	}
	azimuthWords := fmt.Sprintf("full circle: the largest arc without an estimate is %d deg, under the %g deg that makes a sector",
		width, opts.MinSectorGapDeg)
	if s.Azimuth.Sector {
		start := (from + width) % 360
		d.AzimuthHalfWidthDeg = float32(360-width) / 2
		d.AzimuthCentreDeg = float32(math.Remainder(float64(start)+float64(360-width)/2, 360))
		azimuthWords = fmt.Sprintf("sector from %d to %d deg anticlockwise: no estimate in the %d deg from %d (sector threshold %g deg)",
			start, from, width, from, opts.MinSectorGapDeg)
	}
	stamp := "stamped"
	if !m.BuildStamped {
		stamp = "unstamped"
	}
	d.Source = fmt.Sprintf("measured by %s (build %s, git %s, %s): case %s, captures %s, default replay params %s; "+
		"max_range_m is %s, %.2f m, rounded up; %s",
		opts.Tool, m.BuildVersion, m.BuildGitSHA, stamp, opts.CaseID, s.CaptureSetSHA256, m.ParamsSHA256,
		rangeWords, atPercentile, azimuthWords)
	if err := d.Validate(); err != nil {
		return nil, fmt.Errorf("coverage survey of %s: %w", opts.CaseID, err)
	}
	s.DeclarationID = d.ID()
	return &CoverageSurvey{Declaration: d, Stats: s}, nil
}

// largestEmptyArc returns the start and width in degrees of the longest run of
// empty bins round the circle. At least one bin must be occupied.
func largestEmptyArc(bins [360]int) (from, width int) {
	first := 0
	for bins[first] == 0 {
		first++
	}
	runStart, runLen := 0, 0
	for step := 1; step <= 360; step++ {
		i := (first + step) % 360
		if bins[i] == 0 {
			if runLen == 0 {
				runStart = i
			}
			runLen++
			continue
		}
		if runLen > width {
			from, width = runStart, runLen
		}
		runLen = 0
	}
	return from, width
}

// CoverageSurveyStatsPath is where a declaration set's survey statistics are
// kept: beside it, as <name>.survey.json.
func CoverageSurveyStatsPath(setPath string) string {
	return strings.TrimSuffix(setPath, ".json") + ".survey.json"
}

// CheckCoverageSurveyTargets refuses a survey into a set, or its statistics,
// that already declare one of the cases: a re-survey goes to a new set, never
// over a committed declaration. Either file may be absent.
func CheckCoverageSurveyTargets(setPath string, caseIDs []string) error {
	set, stats, err := readCoverageSurveyFiles(setPath)
	if err != nil {
		return err
	}
	for _, id := range caseIDs {
		_, inSet := set[id]
		_, inStats := stats[id]
		if inSet || inStats {
			return fmt.Errorf("case %s is already declared in %s or %s; survey it into a new set",
				id, setPath, CoverageSurveyStatsPath(setPath))
		}
	}
	return nil
}

// AddCoverageSurvey adds a surveyed case to the declaration set at setPath,
// in the form LoadContinuityCoverageSet reads, and its statistics to the
// record beside it. Either file is created if absent, and neither replaces a
// case it already holds.
func AddCoverageSurvey(setPath string, s *CoverageSurvey) error {
	if err := s.Declaration.Validate(); err != nil {
		return err
	}
	if err := CheckCoverageSurveyTargets(setPath, []string{s.Stats.CaseID}); err != nil {
		return err
	}
	// The check has just read both files whole.
	set, stats, _ := readCoverageSurveyFiles(setPath)
	set[s.Stats.CaseID] = s.Declaration
	stats[s.Stats.CaseID] = s.Stats
	if err := writeJSONFile(setPath, set); err != nil {
		return fmt.Errorf("write continuity coverage set: %w", err)
	}
	if err := writeJSONFile(CoverageSurveyStatsPath(setPath), stats); err != nil {
		return fmt.Errorf("write coverage survey statistics: %w", err)
	}
	return nil
}

func readCoverageSurveyFiles(setPath string) (map[string]ContinuityCoverage, map[string]CoverageSurveyStats, error) {
	set := map[string]ContinuityCoverage{}
	if _, err := os.Stat(setPath); err == nil {
		if set, err = LoadContinuityCoverageSet(setPath); err != nil {
			return nil, nil, err
		}
	}
	stats := map[string]CoverageSurveyStats{}
	b, err := os.ReadFile(CoverageSurveyStatsPath(setPath))
	if errors.Is(err, os.ErrNotExist) {
		return set, stats, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read coverage survey statistics: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&stats); err != nil {
		return nil, nil, fmt.Errorf("parse coverage survey statistics: %w", err)
	}
	return set, stats, nil
}

// writeJSONFile replaces path with v's indented encoding through a temporary
// file beside it, so a failed write leaves the previous file whole. Maps
// encode with sorted keys, so a set reads the same whatever order its cases
// were surveyed in.
func writeJSONFile(path string, v any) error {
	b, _ := json.MarshalIndent(v, "", "  ")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
