package replayeval

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strconv"
	"strings"

	"github.com/banshee-data/velocity.report/internal/lidar/l5tracks"
)

// ContinuityCoverage declares where the sensor can observe, for the
// continuity experiments that classify an absence (coast_support,
// class_coast_bounds and occlusion_continuity). Without it they are refused:
// l5tracks.SensorCoverage's zero value covers everything, so a road user
// leaving the field of view would read as missed or occluded, and the
// class-bounded coast would be charged for the wrong reason.
//
// A declaration is a claim about one sensor at one site, so it names its
// source, and it must bound range: a sensor that sees everywhere is exactly
// the claim the refusal exists to stop. Coordinates are the tracker's world
// frame, which is the sensor frame while the pipeline runs without a pose.
type ContinuityCoverage struct {
	// Source says how the coverage was established: a site survey, a
	// measured detection envelope, a synthetic scene's own bounds. Required.
	Source string `json:"source"`
	// SensorXMetres and SensorYMetres are the sensor origin.
	SensorXMetres float32 `json:"sensor_x_m"`
	SensorYMetres float32 `json:"sensor_y_m"`
	// MinRangeMetres and MaxRangeMetres bound the horizontal range.
	// MaxRangeMetres is required.
	MinRangeMetres float32 `json:"min_range_m"`
	MaxRangeMetres float32 `json:"max_range_m"`
	// AzimuthCentreDeg and AzimuthHalfWidthDeg describe the sector,
	// anticlockwise from +X. A half-width of 180 is the full circle and must
	// be written as such.
	AzimuthCentreDeg    float32 `json:"azimuth_centre_deg"`
	AzimuthHalfWidthDeg float32 `json:"azimuth_half_width_deg"`
}

// Validate reports the first way the declaration fails to be one.
func (c ContinuityCoverage) Validate() error {
	if strings.TrimSpace(c.Source) == "" {
		return errors.New("continuity coverage has no source: say how it was established")
	}
	for name, v := range map[string]float32{
		"sensor_x_m": c.SensorXMetres, "sensor_y_m": c.SensorYMetres, "min_range_m": c.MinRangeMetres,
		"max_range_m": c.MaxRangeMetres, "azimuth_centre_deg": c.AzimuthCentreDeg,
		"azimuth_half_width_deg": c.AzimuthHalfWidthDeg,
	} {
		if math.IsNaN(float64(v)) || math.IsInf(float64(v), 0) {
			return fmt.Errorf("continuity coverage %s is not finite", name)
		}
	}
	if c.MaxRangeMetres <= 0 {
		return errors.New("continuity coverage max_range_m must be positive: an unbounded range is the claim that a road user can never leave coverage")
	}
	if c.MinRangeMetres < 0 || c.MinRangeMetres >= c.MaxRangeMetres {
		return fmt.Errorf("continuity coverage min_range_m %g must be at least 0 and below max_range_m %g", c.MinRangeMetres, c.MaxRangeMetres)
	}
	if c.AzimuthHalfWidthDeg <= 0 || c.AzimuthHalfWidthDeg > 180 {
		return fmt.Errorf("continuity coverage azimuth_half_width_deg %g must be in (0, 180]; write 180 for the full circle", c.AzimuthHalfWidthDeg)
	}
	if c.AzimuthCentreDeg < -360 || c.AzimuthCentreDeg > 360 {
		return fmt.Errorf("continuity coverage azimuth_centre_deg %g must be within ±360", c.AzimuthCentreDeg)
	}
	return nil
}

// ID is the declaration's content address: a hash of every field in a fixed
// order, each number in its shortest exact form. It has no failure path, so
// even a declaration Validate refuses, with a NaN or infinite bound, has an
// id of its own rather than sharing one with another.
func (c ContinuityCoverage) ID() string {
	number := func(v float32) string { return strconv.FormatFloat(float64(v), 'g', -1, 32) }
	canonical := strings.Join([]string{
		"source=" + strconv.Quote(c.Source),
		"sensor_x_m=" + number(c.SensorXMetres), "sensor_y_m=" + number(c.SensorYMetres),
		"min_range_m=" + number(c.MinRangeMetres), "max_range_m=" + number(c.MaxRangeMetres),
		"azimuth_centre_deg=" + number(c.AzimuthCentreDeg), "azimuth_half_width_deg=" + number(c.AzimuthHalfWidthDeg),
	}, "\n")
	sum := sha256.Sum256([]byte(canonical))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// LoadContinuityCoverage reads and validates a declaration. Unknown fields
// are refused, so a misspelt bound cannot silently become zero.
func LoadContinuityCoverage(path string) (*ContinuityCoverage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read continuity coverage: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var c ContinuityCoverage
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse continuity coverage %s: %w", path, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("parse continuity coverage %s: trailing data after the declaration", path)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return &c, nil
}

// apply sets the tracker's sensor origin and coverage from the declaration.
func (c ContinuityCoverage) apply(oc *l5tracks.OcclusionContinuityConfig) {
	oc.SensorX, oc.SensorY = c.SensorXMetres, c.SensorYMetres
	oc.Coverage = l5tracks.SensorCoverage{
		MinRangeMetres: c.MinRangeMetres, MaxRangeMetres: c.MaxRangeMetres,
		AzimuthCentreDeg: c.AzimuthCentreDeg, AzimuthHalfWidthDeg: c.AzimuthHalfWidthDeg,
	}
}

// coverageHashSuffix folds an applied declaration into the parameter hash:
// it changes how absences are classified and so how long a track may
// coast. An unapplied declaration changes nothing and hashes as nothing.
func coverageHashSuffix(c *ContinuityCoverage, applied bool) []byte {
	if c == nil || !applied {
		return nil
	}
	return []byte("\ncontinuity_coverage:" + c.ID())
}

// LoadContinuityCoverageSet reads per-case declarations for a corpus run,
// a JSON object from case ID to declaration. Each is validated; a case with
// no entry has no coverage, and its absence-classifying experiments are
// refused when it runs.
func LoadContinuityCoverageSet(path string) (map[string]ContinuityCoverage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read continuity coverage set: %w", err)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var set map[string]ContinuityCoverage
	if err := dec.Decode(&set); err != nil {
		return nil, fmt.Errorf("parse continuity coverage set %s: %w", path, err)
	}
	if dec.More() {
		return nil, fmt.Errorf("parse continuity coverage set %s: trailing data after the set", path)
	}
	if len(set) == 0 {
		return nil, fmt.Errorf("continuity coverage set %s declares no case", path)
	}
	for id, c := range set {
		if err := c.Validate(); err != nil {
			return nil, fmt.Errorf("%s: case %s: %w", path, id, err)
		}
	}
	return set, nil
}
