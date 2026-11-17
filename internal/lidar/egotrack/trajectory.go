// Package egotrack derives the moving sensor platform's own motion (speed,
// acceleration, jerk, yaw rate, stops and vibration) from a trajectory file
// produced by offline odometry. It is the velocity.report side of the route
// SLAM spike: Gerardus (or any odometry tool) writes the trajectory, this
// package reads it, states the uncertainty of every derived quantity, and
// checks the trajectory against independently known poses at its ends.
//
// Nothing here estimates a pose. The package differentiates poses it is given
// and propagates their stated noise; see docs/plans/lidar-route-slam-minimum-plan.md.
package egotrack

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"sort"
)

// SchemaV0 names the provisional trajectory format. It is a subset of the
// route plan's trajectory contract and is expected to be replaced by a proto
// message once Gerardus fixes its output; readers refuse any other value.
const SchemaV0 = "velocity.report/ego-trajectory/v0"

// Trajectory is one capture's pose sequence in a single right-handed frame
// whose z axis is up (a local frame anchored at the first pose, or a projected
// CRS such as EPSG:7131). Positions are metres; times are seconds on the clock
// named by TimeScale.
type Trajectory struct {
	Schema string `json:"schema"`
	// Frame names the frame the poses are in, e.g. "local" or "EPSG:7131".
	Frame string `json:"frame"`
	// TimeScale names what T counts, e.g. "session_seconds" (seconds since
	// EpochUTC on the export's clock). It is never assumed to be GPS time.
	TimeScale string `json:"time_scale"`
	EpochUTC  string `json:"epoch_utc,omitempty"`
	// Source records the tool, version and parameters that produced the poses.
	Source  map[string]string `json:"source,omitempty"`
	Samples []Sample          `json:"samples"`
}

// Sample is one pose. Q is a unit quaternion (w, x, y, z) taking body
// coordinates into the trajectory frame.
type Sample struct {
	Frame   int        `json:"frame"`
	T       float64    `json:"t_s"`
	P       [3]float64 `json:"p_m"`
	Q       [4]float64 `json:"q_wxyz"`
	Quality *Quality   `json:"quality,omitempty"`
}

// Quality is the odometry's own report on a pose.
type Quality struct {
	// SigmaPosM is the 1σ position noise per axis the odometry assigns to this
	// pose relative to its neighbours. Zero means not stated.
	SigmaPosM      float64 `json:"sigma_pos_m,omitempty"`
	Residual       float64 `json:"residual_m,omitempty"`
	InlierFraction float64 `json:"inlier_fraction,omitempty"`
}

// Read decodes and validates a trajectory.
func Read(r io.Reader) (*Trajectory, error) {
	var t Trajectory
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return nil, fmt.Errorf("decode trajectory: %w", err)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return &t, nil
}

// Validate checks the schema, finiteness, strictly increasing time and unit
// quaternions (normalising those within 1e-3 of unit length).
func (t *Trajectory) Validate() error {
	if t.Schema != SchemaV0 {
		return fmt.Errorf("trajectory schema %q, want %q", t.Schema, SchemaV0)
	}
	if t.TimeScale == "" {
		return errors.New("trajectory time_scale is required")
	}
	if len(t.Samples) < 2 {
		return errors.New("trajectory needs at least two samples")
	}
	for i := range t.Samples {
		s := &t.Samples[i]
		for _, v := range append(append([]float64{s.T}, s.P[:]...), s.Q[:]...) {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				return fmt.Errorf("sample %d: non-finite value", i)
			}
		}
		if i > 0 && !(s.T > t.Samples[i-1].T) {
			return fmt.Errorf("sample %d: time %.9f not after %.9f", i, s.T, t.Samples[i-1].T)
		}
		n := math.Sqrt(s.Q[0]*s.Q[0] + s.Q[1]*s.Q[1] + s.Q[2]*s.Q[2] + s.Q[3]*s.Q[3])
		if math.Abs(n-1) > 1e-3 {
			return fmt.Errorf("sample %d: quaternion norm %.6f is not unit", i, n)
		}
		for k := range s.Q {
			s.Q[k] /= n
		}
	}
	return nil
}

