package main

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"sort"

	"github.com/banshee-data/velocity.report/internal/lidar/l3grid"
	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
	"github.com/banshee-data/velocity.report/internal/lidar/replayeval"
)

// groundReportFile is where -ground-report writes a case's report, in the
// first run's output directory.
const groundReportFile = "ground_report.json"

// ground0001's thresholds, fixed with the instrument (2026-10-26) before any
// replay read them.
const (
	// groundMovingMps is the median speed from which a confirmed track is a
	// moving one, whose course and lowest return are read.
	groundMovingMps = 2.0
	// A slope is fitted only from groundMinSamples observed frames over
	// groundMinLengthMetres of travel or more.
	groundMinSamples      = 10
	groundMinLengthMetres = 5.0
	// A lowest return rises or falls with distance when its slope is
	// groundNotableSlope or more, it changes by groundNotableRiseMetres or
	// more over the track, and the slope is groundNotableSE standard errors
	// or more from zero.
	groundNotableSlope      = 0.01
	groundNotableRiseMetres = 0.10
	groundNotableSE         = 2.0
	// groundListedTracks caps the tracks a case lists.
	groundListedTracks = 25
	// groundBandRangeMetres is the range at which the fixed band's height
	// error is predicted.
	groundBandRangeMetres = 30.0
)

// The classes of a moving track's lowest return, by whether it rises or falls
// with distance in the sensor frame and above the surface.
const (
	groundClassFlat        = "flat"         // in neither
	groundClassExplained   = "explained"    // in the sensor frame only: the surface explains it
	groundClassSurfaceOnly = "surface_only" // above the surface only
	groundClassBoth        = "both"         // in both
)

// groundReport is ground0001's measurement for one case: the P11 regional
// surface fitted from the settled background beside the defaults' tracks,
// read and never applied, and what the confirmed moving tracks show against
// it. Angles are in the sensor frame, degrees counter-clockwise from +x.
type groundReport struct {
	SchemaVersion int                 `json:"schema_version"`
	Summary       groundReportSummary `json:"summary"`
	// Tracks are the moving tracks a slope was fitted for, in the order
	// they were first observed.
	Tracks []groundTrackRow `json:"tracks"`
}

// groundReportSummary is the part of a groundReport the case summary keeps.
type groundReportSummary struct {
	Thresholds groundThresholds `json:"thresholds"`
	// Surface is the fit, nil when the background never settled to give one.
	Surface       *groundSurfaceRow `json:"surface,omitempty"`
	ScoredFrames  int               `json:"scored_frames"`
	ConfirmedSeen int               `json:"confirmed_tracks"`
	MovingTracks  int               `json:"moving_tracks"`
	MovingSamples int               `json:"moving_samples"`
	Course        groundCourse      `json:"course"`
	Departure     groundDeparture   `json:"departure"`
	SlopeSensor   groundSlopeStats  `json:"slope_sensor"`
	SlopeSurface  groundSlopeStats  `json:"slope_above_surface"`
	SlopePlane    groundSlopeStats  `json:"slope_above_plane"`
	Classes       map[string]int    `json:"classes"`
	// PlaneClasses classes the same tracks against the global plane alone,
	// the one-plane control of ground0002, beside Classes' blended cells.
	PlaneClasses   map[string]int `json:"plane_classes,omitempty"`
	ListedTrackIDs []string       `json:"listed_tracks,omitempty"`
}

type groundThresholds struct {
	MovingMps         float64 `json:"moving_mps"`
	MinSamples        int     `json:"min_samples"`
	MinLengthMetres   float64 `json:"min_length_metres"`
	NotableSlope      float64 `json:"notable_slope"`
	NotableRiseMetres float64 `json:"notable_rise_metres"`
	NotableSE         float64 `json:"notable_standard_errors"`
}

// groundSurfaceRow is the global plane z = A·x + B·y + C, its fall line, the
// cells with a plane of their own, and the two errors the geometry predicts
// from the grade g.
type groundSurfaceRow struct {
	A                  float64 `json:"a"`
	B                  float64 `json:"b"`
	C                  float64 `json:"c"`
	Support            int     `json:"support"`
	RMSEMetres         float64 `json:"rmse_metres"`
	Gradient           float64 `json:"gradient"`
	FallLineBearingDeg float64 `json:"fall_line_bearing_deg"`
	RegionCount        int     `json:"region_count"`
	CellMetres         float64 `json:"cell_metres"`
	// PlanarSpeedScaleError is 1 − cos(atan g): how much shorter the planar
	// speed is than the speed along the plane.
	PlanarSpeedScaleError float64 `json:"planar_speed_scale_error"`
	// BandHeightError30m is 30·g: how far the fixed band moves against the
	// plane 30 m along the fall line.
	BandHeightError30m float64 `json:"band_height_error_30m_metres"`
}

