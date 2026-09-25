package l8behaviour

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// registrySurface is one external behaviour surface and a sample of what it
// serves. Add a row for every surface that can be built from this package's
// types. A surface built elsewhere (an API handler, a report data file) calls
// AuditSurfaceJSON on its own output from its own package's tests, against the
// same SurfaceFieldNames.
type registrySurface struct {
	name    string
	samples func(t *testing.T) [][]byte
}

func encodeAll[T any](t *testing.T, values []T) [][]byte {
	t.Helper()
	out := make([][]byte, 0, len(values))
	for _, v := range values {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		out = append(out, raw)
	}
	return out
}

// surfaceInteractions are the stored records of every scenario, final and
// provisional, so every record shape the mapping produces is audited.
func surfaceInteractions(t *testing.T) []FollowingInteraction {
	t.Helper()
	var out []FollowingInteraction
	for _, sc := range append(EncounterScenarios(), provisionalScenario()) {
		_, fis := interactionsOf(t, sc)
		out = append(out, fis...)
	}
	return out
}

func registrySurfaces() []registrySurface {
	return []registrySurface{
		{"persistence: lidar_interaction_events.event_json", func(t *testing.T) [][]byte {
			var events []InteractionEvent
			for _, fi := range surfaceInteractions(t) {
				events = append(events, fi.Event)
			}
			return encodeAll(t, events)
		}},
		{"persistence: lidar_interaction_instants.instant_json", func(t *testing.T) [][]byte {
			var instants []InteractionInstant
			for _, fi := range surfaceInteractions(t) {
				instants = append(instants, fi.Instants...)
			}
			return encodeAll(t, instants)
		}},
		{"persistence: lidar_exposure_windows.window_json", func(t *testing.T) [][]byte {
			var windows []ExposureWindow
			for _, fi := range surfaceInteractions(t) {
				windows = append(windows, fi.Windows...)
			}
			return encodeAll(t, windows)
		}},
	}
}

// Every name a registered surface emits is a registered metric id, a
// registered suppression reason or a declared structural name.
func TestRegistrySurfacesUseRegisteredNames(t *testing.T) {
	for _, s := range registrySurfaces() {
		t.Run(s.name, func(t *testing.T) {
			samples := s.samples(t)
			if len(samples) == 0 {
				t.Fatal("no samples")
			}
			for _, raw := range samples {
				if err := AuditSurfaceJSON(raw); err != nil {
					t.Fatalf("%v\nin %s", err, raw)
				}
			}
		})
	}
}

// The metric-keyed blocks of the stored records use every id they should,
// so the audit above is exercised on metric keys and not only on fields.
func TestStoredRecordsKeyByRegistryIDs(t *testing.T) {
	keys := map[MetricID]bool{}
	for _, fi := range surfaceInteractions(t) {
		for id := range fi.Event.Measurements {
			keys[id] = true
		}
		for id := range fi.Event.Accounting.BandNanos {
			keys[id] = true
		}
		for _, in := range fi.Instants {
			for id := range in.Values {
				keys[id] = true
			}
		}
	}
	want := append(EncounterMetrics(), MetricFollowingSpatialGap, MetricFollowingNetTimeGap, MetricFollowingPredictedGap)
	for _, id := range want {
		if !keys[id] {
			t.Errorf("no stored record is keyed by %s", id)
		}
	}
	for id := range keys {
		if _, ok := LookupMetric(id); !ok {
			t.Errorf("stored key %s is not registered", id)
		}
	}
}

// A structural name is never a metric id, a reason token or an alias, and is
// listed once.
func TestSurfaceFieldNamesAreNotMetricNames(t *testing.T) {
	names := SurfaceFieldNames()
	if !sort.StringsAreSorted(names) {
		t.Fatal("SurfaceFieldNames is not sorted")
	}
	reasons := map[string]bool{}
	for _, r := range SuppressionReasons() {
		reasons[r.String()] = true
	}
	aliases := map[string]bool{}
	for _, d := range FollowingMetrics() {
		for _, a := range d.Aliases {
			aliases[a] = true
		}
	}
	for i, n := range names {
		if i > 0 && names[i-1] == n {
			t.Errorf("%s listed twice", n)
		}
		if _, ok := LookupMetric(MetricID(n)); ok || reasons[n] || aliases[n] || !snakeCase.MatchString(n) {
			t.Errorf("structural name %q is a metric id, reason, alias or not snake_case", n)
		}
	}
}

// Every JSON field of the stored record types is a declared structural name,
// including fields the scenarios never populate.
func TestSurfaceFieldNamesCoverRecordTypes(t *testing.T) {
	declared := map[string]bool{}
	for _, n := range SurfaceFieldNames() {
		declared[n] = true
	}
	seen := map[reflect.Type]bool{}
	var walk func(reflect.Type)
	walk = func(rt reflect.Type) {
		for rt.Kind() == reflect.Pointer || rt.Kind() == reflect.Slice || rt.Kind() == reflect.Map {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct || seen[rt] {
			return
		}
		seen[rt] = true
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
			if name == "" || name == "-" {
				t.Errorf("%s.%s has no JSON name", rt.Name(), f.Name)
				continue
			}
			if !declared[name] {
				t.Errorf("%s.%s is stored as %q, which is not in SurfaceFieldNames", rt.Name(), f.Name, name)
			}
			walk(f.Type)
		}
	}
	for _, v := range []any{InteractionEvent{}, InteractionInstant{}, ExposureWindow{}} {
		walk(reflect.TypeOf(v))
	}
}

func TestAuditSurfaceJSONRejectsInventedNames(t *testing.T) {
	for _, c := range []struct {
		doc, want string
	}{
		{`{"thw": 1.2}`, "alias of interaction.following_net_time_gap_s"},
		{`{"values": {"gap": {"value": 3}}}`, "alias of interaction.following_spatial_gap_m"},
		{`{"measurements": {"interaction.following_gap_m": {}}}`, `key "interaction.following_gap_m" is not a registered`},
		{`{"headway_seconds": 1}`, `key "headway_seconds" is not a registered`},
		{`{"name": "interaction.following_headway_s"}`, `claims level "interaction"`},
		{`{"name": "time_headway"}`, "value \"time_headway\" is an alias"},
		{`[{"reason": "not_observed"}, {"suppressions": {"tailgating": {}}}]`, `$[1].suppressions: key "tailgating"`},
		{`not json`, "not JSON"},
		{`{"measurements": {}} {"thw": 1}`, "not a single JSON document"},
		{`{"measurements": {}} trailing`, "not a single JSON document"},
	} {
		err := AuditSurfaceJSON([]byte(c.doc))
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err %v, want it to mention %q", c.doc, err, c.want)
		}
	}
	// Every problem is reported, not only the first.
	err := AuditSurfaceJSON([]byte(`{"thw": 1, "gap": 2, "headway": 3}`))
	if err == nil || strings.Count(err.Error(), "\n") != 3 {
		t.Fatalf("want three problems, got %v", err)
	}
	ok := `{"measurements": {"interaction.following_valid_time_s": {"name": "interaction.following_valid_time_s",` +
		` "reason": "not_observed"}}, "accounting": {"suppressions": {"below_speed_floor": {"instants": 1, "nanos": 5}}}}`
	if err := AuditSurfaceJSON([]byte(ok)); err != nil {
		t.Fatal(err)
	}
	// Trailing whitespace, as an encoder writes it, is still one document.
	if err := AuditSurfaceJSON([]byte(ok + "\n\t ")); err != nil {
		t.Fatalf("trailing whitespace refused: %v", err)
	}
}
