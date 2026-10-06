package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/db"
	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
	"github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

const (
	headwaySourceA   = "source/v1/a-headway-api-test"
	headwaySourceB   = "source/v1/b-headway-api-test"
	headwaySourceC   = "source/v1/c-headway-api-test"
	nanosPerSecondNS = int64(1_000_000_000)
	hourNS           = 3600 * nanosPerSecondNS
)

// verdictWords are words no served headway text may contain: the surface
// describes observed gaps and time gaps, never a road user or a judgement
// of one (Sections 1 and 10.4 of the behaviour plan).
var verdictWords = regexp.MustCompile(`(?i)tailgat|risk|driver|score|unsafe|danger|aggressi|verdict`)

// assertNoVerdictWords checks every key and string value of a JSON body.
func assertNoVerdictWords(t *testing.T, body []byte) {
	t.Helper()
	var v any
	if err := json.Unmarshal(body, &v); err != nil {
		t.Fatalf("not JSON: %v", err)
	}
	var walk func(path string, v any)
	walk = func(path string, v any) {
		switch x := v.(type) {
		case map[string]any:
			for k, child := range x {
				if verdictWords.MatchString(k) {
					t.Errorf("%s: key %q is a verdict word", path, k)
				}
				walk(path+"."+k, child)
			}
		case []any:
			for _, child := range x {
				walk(path+"[]", child)
			}
		case string:
			if verdictWords.MatchString(x) {
				t.Errorf("%s: %q contains a verdict word", path, x)
			}
		}
	}
	walk("$", v)
}

func TestVerdictWordsCatchesTheWordsItNames(t *testing.T) {
	for _, s := range []string{"Tailgating", "risk score", "driver", "UNSAFE", "aggressive"} {
		if !verdictWords.MatchString(s) {
			t.Errorf("%q passed", s)
		}
	}
	for _, s := range []string{"interaction.following_net_time_gap_s", "no_established_threshold", "PROVISIONAL"} {
		if verdictWords.MatchString(s) {
			t.Errorf("%q was refused", s)
		}
	}
}

// shiftedScenario moves a scenario in capture time.
func shiftedScenario(sc l8behaviour.EncounterScenario, byNanos int64) l8behaviour.EncounterScenario {
	for i := range sc.Trajectories {
		for j := range sc.Trajectories[i].Samples {
			sc.Trajectories[i].Samples[j].CaptureUnixNanos += byNanos
			sc.Trajectories[i].Samples[j].LastObservedUnixNanos += byNanos
		}
	}
	return sc
}

// atStage relabels a scenario's estimates to another stage.
func atStage(sc l8behaviour.EncounterScenario, stage l8behaviour.EstimateStage) l8behaviour.EncounterScenario {
	for i := range sc.Trajectories {
		for j := range sc.Trajectories[i].Samples {
			sc.Trajectories[i].Samples[j].Stage = stage
		}
	}
	return sc
}

// fromEstimator relabels a scenario as the output of a field estimator.
func fromEstimator(sc l8behaviour.EncounterScenario) l8behaviour.EncounterScenario {
	for i := range sc.Trajectories {
		sc.Trajectories[i].Estimate = l8behaviour.EstimateIdentity{
			EstimatorID: "cv_kf_v1", ObsModelID: "medoid_v0", ParamHash: "sha256:headway-api-test",
		}
	}
	return sc
}

func interactionsOfScenario(t *testing.T, sourceID string, sc l8behaviour.EncounterScenario) []l8behaviour.FollowingInteraction {
	t.Helper()
	a, err := l8behaviour.AnalyseFollowing(sc.Trajectories, sc.Params)
	if err != nil {
		t.Fatalf("%s: %v", sc.Name, err)
	}
	fis, err := l8behaviour.FollowingInteractions(sourceID, a)
	if err != nil {
		t.Fatalf("%s: %v", sc.Name, err)
	}
	return fis
}

// headwayFixture is the seeded database: what was stored, by role.
type headwayFixture struct {
	final, fixed, ambiguous, other []l8behaviour.FollowingInteraction
}

