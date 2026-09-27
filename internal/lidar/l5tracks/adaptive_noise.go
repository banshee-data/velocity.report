package l5tracks

// Adaptive, anisotropic measurement noise: state-estimation plan Phase 3 and
// Section 8.1, behind TrackerConfig.AdaptiveMeasurementNoise.
//
// The shipped filter gives every position measurement the same isotropic
// variance, MeasurementNoise, at every range, support and viewing angle. This
// model computes R where its error structure is physical and diagonal — along
// the sensor's line of sight to the measurement (radial) and across it
// (tangential) — and rotates it into the site frame, which supplies the
// off-diagonal terms. Each axis's variance is the sum of two parts:
//
//	sigma_axis² = c_axis(stratum) + phi_axis(range, support, aspect)
//
// phi is the sensor physics Section 8.1 names and the observation carries
// today: range accuracy and the grazing-incidence spread along the line of
// sight, azimuth quantisation and its averaging across it. It is small: at
// 30 m the whole tangential term is under 0.02 m². The edge-localisation term
// needs EdgeMeasurement.Truncated, which the online measurement does not have,
// so it is absent rather than guessed.
//
// c is everything phi does not explain — clustering jitter, visible-surface
// hops, a medoid moving between faces — as a coefficient per stratum: the
// measurement source, a range bin, a support bin and a folded aspect bin. It is
// the part the calibration fits (l8analytics.FitUncertaintyCalibration). With
// no calibration every stratum's coefficient is the shipped MeasurementNoise,
// so the uncalibrated model is the shipped variance plus the physics terms.
//
// The plan's constraint governs how c may be fitted: a systematic bias must be
// corrected by the observation model, not inflated into R. The fit therefore
// estimates each coefficient from the innovation's variance about its own mean,
// and reports a stratum whose mean is a large share of its spread as bias
// dominated rather than widening R to cover it.

import (
	"errors"
	"fmt"
	"math"
)

const (
	// PandarRangeSigmaMetres is the Pandar40P's specified range accuracy
	// (plan Section 8.1, sigma_range).
	PandarRangeSigmaMetres = 0.02
	// PandarAzimuthStepRad is the Pandar40P's azimuth step at 10 Hz: 0.2
	// degrees, the 1,800 bins per rotation the background grid also uses.
	PandarAzimuthStepRad = 0.2 * math.Pi / 180
	// minNoiseGeometryRangeMetres is the range below which the line of sight
	// is undefined. A measurement there gets the shipped isotropic variance.
	minNoiseGeometryRangeMetres = 0.5
)

// Noise model strata. Changing any edge changes which coefficient a
// measurement reads, so a calibration records the edges it was fitted with
// and is refused by a build whose edges differ.
var (
	// NoiseRangeEdgesMetres are the lower edges of the range bins. The last
	// bin is open-ended.
	NoiseRangeEdgesMetres = [...]float32{0, 10, 20, 30, 50}
	// NoiseSupportEdges are the lower edges of the cluster point-count bins.
	NoiseSupportEdges = [...]int{0, 10, 30, 100}
)

const (
	// NoiseRangeBins, NoiseSupportBins and NoiseAspectBins size the stratum
	// table.
	NoiseRangeBins   = len(NoiseRangeEdgesMetres)
	NoiseSupportBins = len(NoiseSupportEdges)
	// NoiseAspectBins splits the folded aspect, [0, 90] degrees, into end-on
	// [0, 30), oblique [30, 60) and broadside [60, 90].
	NoiseAspectBins = 3
	// NoiseAxisRadial and NoiseAxisTangential index the two sensor-frame axes.
	NoiseAxisRadial     = 0
	NoiseAxisTangential = 1
	// NoiseAxisCount is the number of sensor-frame axes.
	NoiseAxisCount = 2
)

// NoiseSources are the measurement sources the table has a row for, in index
// order. The near-edge candidate is offline only and has no online row.
var NoiseSources = [...]MeasurementSource{MeasurementMedoidV0, MeasurementOBBCentreV1, MeasurementMedoidFallbackV1}