// groundCourse is the moving tracks' course against the fall line. The axis
// is the length-weighted axial mean of the tracks' net displacements, and
// its concentration 1 when every track runs on it (0.5 or less at a
// crossing of two equal flows). The grade components are the global plane's
// rise per metre along and across each track's course, medians over the
// tracks.
type groundCourse struct {
	AxisDeg            float64 `json:"axis_deg"`
	AxisConcentration  float64 `json:"axis_concentration"`
	FallLineToAxisDeg  float64 `json:"fall_line_to_axis_deg"`
	GradeAlongMedian   float64 `json:"grade_along_median"`
	GradeAcrossMedian  float64 `json:"grade_across_median"`
	GradeAlongMaxTrack float64 `json:"grade_along_max"`
}

// groundDeparture is the cell planes' height less the global plane's at the
// moving tracks' positions, one sample a track a frame: absolute p50, p90
// and largest, the signed median, and the share of samples in a cell with a
// plane of its own (the rest are served by the global plane, departure 0).
type groundDeparture struct {
	Samples      int     `json:"samples"`
	P50Metres    float64 `json:"p50_metres"`
	P90Metres    float64 `json:"p90_metres"`
	MaxMetres    float64 `json:"max_metres"`
	SignedMedian float64 `json:"signed_median_metres"`
	OwnCellShare float64 `json:"own_cell_share"`
}

// groundSlopeStats are the absolute slopes over the moving tracks.
type groundSlopeStats struct {
	MedianAbs float64 `json:"median_abs"`
	P90Abs    float64 `json:"p90_abs"`
}

// groundTrackRow is one moving track: its lowest return's least-squares slope
// against along-track distance in the sensor frame and above the surface,
// with standard errors, and the global plane's signed rise per metre along
// its course, which the sensor-frame slope should match where the track
// follows the plane.
type groundTrackRow struct {
	TrackID        string  `json:"track_id"`
	Samples        int     `json:"samples"`
	LengthMetres   float64 `json:"length_metres"`
	MedianMps      float64 `json:"median_mps"`
	CourseDeg      float64 `json:"course_deg"`
	RangeStart     float64 `json:"range_start_metres"`
	RangeEnd       float64 `json:"range_end_metres"`
	GradeAlong     float64 `json:"grade_along"`
	SlopeSensor    float64 `json:"slope_sensor"`
	SlopeSensorSE  float64 `json:"slope_sensor_se"`
	SlopeSurface   float64 `json:"slope_above_surface"`
	SlopeSurfaceSE float64 `json:"slope_above_surface_se"`
	// SlopePlane is the slope of the lowest return's height above the global
	// plane alone, and PlaneClass the class it gives.
	SlopePlane   float64 `json:"slope_above_plane"`
	SlopePlaneSE float64 `json:"slope_above_plane_se"`
	Class        string  `json:"class"`
	PlaneClass   string  `json:"plane_class,omitempty"`
}

type groundSample struct {
	at         int64
	x, y, z, v float64
}

// groundReporter gathers a replay's confirmed, observed tracks from its
// TrackerTap over the scoring window, one sample a track a frame. It reads
// tracker state and changes nothing.
type groundReporter struct {
	frames int
	lastAt int64
	order  []string
	tracks map[string][]groundSample
}

func newGroundReporter() *groundReporter {
	return &groundReporter{tracks: map[string][]groundSample{}}
}

func (g *groundReporter) observe(f replayeval.TrackerTapFrame) {
	if !f.Scored {
		return
	}
	at := f.At.UnixNano()
	if at != g.lastAt || g.frames == 0 {
		g.frames++
		g.lastAt = at
	}
	for _, t := range f.Tracks {
		if t == nil || t.TrackState != l5tracks.TrackConfirmed || t.Misses != 0 {
			continue
		}
		samples, seen := g.tracks[t.TrackID]
		if n := len(samples); n > 0 && samples[n-1].at == at {
			continue
		}
		if !seen {
			g.order = append(g.order, t.TrackID)
		}
		// LatestZ is the associated cluster's lowest return in this frame.
		g.tracks[t.TrackID] = append(samples, groundSample{at: at, x: float64(t.X), y: float64(t.Y),
			z: float64(t.LatestZ), v: float64(t.Speed())})
	}
}