// MedianInterval is the median spacing between consecutive samples.
func (t *Trajectory) MedianInterval() float64 {
	d := make([]float64, 0, len(t.Samples)-1)
	for i := 1; i < len(t.Samples); i++ {
		d = append(d, t.Samples[i].T-t.Samples[i-1].T)
	}
	sort.Float64s(d)
	return d[len(d)/2]
}

// segments splits sample indices where a gap exceeds gapFactor times the
// median interval, so no fit or spectrum straddles missing poses.
func (t *Trajectory) segments(gapFactor float64) [][2]int {
	limit := gapFactor * t.MedianInterval()
	var out [][2]int
	start := 0
	for i := 1; i < len(t.Samples); i++ {
		if t.Samples[i].T-t.Samples[i-1].T > limit {
			out = append(out, [2]int{start, i})
			start = i
		}
	}
	return append(out, [2]int{start, len(t.Samples)})
}

// PoseAt interpolates the pose at time tq: position linearly, orientation by
// spherical linear interpolation. It refuses times outside the trajectory or
// inside a gap longer than maxGap seconds; that refusal is the file-backed
// pose provider's contract for invalid intervals.
func (t *Trajectory) PoseAt(tq, maxGap float64) (p [3]float64, q [4]float64, err error) {
	s := t.Samples
	switch {
	case len(s) == 0:
		return p, q, errors.New("empty trajectory")
	case math.IsNaN(tq):
		return p, q, errors.New("query time is NaN")
	case math.IsNaN(maxGap) || maxGap < 0:
		// A NaN would make every gap comparison false and interpolate across
		// any gap, losing the refusal this lookup exists to give.
		return p, q, fmt.Errorf("max gap %v must be a non-negative number", maxGap)
	}
	if tq < s[0].T || tq > s[len(s)-1].T {
		return p, q, fmt.Errorf("time %.6f outside trajectory [%.6f, %.6f]", tq, s[0].T, s[len(s)-1].T)
	}
	j := sort.Search(len(s), func(i int) bool { return s[i].T >= tq })
	if s[j].T == tq {
		return s[j].P, s[j].Q, nil
	}
	a, b := s[j-1], s[j]
	// The same microsecond of tolerance as the fit window: a gap equal to
	// maxGap must not be refused because two timestamps' difference rounds up.
	if b.T-a.T > maxGap+windowEps {
		return p, q, fmt.Errorf("time %.6f falls in a %.3f s gap", tq, b.T-a.T)
	}
	u := (tq - a.T) / (b.T - a.T)
	for k := 0; k < 3; k++ {
		p[k] = a.P[k] + u*(b.P[k]-a.P[k])
	}
	return p, slerp(a.Q, b.Q, u), nil
}

func slerp(a, b [4]float64, u float64) [4]float64 {
	dot := a[0]*b[0] + a[1]*b[1] + a[2]*b[2] + a[3]*b[3]
	if dot < 0 {
		for k := range b {
			b[k] = -b[k]
		}
		dot = -dot
	}
	var out [4]float64
	if dot > 0.9995 {
		for k := range out {
			out[k] = a[k] + u*(b[k]-a[k])
		}
	} else {
		th := math.Acos(dot)
		sa, sb := math.Sin((1-u)*th)/math.Sin(th), math.Sin(u*th)/math.Sin(th)
		for k := range out {
			out[k] = sa*a[k] + sb*b[k]
		}
	}
	n := math.Sqrt(out[0]*out[0] + out[1]*out[1] + out[2]*out[2] + out[3]*out[3])
	for k := range out {
		out[k] /= n
	}
	return out
}

// EulerZYX returns yaw, pitch and roll (radians) of q: rotation about the
// frame's z, then the body's y, then the body's x.
func EulerZYX(q [4]float64) (yaw, pitch, roll float64) {
	w, x, y, z := q[0], q[1], q[2], q[3]
	yaw = math.Atan2(2*(w*z+x*y), 1-2*(y*y+z*z))
	sp := 2 * (w*y - z*x)
	sp = math.Max(-1, math.Min(1, sp))
	pitch = math.Asin(sp)
	roll = math.Atan2(2*(w*x+y*z), 1-2*(x*x+y*y))
	return
}