// NoiseSourceCount is the number of sources the stratum table covers.
const NoiseSourceCount = len(NoiseSources)

// NoiseAxisName names an axis for reports.
func NoiseAxisName(axis int) string {
	if axis == NoiseAxisRadial {
		return "radial"
	}
	return "tangential"
}

// NoiseSourceIndex returns the table row for a measurement source.
func NoiseSourceIndex(source MeasurementSource) (int, bool) {
	for i, s := range NoiseSources {
		if s == source {
			return i, true
		}
	}
	return 0, false
}

// NoiseStratum addresses one cell of the coefficient table.
type NoiseStratum struct {
	Source, Range, Support, Aspect int
}

// MeasurementGeometry is the viewing geometry the noise model conditions on,
// for one measurement seen from the sensor under one track's heading.
type MeasurementGeometry struct {
	// Valid is false when the measurement is too close to the sensor for a
	// line of sight to exist.
	Valid bool
	// RangeMetres is the horizontal distance from the sensor to the
	// measurement.
	RangeMetres float32
	// RadialX, RadialY is the unit line of sight, sensor to measurement. The
	// tangential axis is its left-hand perpendicular, (-RadialY, RadialX).
	RadialX, RadialY float32
	// Support is the cluster's point count.
	Support int
	// AspectRad is the sensor's bearing as seen from the body, in the body
	// frame, in [0, 2π): 0 means the sensor lies straight ahead of the track's
	// heading, π/2 on its left. Read it with the heading's own reliability: a
	// slow track's heading may be the PCA axis rather than its course.
	AspectRad float32
}

// FoldedAspectRad folds AspectRad onto [0, π/2]: 0 is end-on (a front or rear
// face towards the sensor), π/2 broadside. A box is symmetric under both
// reflections, so this is what the incidence term and the aspect bins read.
func (g MeasurementGeometry) FoldedAspectRad() float32 {
	a := math.Mod(float64(g.AspectRad), math.Pi)
	if a < 0 {
		a += math.Pi
	}
	if a > math.Pi/2 {
		a = math.Pi - a
	}
	return float32(a)
}

// AspectOctant is the aspect's 45-degree octant, 0 to 7, the unit G-UNC-1's
// aspect check stratifies by.
func (g MeasurementGeometry) AspectOctant() int {
	o := int(float64(g.AspectRad) / (math.Pi / 4))
	if o < 0 {
		return 0
	}
	if o > 7 {
		return 7
	}
	return o
}

// MeasurementGeometryFor derives the viewing geometry of a measurement at
// (x, y) from a sensor at (sensorX, sensorY), for a body whose believed heading
// is headingRad.
func MeasurementGeometryFor(x, y, sensorX, sensorY, headingRad float32, support int) MeasurementGeometry {
	dx, dy := float64(x-sensorX), float64(y-sensorY)
	r := math.Hypot(dx, dy)
	g := MeasurementGeometry{RangeMetres: float32(r), Support: support}
	if !(r >= minNoiseGeometryRangeMetres) || math.IsInf(r, 0) || !finiteMeasurementCoordinate(headingRad) {
		return g
	}
	g.Valid = true
	g.RadialX, g.RadialY = float32(dx/r), float32(dy/r)
	// The sensor seen from the body is the reverse of the line of sight.
	bearing := math.Atan2(-dy, -dx) - float64(headingRad)
	bearing = math.Mod(bearing, 2*math.Pi)
	if bearing < 0 {
		bearing += 2 * math.Pi
	}
	if bearing >= 2*math.Pi {
		bearing = 0
	}
	g.AspectRad = float32(bearing)
	return g
}

// NoiseStratumFor places a measurement in the coefficient table. It reports
// false for a source the table has no row for or an invalid geometry.
func NoiseStratumFor(source MeasurementSource, g MeasurementGeometry) (NoiseStratum, bool) {
	si, ok := NoiseSourceIndex(source)
	if !ok || !g.Valid {
		return NoiseStratum{}, false
	}
	return NoiseStratum{
		Source:  si,
		Range:   noiseRangeBin(g.RangeMetres),
		Support: noiseSupportBin(g.Support),
		Aspect:  noiseAspectBin(g.FoldedAspectRad()),
	}, true
}