// report reads the gathered tracks against the fitted surface, or nil.
func (g *groundReporter) report(surface *l3grid.RegionalGroundSurface) groundReport {
	s := groundReportSummary{
		Thresholds: groundThresholds{MovingMps: groundMovingMps, MinSamples: groundMinSamples,
			MinLengthMetres: groundMinLengthMetres, NotableSlope: groundNotableSlope,
			NotableRiseMetres: groundNotableRiseMetres, NotableSE: groundNotableSE},
		ScoredFrames:  g.frames,
		ConfirmedSeen: len(g.order),
		Classes:       map[string]int{},
	}
	if surface != nil {
		s.PlaneClasses = map[string]int{}
	}
	var a, b float64
	if surface != nil {
		gl := surface.Global
		a, b = gl.A, gl.B
		grad := gl.GradientMetre
		s.Surface = &groundSurfaceRow{A: gl.A, B: gl.B, C: gl.C, Support: gl.Support, RMSEMetres: gl.RMSEMetres,
			Gradient: grad, FallLineBearingDeg: bearingDeg(-gl.A, -gl.B), RegionCount: surface.RegionCount,
			CellMetres: surface.CellMetres, PlanarSpeedScaleError: 1 - math.Cos(math.Atan(grad)),
			BandHeightError30m: groundBandRangeMetres * grad}
	}
	rows := make([]groundTrackRow, 0)
	var departures, signed []float64
	ownCell := 0
	var sumC, sumS, sumW float64
	var along, across []float64
	for _, id := range g.order {
		samples := g.tracks[id]
		if len(samples) < groundMinSamples {
			continue
		}
		speeds := make([]float64, len(samples))
		for i, p := range samples {
			speeds[i] = p.v
		}
		medianMps := percentile(speeds, 50)
		if medianMps < groundMovingMps {
			continue
		}
		dist := make([]float64, len(samples))
		for i := 1; i < len(samples); i++ {
			dist[i] = dist[i-1] + math.Hypot(samples[i].x-samples[i-1].x, samples[i].y-samples[i-1].y)
		}
		length := dist[len(dist)-1]
		if length < groundMinLengthMetres {
			continue
		}
		first, last := samples[0], samples[len(samples)-1]
		dx, dy := last.x-first.x, last.y-first.y
		row := groundTrackRow{TrackID: id, Samples: len(samples), LengthMetres: length, MedianMps: medianMps,
			CourseDeg: bearingDeg(dx, dy), RangeStart: math.Hypot(first.x, first.y),
			RangeEnd: math.Hypot(last.x, last.y)}
		if net := math.Hypot(dx, dy); net > 0 {
			ux, uy := dx/net, dy/net
			row.GradeAlong = a*ux + b*uy
			along = append(along, math.Abs(row.GradeAlong))
			across = append(across, math.Abs(-a*uy+b*ux))
			theta := math.Atan2(dy, dx)
			sumC += length * math.Cos(2*theta)
			sumS += length * math.Sin(2*theta)
			sumW += length
		}
		z := make([]float64, len(samples))
		above := make([]float64, len(samples))
		abovePlane := make([]float64, len(samples))
		for i, p := range samples {
			z[i] = p.z
			if surface != nil {
				above[i] = p.z - surface.BlendedHeightAt(p.x, p.y)
				abovePlane[i] = p.z - surface.Global.HeightAt(p.x, p.y)
				d := 0.0
				if plane, ok := surface.RegionAt(p.x, p.y); ok {
					d = plane.HeightAt(p.x, p.y) - surface.Global.HeightAt(p.x, p.y)
					ownCell++
				}
				departures = append(departures, math.Abs(d))
				signed = append(signed, d)
			}
		}
		row.SlopeSensor, row.SlopeSensorSE = slopeFit(dist, z)
		sensorNotable := notableSlope(row.SlopeSensor, row.SlopeSensorSE, length)
		row.Class = groundClassFlat
		if surface != nil {
			row.SlopeSurface, row.SlopeSurfaceSE = slopeFit(dist, above)
			row.Class = groundClass(sensorNotable, notableSlope(row.SlopeSurface, row.SlopeSurfaceSE, length))
			row.SlopePlane, row.SlopePlaneSE = slopeFit(dist, abovePlane)
			row.PlaneClass = groundClass(sensorNotable, notableSlope(row.SlopePlane, row.SlopePlaneSE, length))
			s.PlaneClasses[row.PlaneClass]++
		} else if sensorNotable {
			// Without a surface nothing is explained.
			row.Class = groundClassBoth
		}
		s.Classes[row.Class]++
		s.MovingSamples += len(samples)
		rows = append(rows, row)
	}
	s.MovingTracks = len(rows)
	if sumW > 0 {
		axis := 0.5 * math.Atan2(sumS, sumC)
		s.Course.AxisDeg = bearingDeg(math.Cos(axis), math.Sin(axis))
		s.Course.AxisConcentration = math.Hypot(sumC, sumS) / sumW
		if s.Surface != nil && s.Surface.Gradient > 0 {
			s.Course.FallLineToAxisDeg = axialSeparationDeg(math.Atan2(-b, -a), axis)
		}
	}
	s.Course.GradeAlongMedian = percentile(along, 50)
	s.Course.GradeAcrossMedian = percentile(across, 50)
	s.Course.GradeAlongMaxTrack = percentile(along, 100)
	if n := len(departures); n > 0 {
		s.Departure = groundDeparture{Samples: n, P50Metres: percentile(departures, 50),
			P90Metres: percentile(departures, 90), MaxMetres: percentile(departures, 100),
			SignedMedian: percentile(signed, 50), OwnCellShare: float64(ownCell) / float64(n)}
	}
	sensorAbs := make([]float64, len(rows))
	surfaceAbs := make([]float64, len(rows))
	planeAbs := make([]float64, len(rows))
	for i, r := range rows {
		sensorAbs[i], surfaceAbs[i], planeAbs[i] = math.Abs(r.SlopeSensor), math.Abs(r.SlopeSurface), math.Abs(r.SlopePlane)
	}
	s.SlopeSensor = groundSlopeStats{MedianAbs: percentile(sensorAbs, 50), P90Abs: percentile(sensorAbs, 90)}
	if surface != nil {
		s.SlopeSurface = groundSlopeStats{MedianAbs: percentile(surfaceAbs, 50), P90Abs: percentile(surfaceAbs, 90)}
		s.SlopePlane = groundSlopeStats{MedianAbs: percentile(planeAbs, 50), P90Abs: percentile(planeAbs, 90)}
	}
	// A track that rises or falls above the surface is a merge, a clip
	// artefact or a kerb until read otherwise: list the largest changes.
	var listed []groundTrackRow
	for _, r := range rows {
		if r.Class == groundClassSurfaceOnly || r.Class == groundClassBoth {
			listed = append(listed, r)
		}
	}
	sort.SliceStable(listed, func(i, j int) bool {
		return math.Abs(listed[i].SlopeSurface)*listed[i].LengthMetres > math.Abs(listed[j].SlopeSurface)*listed[j].LengthMetres
	})
	for i, r := range listed {
		if i == groundListedTracks {
			break
		}
		s.ListedTrackIDs = append(s.ListedTrackIDs, r.TrackID)
	}
	return groundReport{SchemaVersion: 1, Summary: s, Tracks: rows}
}

