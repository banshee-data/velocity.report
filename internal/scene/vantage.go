package scene

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"regexp"
	"strings"
)

// VantagesFile is the one place a scene's vantages are written.
//
// It sits at the scene root beside manifest.json, because vantages describe a
// place rather than a run: a scene split into several parts has one set of
// viewpoints, not one per part. Nothing else stores them. The exporter does
// not copy them into a part header, the recorder does not bake them into a
// VRLOG, and the scene index does not keep a column of them, because a second
// copy is a copy that drifts and the viewer would then have to guess which one
// the publisher meant.
//
// The viewer's "Copy this view" button emits exactly one entry of this file,
// so framing an angle by eye and pasting the result is the editing loop.
const VantagesFile = "vantages.json"

// Vantage is a named viewpoint on a scene.
//
// A compass bearing is a poor label for a street: "from north" tells a reader
// nothing, while "Eastbound Howard" tells them what they are looking along.
// The geometry is stored relative to the scene's own framing rather than in
// world coordinates, so a vantage keeps working when the export is
// regenerated, the background changes, or the observed area shifts.
type Vantage struct {
	// ID is stable and URL-safe; the viewer uses it to mark the active chip.
	ID string `json:"id"`
	// Label is what a reader sees: "Eastbound Howard", "Northbound 5th".
	Label string `json:"label"`

	// AzimuthDeg is the compass bearing the camera looks *from*, where 0 is
	// north and the angle increases clockwise.
	AzimuthDeg float64 `json:"azimuth_deg"`
	// PolarDeg is the angle down from vertical: 0 is directly overhead, 90 is
	// at ground level. Clamped away from both extremes when applied.
	PolarDeg float64 `json:"polar_deg"`
	// Zoom multiplies the framing distance. 1 is the whole scene; below 1
	// moves closer.
	Zoom float64 `json:"zoom"`

	// OffsetX and OffsetY shift the look-at point across the ground in metres,
	// in the scene's own coordinate frame, so a vantage can centre on one
	// approach rather than the middle of the junction.
	OffsetX float64 `json:"offset_x,omitempty"`
	OffsetY float64 `json:"offset_y,omitempty"`

	// Fly opts a vantage into or out of the viewer's drone circuit, which
	// visits the eligible ones in bearing order. Absent means yes: a vantage
	// worth naming is usually worth flying past. A pointer so that "not
	// mentioned" and "explicitly excluded" stay distinguishable, which is what
	// lets the checker report the flight a scene will actually make.
	Fly *bool `json:"fly,omitempty"`
}

// InFlight reports whether this vantage joins the drone circuit.
func (v *Vantage) InFlight() bool { return v.Fly == nil || *v.Fly }

var vantageIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,47}$`)

// DefaultVantages are what the viewer falls back to when a scene ships no
// vantages.json. They are compass bearings because that is all that can be
// known without someone who has stood at the junction.
func DefaultVantages() []Vantage {
	return []Vantage{
		{ID: "overview", Label: "Overview", AzimuthDeg: 45, PolarDeg: 55, Zoom: 1.0},
		{ID: "north", Label: "From north", AzimuthDeg: 0, PolarDeg: 68, Zoom: 0.85},
		{ID: "east", Label: "From east", AzimuthDeg: 90, PolarDeg: 68, Zoom: 0.85},
		{ID: "south", Label: "From south", AzimuthDeg: 180, PolarDeg: 68, Zoom: 0.85},
		{ID: "west", Label: "From west", AzimuthDeg: 270, PolarDeg: 68, Zoom: 0.85},
		{ID: "top", Label: "Overhead", AzimuthDeg: 0, PolarDeg: 2, Zoom: 0.95},
	}
}

// Validate reports what is wrong with a vantage in words the person editing it
// can act on, rather than an error value that only a programmer can read.
func (v *Vantage) Validate() string {
	if !vantageIDPattern.MatchString(v.ID) {
		return fmt.Sprintf(
			"vantage id %q must be lowercase letters, digits, dashes or underscores", v.ID)
	}
	if strings.TrimSpace(v.Label) == "" {
		return fmt.Sprintf("vantage %q needs a label a reader will understand", v.ID)
	}
	if v.PolarDeg < 0 || v.PolarDeg > 90 {
		return fmt.Sprintf(
			"vantage %q: angle must be between 0 (overhead) and 90 (ground level), got %g",
			v.ID, v.PolarDeg)
	}
	if v.Zoom <= 0 || v.Zoom > 4 {
		return fmt.Sprintf("vantage %q: zoom must be between 0 and 4, got %g", v.ID, v.Zoom)
	}
	if math.Abs(v.OffsetX) > 500 || math.Abs(v.OffsetY) > 500 {
		return fmt.Sprintf("vantage %q: offset must be within 500 m of the scene centre", v.ID)
	}
	return ""
}

// Normalise wraps the bearing into [0, 360) and fills a missing zoom, so a
// hand-edited value like -90 or 450 behaves as the person meant.
func (v *Vantage) Normalise() {
	v.AzimuthDeg = math.Mod(math.Mod(v.AzimuthDeg, 360)+360, 360)
	if v.Zoom == 0 {
		v.Zoom = 1
	}
	v.ID = strings.TrimSpace(v.ID)
	v.Label = strings.TrimSpace(v.Label)
}

// ValidateVantages checks a whole set, including that the ids are distinct.
// A duplicate id would make one chip unreachable.
func ValidateVantages(vs []Vantage) string {
	seen := map[string]bool{}
	for i := range vs {
		vs[i].Normalise()
		if problem := vs[i].Validate(); problem != "" {
			return problem
		}
		if seen[vs[i].ID] {
			return fmt.Sprintf("vantage id %q is used more than once", vs[i].ID)
		}
		seen[vs[i].ID] = true
	}
	return ""
}

// LoadVantages reads a vantage list from a JSON file, accepting either a bare
// array or an object with a "vantages" key, so the same file can be a whole
// scene sidecar or just the list.
func LoadVantages(path string) ([]Vantage, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read vantages: %w", err)
	}

	var list []Vantage
	if err := json.Unmarshal(raw, &list); err != nil {
		var wrapper struct {
			Vantages []Vantage `json:"vantages"`
		}
		if err2 := json.Unmarshal(raw, &wrapper); err2 != nil {
			return nil, fmt.Errorf("%s is neither a vantage array nor an object with a vantages key: %w", path, err)
		}
		list = wrapper.Vantages
	}
	// A file that was explicitly named but yields nothing is almost always a
	// mistyped key. Falling back to compass defaults there would publish the
	// wrong labels and say nothing about why.
	if len(list) == 0 {
		return nil, fmt.Errorf("%s contains no vantages; expected an array or a \"vantages\" key", path)
	}
	if problem := ValidateVantages(list); problem != "" {
		return nil, fmt.Errorf("%s: %s", path, problem)
	}
	return list, nil
}