func noiseRangeBin(r float32) int {
	bin := 0
	for i, edge := range NoiseRangeEdgesMetres {
		if r >= edge {
			bin = i
		}
	}
	return bin
}

func noiseSupportBin(n int) int {
	bin := 0
	for i, edge := range NoiseSupportEdges {
		if n >= edge {
			bin = i
		}
	}
	return bin
}

func noiseAspectBin(folded float32) int {
	bin := int(float64(folded) / (math.Pi / 2 / NoiseAspectBins))
	if bin < 0 {
		return 0
	}
	if bin >= NoiseAspectBins {
		return NoiseAspectBins - 1
	}
	return bin
}

// SensorPhysicsNoise returns the Section 8.1 variance terms, in square metres,
// along and across the line of sight:
//
//	radial     = sigma_range² + (r·delta_az · tan(theta_incidence))²
//	tangential = (r·delta_az)²/12 + (r·delta_az)²/N
//
// theta_incidence is the angle between the line of sight and the normal of the
// most nearly face-on visible face, min(aspect, 90° - aspect) on the folded
// aspect, so the grazing term peaks at 45 degrees and vanishes end-on and
// broadside. The beam footprint is taken as one azimuth step. N is the whole
// cluster's point count: the online measurement does not know which returns
// lie on the face, which the plan's N_eff asks for, so the averaging gain is
// if anything overstated. An invalid geometry returns zero for both.
func SensorPhysicsNoise(g MeasurementGeometry) (radial, tangential float32) {
	if !g.Valid {
		return 0, 0
	}
	footprint := float64(g.RangeMetres) * PandarAzimuthStepRad
	a := float64(g.FoldedAspectRad())
	incidence := math.Min(a, math.Pi/2-a)
	grazing := footprint * math.Tan(incidence)
	radial = float32(PandarRangeSigmaMetres*PandarRangeSigmaMetres + grazing*grazing)
	n := float64(g.Support)
	if n < 1 {
		n = 1
	}
	tangential = float32(footprint*footprint/12 + footprint*footprint/n)
	return radial, tangential
}

// NoiseCalibration is a fitted coefficient table: for every stratum and axis,
// the variance the adaptive model adds to its physics terms. It is produced by
// l8analytics.FitUncertaintyCalibration from a replay's pre-gate residuals, and
// carried by pointer in TrackerConfig, so it must not be mutated once a tracker
// holds it.
type NoiseCalibration struct {
	// ID names the calibration: a digest of its coefficients, recorded with a
	// replay that used it so the estimate can say which table produced it.
	ID string
	// Coefficients is indexed [source][range][support][aspect][axis], in
	// square metres.
	Coefficients [NoiseSourceCount][NoiseRangeBins][NoiseSupportBins][NoiseAspectBins][NoiseAxisCount]float32
}

// Coefficient returns one stratum's coefficient for an axis.
func (c *NoiseCalibration) Coefficient(s NoiseStratum, axis int) float32 {
	return c.Coefficients[s.Source][s.Range][s.Support][s.Aspect][axis]
}

// Validate refuses a table the filter could not use: every coefficient must
// be finite and positive, or R could stop being positive definite.
func (c *NoiseCalibration) Validate() error {
	if c == nil {
		return errors.New("noise calibration is nil")
	}
	for si := range c.Coefficients {
		for ri := range c.Coefficients[si] {
			for ni := range c.Coefficients[si][ri] {
				for ai := range c.Coefficients[si][ri][ni] {
					for axis, v := range c.Coefficients[si][ri][ni][ai] {
						if !(v > 0) || math.IsInf(float64(v), 0) {
							return fmt.Errorf("noise calibration %s range %d support %d aspect %d %s: coefficient %v is not finite and positive",
								NoiseSources[si], ri, ni, ai, NoiseAxisName(axis), v)
						}
					}
				}
			}
		}
	}
	return nil
}