// writeGroundReport writes the report into dir.
func writeGroundReport(dir string, report groundReport) error {
	b, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, groundReportFile), append(b, '\n'), 0o644)
}

// groundClass names a moving track's class from whether its lowest return
// rises or falls in the sensor frame and above a surface.
func groundClass(sensor, surface bool) string {
	switch {
	case sensor && surface:
		return groundClassBoth
	case sensor:
		return groundClassExplained
	case surface:
		return groundClassSurfaceOnly
	}
	return groundClassFlat
}

// notableSlope says whether a slope fitted over length metres rises or falls.
func notableSlope(slope, se, length float64) bool {
	m := math.Abs(slope)
	return m >= groundNotableSlope && m*length >= groundNotableRiseMetres && m >= groundNotableSE*se
}

// slopeFit is the least-squares slope of y against x and its standard error.
func slopeFit(x, y []float64) (slope, se float64) {
	n := float64(len(x))
	if len(x) < 3 || len(x) != len(y) {
		return 0, 0
	}
	var mx, my float64
	for i := range x {
		mx += x[i]
		my += y[i]
	}
	mx, my = mx/n, my/n
	var sxx, sxy float64
	for i := range x {
		sxx += (x[i] - mx) * (x[i] - mx)
		sxy += (x[i] - mx) * (y[i] - my)
	}
	if sxx == 0 {
		return 0, 0
	}
	slope = sxy / sxx
	var rss float64
	for i := range x {
		r := y[i] - my - slope*(x[i]-mx)
		rss += r * r
	}
	return slope, math.Sqrt(rss / (n - 2) / sxx)
}

// percentile is the p-th percentile of values by linear interpolation, 0
// for none; values is not modified.
func percentile(values []float64, p float64) float64 {
	if len(values) == 0 {
		return 0
	}
	v := append([]float64(nil), values...)
	sort.Float64s(v)
	pos := p / 100 * float64(len(v)-1)
	lo := int(math.Floor(pos))
	if lo >= len(v)-1 {
		return v[len(v)-1]
	}
	return v[lo] + (pos-float64(lo))*(v[lo+1]-v[lo])
}

// bearingDeg is the direction of (x, y) in degrees counter-clockwise from +x,
// in [0, 360).
func bearingDeg(x, y float64) float64 {
	d := math.Atan2(y, x) * 180 / math.Pi
	if d < 0 {
		d += 360
	}
	return d
}

// axialSeparationDeg is the angle between two undirected axes, in [0, 90].
func axialSeparationDeg(a, b float64) float64 {
	d := math.Mod(math.Abs(a-b), math.Pi)
	if d > math.Pi/2 {
		d = math.Pi - d
	}
	return d * 180 / math.Pi
}
