package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// solidBodyBlockJSON is a complete solid-body block, every key present, as
// the strict loader requires once the block is there.
func solidBodyBlockJSON(overrides map[string]interface{}) string {
	block := map[string]interface{}{
		"enabled": true, "full_members": true, "near_edge_tracking": true, "near_edge_medoid_gate": false,
		"face_hysteresis": true, "face_entry_consider": false, "course_aligned_faces": true,
		"reference_translation": false, "rank_one_medoid_scale": 0, "course_heading": false,
		"extent_prior_floor": false, "face_plane_spans": false, "extent_growth_admission": true,
		"vehicle_extent_floor": true, "end_face_centring": true, "end_face_centring_open_prior": true,
		"containment": true, "rectangle_fit": false, "rectangle_heading": true,
		"rectangle_course_fusion": true, "rectangle_sigma_scale": 1.5,
	}
	for k, v := range overrides {
		block[k] = v
	}
	data, err := json.Marshal(block)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// withSolidBody is the sample config with the block spliced into its L5
// engine block.
func withSolidBody(t *testing.T, block string) []byte {
	t.Helper()
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(mustMarshalJSON(t, sampleValidConfig()), &raw); err != nil {
		t.Fatal(err)
	}
	var l5 map[string]json.RawMessage
	if err := json.Unmarshal(raw["l5"], &l5); err != nil {
		t.Fatal(err)
	}
	var engine map[string]json.RawMessage
	if err := json.Unmarshal(l5["cv_kf_v1"], &engine); err != nil {
		t.Fatal(err)
	}
	engine["solid_body"] = json.RawMessage(block)
	l5["cv_kf_v1"], _ = json.Marshal(engine)
	raw["l5"], _ = json.Marshal(l5)
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// The block is optional: a config without it loads as before and carries no
// solid body, and the shipped defaults never carry one, so their fingerprint
// is the one the perf baselines were captured against.
func TestSolidBodyBlockIsAbsentByDefault(t *testing.T) {
	cfg, err := LoadTuningConfig(writeConfigFile(t, mustMarshalJSON(t, sampleValidConfig())))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.L5.CvKfV1.SolidBody != nil {
		t.Fatalf("a config without the block loaded one: %+v", cfg.L5.CvKfV1.SolidBody)
	}
	if MustLoadDefaultConfig().L5.CvKfV1.SolidBody != nil {
		t.Fatal("the shipped defaults carry a solid-body block; that moves every fingerprint")
	}
	if !strings.Contains(mustJSON(t, cfg), `"min_observations_for_classification"`) || strings.Contains(mustJSON(t, cfg), `"solid_body"`) {
		t.Fatal("a nil block was written out, which would change the fingerprint of every config")
	}
}

// With the block present every key is required and read, and the config's
// fingerprint differs from the same config without it.
func TestSolidBodyBlockLoadsAndFingerprints(t *testing.T) {
	plain, err := LoadTuningConfig(writeConfigFile(t, mustMarshalJSON(t, sampleValidConfig())))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := LoadTuningConfig(writeConfigFile(t, withSolidBody(t, solidBodyBlockJSON(nil))))
	if err != nil {
		t.Fatalf("a complete block was refused: %v", err)
	}
	sb := cfg.L5.CvKfV1.SolidBody
	if sb == nil || !sb.Enabled || !sb.FullMembers || !sb.NearEdgeTracking || !sb.RectangleHeading || sb.RectangleSigmaScale != 1.5 || !sb.EndFaceCentringOpenPrior {
		t.Fatalf("the block did not round-trip: %+v", sb)
	}
	if cfg.Fingerprint() == plain.Fingerprint() {
		t.Fatal("the block did not change the fingerprint, so two runs that differ by it would read as one workload")
	}
	// Written back out, the block is there and loads the same.
	again, err := ParseTuningConfig([]byte(mustJSON(t, cfg)))
	if err != nil {
		t.Fatalf("the marshalled config does not load: %v", err)
	}
	if again.Fingerprint() != cfg.Fingerprint() {
		t.Fatal("marshal and load changed the fingerprint")
	}
	// A block missing a key is refused by name, as every engine key is.
	var partial map[string]interface{}
	_ = json.Unmarshal([]byte(solidBodyBlockJSON(nil)), &partial)
	delete(partial, "containment")
	data, _ := json.Marshal(partial)
	if _, err := LoadTuningConfig(writeConfigFile(t, withSolidBody(t, string(data)))); err == nil || !strings.Contains(err.Error(), "containment") {
		t.Fatalf("a block missing containment loaded, or the error does not name it: %v", err)
	}
	if _, err := LoadTuningConfig(writeConfigFile(t, withSolidBody(t, solidBodyBlockJSON(map[string]interface{}{"course_aligned": true})))); err == nil || !strings.Contains(err.Error(), "course_aligned") {
		t.Fatalf("a block with an unknown key loaded, or the error does not name it: %v", err)
	}
}

// The block's rules: qualifiers without the estimator, the estimator without
// the members, A1 without A2, open-prior centring without centring, and a
// negative scale are each refused with the key named.
func TestSolidBodyBlockValidation(t *testing.T) {
	for _, c := range []struct {
		name      string
		overrides map[string]interface{}
		want      string
	}{
		{"qualified but disabled", map[string]interface{}{"enabled": false, "full_members": false}, "enabled is false"},
		{"members only, disabled", map[string]interface{}{"enabled": false, "full_members": true, "near_edge_tracking": false, "face_hysteresis": false, "course_aligned_faces": false, "extent_growth_admission": false, "vehicle_extent_floor": false, "end_face_centring": false, "end_face_centring_open_prior": false, "containment": false, "rectangle_heading": false, "rectangle_course_fusion": false, "rectangle_sigma_scale": 0}, "enabled is false"},
		{"enabled without members", map[string]interface{}{"full_members": false}, "requires full_members"},
		{"A1 without A2", map[string]interface{}{"near_edge_tracking": false, "near_edge_medoid_gate": true}, "near_edge_medoid_gate"},
		{"open prior without centring", map[string]interface{}{"end_face_centring": false}, "end_face_centring_open_prior"},
		{"negative sigma scale", map[string]interface{}{"rectangle_sigma_scale": -1}, "rectangle_sigma_scale"},
		{"negative rank-one scale", map[string]interface{}{"rank_one_medoid_scale": -0.5}, "rank_one_medoid_scale"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, err := LoadTuningConfig(writeConfigFile(t, withSolidBody(t, solidBodyBlockJSON(c.overrides))))
			if err == nil || !strings.Contains(err.Error(), c.want) {
				t.Fatalf("got %v, want an error naming %q", err, c.want)
			}
		})
	}
	// The estimator with the members and nothing else is the plain shadow,
	// and valid.
	off := map[string]interface{}{"near_edge_tracking": false, "face_hysteresis": false, "course_aligned_faces": false, "extent_growth_admission": false, "vehicle_extent_floor": false, "end_face_centring": false, "end_face_centring_open_prior": false, "containment": false, "rectangle_heading": false, "rectangle_course_fusion": false, "rectangle_sigma_scale": 0}
	if _, err := LoadTuningConfig(writeConfigFile(t, withSolidBody(t, solidBodyBlockJSON(off)))); err != nil {
		t.Fatalf("the plain shadow was refused: %v", err)
	}
	var none *L5SolidBody
	if err := none.Validate(); err != nil || none.Qualified() {
		t.Fatalf("a nil block is the estimator off: %v %v", err, none.Qualified())
	}
}