// UniformNoiseCalibration returns a table with the same coefficient in every
// cell: the uncalibrated model's table, made explicit.
func UniformNoiseCalibration(coefficient float32) *NoiseCalibration {
	c := &NoiseCalibration{ID: "uniform"}
	for si := range c.Coefficients {
		for ri := range c.Coefficients[si] {
			for ni := range c.Coefficients[si][ri] {
				for ai := range c.Coefficients[si][ri][ni] {
					for axis := range c.Coefficients[si][ri][ni][ai] {
						c.Coefficients[si][ri][ni][ai][axis] = coefficient
					}
				}
			}
		}
	}
	return c
}

// NoiseEvaluation is the adaptive model's reading of one measurement for one
// track: the geometry, the stratum, both parts of each axis's variance, and R
// rotated into the site frame. It is recorded with every calibration sample so
// the fit can separate the physics terms from the coefficient.
type NoiseEvaluation struct {
	Geometry        MeasurementGeometry
	Stratum         NoiseStratum
	StratumValid    bool
	PhysRadial      float32
	PhysTangential  float32
	CoefRadial      float32
	CoefTangential  float32
	Radial          float32
	Tangential      float32
	SiteCovariance  MeasurementCovariance
	FallbackReason  string
	CalibrationUsed bool
}

// evaluateNoise applies the adaptive model to one measurement for one track.
// It reads only the configuration, the measurement, the cluster's support and
// the track's believed heading, so the gate, the likelihood cost and the
// update all see the same R for the same pairing.
func (t *Tracker) evaluateNoise(track *TrackedObject, cluster WorldCluster, measurement PositionMeasurement) NoiseEvaluation {
	base := t.Config.MeasurementNoise
	g := MeasurementGeometryFor(measurement.X, measurement.Y, t.Config.NoiseSensorX, t.Config.NoiseSensorY,
		track.OBBHeadingRad, cluster.PointsCount)
	ev := NoiseEvaluation{Geometry: g, CoefRadial: base, CoefTangential: base}
	if !g.Valid {
		// No line of sight: the shipped isotropic variance, stated as such.
		ev.FallbackReason = "no_line_of_sight"
		ev.Radial, ev.Tangential = base, base
		ev.SiteCovariance = MeasurementCovariance{XX: base, YY: base}
		return ev
	}
	ev.PhysRadial, ev.PhysTangential = SensorPhysicsNoise(g)
	ev.Stratum, ev.StratumValid = NoiseStratumFor(measurement.Source, g)
	if cal := t.Config.MeasurementNoiseCalibration; cal != nil && ev.StratumValid {
		ev.CoefRadial = cal.Coefficient(ev.Stratum, NoiseAxisRadial)
		ev.CoefTangential = cal.Coefficient(ev.Stratum, NoiseAxisTangential)
		ev.CalibrationUsed = true
	} else if cal != nil {
		ev.FallbackReason = "source_without_calibration_row"
	}
	ev.Radial = ev.CoefRadial + ev.PhysRadial
	ev.Tangential = ev.CoefTangential + ev.PhysTangential
	ev.SiteCovariance = rotateSensorNoise(ev.Radial, ev.Tangential, g.RadialX, g.RadialY)
	return ev
}

// rotateSensorNoise turns a diagonal sensor-frame covariance into the site
// frame: R = radial·u·uᵀ + tangential·v·vᵀ, with u the unit line of sight and
// v = (-u_y, u_x).
func rotateSensorNoise(radial, tangential, ux, uy float32) MeasurementCovariance {
	return MeasurementCovariance{
		XX: radial*ux*ux + tangential*uy*uy,
		XY: (radial - tangential) * ux * uy,
		YY: radial*uy*uy + tangential*ux*ux,
	}
}

// adaptiveInnovationCovariance is S = HPHᵀ + R under the adaptive model. The
// shipped isotropic S stays written out at each call site so that its
// arithmetic, and so every default output, is untouched.
func adaptiveInnovationCovariance(p *[16]float32, r MeasurementCovariance) (s00, s01, s10, s11 float32) {
	return p[0*4+0] + r.XX, p[0*4+1] + r.XY, p[1*4+0] + r.XY, p[1*4+1] + r.YY
}
