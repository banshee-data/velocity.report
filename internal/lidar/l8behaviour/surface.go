package l8behaviour

// The registry check for external surfaces, per Section 10.4 of
// docs/plans/lidar-behaviour-analytics-plan.md ("canonical registry names
// across storage, API and report") and the surface completeness check of
// docs/platform/architecture/metrics-registry.md.
//
// A behaviour surface (a persisted payload, an API response, a report's data
// file) may use three kinds of name: a registered metric id, a registered
// suppression reason, and a structural field from surfaceFieldNames. Anything
// else is a name some surface invented, which is exactly how "thw" in one
// place and "time_headway" in another would drift apart. The metric table in
// metrics.go stays the single source of truth for ids; this file only refuses
// names that are not in it.
//
// surfaceFieldNames is deliberately one shared list rather than one per
// surface: a field called duration_nanos in storage should not be
// window_seconds in the API. A surface that needs a new structural name adds
// it here, where review sees it, and a test fails if the name is a metric
// alias.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
)

// surfaceFieldNames are the structural names a behaviour surface may use.
var surfaceFieldNames = []string{
	// Interaction identity and version.
	"schema", "event_id", "source_id", "interaction_type", "primary_track_id", "secondary_track_id",
	"start_unix_nanos", "end_unix_nanos", "version", "estimate_stage", "estimator_id", "obs_model_id",
	"method_id", "geometry_id", "param_hash", "worst_support",
	// Input provenance.
	"input", "contributing_track_ids", "first_unix_nanos", "last_unix_nanos", "observed_frames",
	"coasted_frames", "planar_fallback", "provenance",
	// Measurement, uncertainty and benchmark.
	"measurements", "provisional", "name", "unit", "value", "uncertainty", "suppressed", "reason",
	"benchmark", "percentile", "opportunity_seconds", "kind", "sigma", "lower", "upper", "coverage",
	"method", "samples", "scopes", "track_local", "shared_sensor", "shared_geometry", "threshold",
	"citation", "jurisdiction", "effective_from_unix_nanos", "stratification",
	// Accounting.
	"accounting", "instants", "valid_nanos", "band_nanos", "predicted_only_nanos", "record_gap_nanos",
	"suppressions", "nanos",
	// Encounter instants and endpoints.
	"capture_unix_nanos", "interval_nanos", "record_gap", "basis", "role", "follower_support",
	"leader_support", "valid", "condition", "reasons", "leader_trailing", "follower_leading", "values",
	"coast_age_nanos", "track_id", "extremity", "arc_m", "sigma_m", "source", "support",
	"extent_converged", "length_provenance", "width_provenance",
	// Exposure windows.
	"window_id", "counterpart_track_id", "duration_nanos",
	// Distribution (distribution.go).
	"events", "exposure_events", "accounted_nanos", "histograms", "bins", "sigma_inside_nanos",
	"sigma_overlap_nanos", "excluded", "bands", "rate",
}

// SurfaceFieldNames returns the structural names a behaviour surface may use,
// sorted.
func SurfaceFieldNames() []string {
	out := append([]string(nil), surfaceFieldNames...)
	sort.Strings(out)
	return out
}

// AuditSurfaceJSON checks every name in a behaviour surface's JSON document.
// Each object key must be a registered metric id, a registered suppression
// reason, or a structural name from SurfaceFieldNames; no key may be a metric
// alias; and no string value may be an alias or claim a registered metric
// level ("interaction.") without being a registered id. It reports every
// violation with its path, not only the first.
//
// A new surface is audited by encoding what it serves and passing it here:
// the persisted records in this package's tests, an API response or a report
// data file in the tests of the package that produces it.
func AuditSurfaceJSON(doc []byte) error {
	dec := json.NewDecoder(bytes.NewReader(doc))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return fmt.Errorf("surface is not JSON: %w", err)
	}
	// Decode reads one value; anything after it would go unaudited.
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("surface is not a single JSON document: data follows the first value")
	}
	a := newSurfaceAuditor()
	a.walk("$", v)
	if len(a.problems) == 0 {
		return nil
	}
	sort.Strings(a.problems)
	return errors.New("surface uses unregistered names:\n  " + strings.Join(a.problems, "\n  "))
}

type surfaceAuditor struct {
	structural map[string]bool
	metrics    map[string]bool
	reasons    map[string]bool
	aliases    map[string]MetricID
	levels     []string
	problems   []string
}

func newSurfaceAuditor() *surfaceAuditor {
	a := &surfaceAuditor{
		structural: map[string]bool{}, metrics: map[string]bool{}, reasons: map[string]bool{},
		aliases: map[string]MetricID{},
	}
	for _, n := range surfaceFieldNames {
		a.structural[n] = true
	}
	levels := map[string]bool{}
	for _, d := range FollowingMetrics() {
		a.metrics[string(d.ID)] = true
		levels[d.Level+"."] = true
		for _, alias := range d.Aliases {
			a.aliases[alias] = d.ID
		}
	}
	for l := range levels {
		a.levels = append(a.levels, l)
	}
	for _, r := range SuppressionReasons() {
		a.reasons[r.String()] = true
	}
	return a
}

func (a *surfaceAuditor) walk(path string, v any) {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			a.checkKey(path, k)
			a.walk(path+"."+k, child)
		}
	case []any:
		for i, child := range x {
			a.walk(fmt.Sprintf("%s[%d]", path, i), child)
		}
	case string:
		a.checkString(path, x)
	}
}

func (a *surfaceAuditor) checkKey(path, k string) {
	if id, ok := a.aliases[k]; ok {
		a.problems = append(a.problems, fmt.Sprintf("%s: key %q is an alias of %s", path, k, id))
		return
	}
	if a.metrics[k] || a.reasons[k] || a.structural[k] {
		return
	}
	a.problems = append(a.problems, fmt.Sprintf("%s: key %q is not a registered metric id, suppression reason or structural name", path, k))
}

func (a *surfaceAuditor) checkString(path, s string) {
	if id, ok := a.aliases[s]; ok {
		a.problems = append(a.problems, fmt.Sprintf("%s: value %q is an alias of %s", path, s, id))
		return
	}
	for _, l := range a.levels {
		if strings.HasPrefix(s, l) && !a.metrics[s] {
			a.problems = append(a.problems, fmt.Sprintf("%s: value %q claims level %q but is not a registered metric id", path, s, strings.TrimSuffix(l, ".")))
			return
		}
	}
}