// seedHeadway stores following interactions and scenes:
//
//   - source A at the fixture base time: three final encounters, the steady
//     approach again over fixed-lag estimates, and the two ambiguous
//     encounters under another method version, written first;
//   - sources B and C an hour later: the steady approach from a field
//     estimator, extracted twice;
//   - scenes over source A, over B and C, over nothing, and with no window.
func seedHeadway(t *testing.T, dbInst *db.DB) headwayFixture {
	t.Helper()
	base := l8behaviour.FixtureBaseUnixNanos
	f := headwayFixture{
		ambiguous: interactionsOfScenario(t, headwaySourceA, l8behaviour.ScenarioAmbiguousThenResolved()),
		final: append(append(interactionsOfScenario(t, headwaySourceA, l8behaviour.ScenarioSteadyApproach()),
			interactionsOfScenario(t, headwaySourceA, l8behaviour.ScenarioOcclusion())...),
			interactionsOfScenario(t, headwaySourceA, l8behaviour.ScenarioStandstillQueue())...),
		fixed: interactionsOfScenario(t, headwaySourceA, atStage(l8behaviour.ScenarioSteadyApproach(), l8behaviour.StageFixedLag)),
		other: interactionsOfScenario(t, headwaySourceB, fromEstimator(shiftedScenario(l8behaviour.ScenarioSteadyApproach(), hourNS))),
	}
	twin := interactionsOfScenario(t, headwaySourceC, fromEstimator(shiftedScenario(l8behaviour.ScenarioSteadyApproach(), hourNS)))
	store := sqlite.NewInteractionStore(dbInst)
	for i, batch := range [][]l8behaviour.FollowingInteraction{f.ambiguous, f.fixed, f.final, f.other, twin} {
		if err := store.Insert(batch...); err != nil {
			t.Fatal(err)
		}
		// Pin the write order, so "most recently written" is not a race.
		for _, fi := range batch {
			if _, err := dbInst.Exec(`UPDATE lidar_interaction_events SET inserted_at_ns = ? WHERE event_id = ?`,
				int64(100+i), fi.Event.EventID); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, sc := range []struct {
		id         string
		start, end int64
	}{
		{"headway-a", base, base + 10*nanosPerSecondNS},
		{"headway-twin", base + hourNS, base + hourNS + 10*nanosPerSecondNS},
		{"headway-empty", base + 24*hourNS, base + 24*hourNS + 10*nanosPerSecondNS},
		{"headway-partial", f.other[0].Event.EndUnixNanos, f.other[0].Event.EndUnixNanos + 1},
		{"headway-none", 0, 0},
	} {
		scene := &db.Scene{SceneID: sc.id, Title: sc.id}
		if sc.start > 0 {
			start, end := sc.start, sc.end
			scene.CapturedStartNs, scene.CapturedEndNs = &start, &end
		}
		if err := dbInst.CreateScene(context.Background(), scene); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// getHeadway requests a scene's headway through the server's mux, and
// audits and checks every successful body.
func getHeadway(t *testing.T, server *Server, sceneID string, q url.Values) (int, sceneHeadwayResponse, []byte) {
	t.Helper()
	target := "/api/scenes/" + sceneID + "/headway"
	if len(q) > 0 {
		target += "?" + q.Encode()
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	w := httptest.NewRecorder()
	server.ServeMux().ServeHTTP(w, req)
	body := w.Body.Bytes()
	var resp sceneHeadwayResponse
	if w.Code == http.StatusOK {
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("content type %q", ct)
		}
		if err := l8behaviour.AuditSurfaceJSON(body); err != nil {
			t.Fatalf("served headway uses unregistered names: %v", err)
		}
		assertNoVerdictWords(t, body)
		if err := json.Unmarshal(body, &resp); err != nil {
			t.Fatalf("decode: %v\n%s", err, body)
		}
		if !strings.HasPrefix(resp.Status.Label(), "PROVISIONAL") {
			t.Fatalf("status %q is not visibly provisional", resp.Status)
		}
	}
	return w.Code, resp, body
}

func TestSceneHeadwayServesTheLatestFinalVersion(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)
	f := seedHeadway(t, dbInst)

	code, resp, _ := getHeadway(t, server, "headway-a", nil)
	if code != http.StatusOK || resp.Availability != headwayAvailable {
		t.Fatalf("%d %q", code, resp.Availability)
	}
	vFinal := f.final[0].Event.Version.InteractionVersion()
	if resp.SourceID != headwaySourceA || resp.Version == nil || *resp.Version != vFinal ||
		resp.Status != l8behaviour.StatusSyntheticOracle {
		t.Fatalf("source %s, version %+v, status %s", resp.SourceID, resp.Version, resp.Status)
	}
	if len(resp.Sources) != 1 || resp.Sources[0].Events != 6 {
		t.Fatalf("sources %+v", resp.Sources)
	}
	var versions []l8behaviour.InteractionVersion
	for _, v := range resp.Versions {
		versions = append(versions, v.Version)
	}
	want := []l8behaviour.InteractionVersion{vFinal, f.fixed[0].Event.Version.InteractionVersion(),
		f.ambiguous[0].Event.Version.InteractionVersion()}
	if !reflect.DeepEqual(versions, want) {
		t.Fatalf("versions %+v, want newest first %+v", versions, want)
	}

	direct, err := l8behaviour.AggregateFollowing(f.final)
	if err != nil {
		t.Fatal(err)
	}
	if resp.Distribution == nil || !reflect.DeepEqual(*resp.Distribution, direct) {
		t.Fatal("the served distribution is not the aggregate of the stored final encounters")
	}
	if len(resp.Encounters) != len(f.final) {
		t.Fatalf("%d encounter rows, want %d", len(resp.Encounters), len(f.final))
	}
	ids := map[string]bool{}
	for _, fi := range f.final {
		ids[fi.Event.EventID] = true
	}
	for _, row := range resp.Encounters {
		if !ids[row.EventID] || row.PrimaryTrackID == "" || row.SecondaryTrackID == "" || row.Provisional != nil {
			t.Fatalf("encounter row %+v", row)
		}
		min := row.Measurements[l8behaviour.MetricFollowingNetTimeGapMin]
		if !min.Suppressed && (min.Uncertainty == nil || min.Uncertainty.Method != l8behaviour.MethodMonteCarlo) {
			t.Fatalf("%s: minimum net time gap lost its stored interval: %+v", row.EventID, min)
		}
	}
	if *resp.StartUnixNanos != l8behaviour.FixtureBaseUnixNanos || *resp.EndUnixNanos-*resp.StartUnixNanos != 10*nanosPerSecondNS {
		t.Fatalf("window %d to %d", *resp.StartUnixNanos, *resp.EndUnixNanos)
	}
}

func TestSceneHeadwaySelectsAVersion(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)
	f := seedHeadway(t, dbInst)

	// A fixed-lag version is served from its review-only block and labelled
	// by its stage; its production block stays suppressed.
	code, resp, _ := getHeadway(t, server, "headway-a", url.Values{"stage": {"fixed_lag"}})
	if code != http.StatusOK || resp.Availability != headwayAvailable || resp.Version.EstimateStage != l8behaviour.StageFixedLag {
		t.Fatalf("%d %q %+v", code, resp.Availability, resp.Version)
	}
	if resp.Distribution.ExposureEvents != 1 || len(resp.Encounters) != 1 {
		t.Fatalf("fixed-lag: %d with exposure, %d rows", resp.Distribution.ExposureEvents, len(resp.Encounters))
	}
	row := resp.Encounters[0]
	if row.Provisional == nil || !row.Measurements[l8behaviour.MetricFollowingValidTime].Suppressed ||
		row.Measurements[l8behaviour.MetricFollowingValidTime].Reason != l8behaviour.ReasonEstimateNotFinal ||
		row.Provisional[l8behaviour.MetricFollowingValidTime].Value == nil {
		t.Fatalf("fixed-lag row %+v", row)
	}

	// All four axes name one version exactly: the ambiguous group, whose
	// band exposure is suppressed and shown beside the empty bins.
	v := f.ambiguous[0].Event.Version.InteractionVersion()
	code, resp, _ = getHeadway(t, server, "headway-a", url.Values{
		"estimator_id": {v.EstimatorID}, "obs_model_id": {v.ObsModelID}, "method_id": {v.MethodID}, "param_hash": {v.ParamHash},
	})
	if code != http.StatusOK || resp.Version == nil || *resp.Version != v || resp.Distribution.ExposureEvents != 0 ||
		resp.Distribution.Excluded[l8behaviour.ReasonInsufficientObservation].Nanos == 0 {
		t.Fatalf("exact version: %d %+v", code, resp.Version)
	}

	// No online version: say so, list what exists, never fall back.
	code, resp, _ = getHeadway(t, server, "headway-a", url.Values{"stage": {"online"}})
	if code != http.StatusOK || resp.Availability != headwayNoMatchingVersion || resp.Distribution != nil ||
		resp.Version != nil || len(resp.Versions) != 3 || len(resp.Encounters) != 0 {
		t.Fatalf("online: %d %q %d versions", code, resp.Availability, len(resp.Versions))
	}
	mismatch := url.Values{"stage": {"fixed_lag"}, "estimator_id": {v.EstimatorID}, "obs_model_id": {v.ObsModelID},
		"method_id": {v.MethodID}, "param_hash": {v.ParamHash}}
	if code, resp, _ = getHeadway(t, server, "headway-a", mismatch); code != http.StatusOK || resp.Availability != headwayNoMatchingVersion {
		t.Fatalf("a version at the wrong stage: %d %q", code, resp.Availability)
	}

	for name, q := range map[string]url.Values{
		"unknown stage":  {"stage": {"latest"}},
		"partial axes":   {"estimator_id": {v.EstimatorID}},
		"foreign source": {"source_id": {headwaySourceB}},
	} {
		if code, _, body := getHeadway(t, server, "headway-a", q); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", name, code, body)
		}
	}
}

func TestSceneHeadwayDerivesTheSourceFromTheCaptureWindow(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)
	f := seedHeadway(t, dbInst)

	// Two extractions of one capture: listed, not merged, until one is named.
	code, resp, _ := getHeadway(t, server, "headway-twin", nil)
	if code != http.StatusOK || resp.Availability != headwaySourceAmbiguous || resp.SourceID != "" ||
		len(resp.Sources) != 2 || resp.Distribution != nil {
		t.Fatalf("two sources: %d %q %+v", code, resp.Availability, resp.Sources)
	}
	code, resp, _ = getHeadway(t, server, "headway-twin", url.Values{"source_id": {headwaySourceB}})
	if code != http.StatusOK || resp.Availability != headwayAvailable || resp.SourceID != headwaySourceB ||
		resp.Status != l8behaviour.StatusProvisional || resp.Encounters[0].EventID != f.other[0].Event.EventID {
		t.Fatalf("named source: %d %q %s %s", code, resp.Availability, resp.SourceID, resp.Status)
	}

	for scene, want := range map[string]string{
		"headway-empty":   headwayNoEncounters,
		"headway-partial": headwayNoEncounters,
		"headway-none":    headwayNoCaptureWindow,
	} {
		code, resp, _ := getHeadway(t, server, scene, nil)
		if code != http.StatusOK || resp.Availability != want || resp.Distribution != nil ||
			resp.Encounters == nil || resp.Sources == nil || resp.Versions == nil {
			t.Errorf("%s: %d %+v", scene, code, resp)
		}
	}
}

func TestSceneHeadwayRouting(t *testing.T) {
	server, dbInst := setupTestServer(t)
	defer cleanupTestServer(t, dbInst)
	seedHeadway(t, dbInst)

	if code, _, _ := getHeadway(t, server, "no-such-scene", nil); code != http.StatusNotFound {
		t.Fatalf("unknown scene: %d", code)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/scenes/headway-a/headway", nil)
	w := httptest.NewRecorder()
	server.ServeMux().ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST: %d", w.Code)
	}
	// The scene itself is still served as before.
	req = httptest.NewRequest(http.MethodGet, "/api/scenes/headway-a", nil)
	w = httptest.NewRecorder()
	server.ServeMux().ServeHTTP(w, req)
	var scene db.Scene
	if w.Code != http.StatusOK || json.Unmarshal(w.Body.Bytes(), &scene) != nil || scene.SceneID != "headway-a" {
		t.Fatalf("scene: %d %s", w.Code, w.Body.String())
	}
}
