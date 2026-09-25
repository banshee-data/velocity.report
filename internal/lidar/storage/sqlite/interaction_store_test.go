package sqlite

import (
	"database/sql"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/l8behaviour"
)

const interactionTestSource = "source/v1/interaction-store-test"

// analysedInteractions runs a frozen scenario through the following method and
// maps its encounters to their stored form.
func analysedInteractions(t *testing.T, sc l8behaviour.EncounterScenario) []l8behaviour.FollowingInteraction {
	t.Helper()
	a, err := l8behaviour.AnalyseFollowing(sc.Trajectories, sc.Params)
	if err != nil {
		t.Fatalf("%s: analyse: %v", sc.Name, err)
	}
	fis, err := l8behaviour.FollowingInteractions(interactionTestSource, a)
	if err != nil {
		t.Fatalf("%s: map: %v", sc.Name, err)
	}
	return fis
}

// fixedLag is a scenario over fixed-lag estimates: stored, never published.
func fixedLag(sc l8behaviour.EncounterScenario) l8behaviour.EncounterScenario {
	for i := range sc.Trajectories {
		for j := range sc.Trajectories[i].Samples {
			sc.Trajectories[i].Samples[j].Stage = l8behaviour.StageFixedLag
		}
	}
	return sc
}

func countRows(t *testing.T, db *sql.DB, table string) int {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func interactionCounts(t *testing.T, db *sql.DB) [3]int {
	return [3]int{
		countRows(t, db, "lidar_interaction_events"),
		countRows(t, db, "lidar_interaction_instants"),
		countRows(t, db, "lidar_exposure_windows"),
	}
}

// Every scenario's interactions read back exactly as they were written.
func TestInteractionStoreRoundTrip(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewInteractionStore(db)
	var all []l8behaviour.FollowingInteraction
	for _, sc := range append(l8behaviour.EncounterScenarios(), fixedLag(l8behaviour.ScenarioOcclusion())) {
		all = append(all, analysedInteractions(t, sc)...)
	}
	if err := store.Insert(all...); err != nil {
		t.Fatal(err)
	}
	var instants, windows int
	for _, fi := range all {
		got, err := store.Get(fi.Event.EventID)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got, fi) {
			t.Fatalf("interaction %s changed in storage", fi.Event.EventID)
		}
		instants += len(fi.Instants)
		windows += len(fi.Windows)
	}
	if got := interactionCounts(t, db); got != [3]int{len(all), instants, windows} {
		t.Fatalf("row counts %v, want %d events, %d instants, %d windows", got, len(all), instants, windows)
	}
	if _, err := store.Get("interaction/v1/missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing id: %v", err)
	}
}

// The generated columns index what the payload says: roles, type, stage,
// worst support, and each instant's basis and validity.
func TestInteractionStoreIndexesThePayload(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	fi := analysedInteractions(t, l8behaviour.ScenarioOcclusion())[0]
	if err := NewInteractionStore(db).Insert(fi); err != nil {
		t.Fatal(err)
	}
	var source, typ, primary, secondary, stage, worst string
	var start, end int64
	if err := db.QueryRow(`
		SELECT source_id, interaction_type, primary_track_id, secondary_track_id, estimate_stage, worst_support,
		       start_unix_nanos, end_unix_nanos
		  FROM lidar_interaction_events WHERE event_id = ?`, fi.Event.EventID).
		Scan(&source, &typ, &primary, &secondary, &stage, &worst, &start, &end); err != nil {
		t.Fatal(err)
	}
	ev := fi.Event
	if source != interactionTestSource || typ != "following" || primary != ev.PrimaryTrackID ||
		secondary != ev.SecondaryTrackID || stage != "final" || worst != "coasted" ||
		start != ev.StartUnixNanos || end != ev.EndUnixNanos {
		t.Fatalf("indexed %s %s %s %s %s %s %d %d", source, typ, primary, secondary, stage, worst, start, end)
	}
	var predicted, validPredicted int
	if err := db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(valid), 0) FROM lidar_interaction_instants
		 WHERE event_id = ? AND basis = 'predicted_only'`, ev.EventID).Scan(&predicted, &validPredicted); err != nil {
		t.Fatal(err)
	}
	if predicted != 5 || validPredicted != 0 {
		t.Fatalf("%d predicted instants, %d of them valid", predicted, validPredicted)
	}
}

// Write-once: an identical re-insert is a no-op, different content under the
// same id is refused, and a refused batch writes nothing at all.
func TestInteractionStoreIsWriteOnce(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewInteractionStore(db)
	steady := analysedInteractions(t, l8behaviour.ScenarioSteadyApproach())[0]
	if err := store.Insert(steady); err != nil {
		t.Fatal(err)
	}
	before := interactionCounts(t, db)
	if err := store.Insert(steady); err != nil {
		t.Fatalf("identical re-insert: %v", err)
	}
	if interactionCounts(t, db) != before {
		t.Fatal("identical re-insert wrote rows")
	}

	// Same identity, a different interval bound: valid, and a silent change.
	changed, err := store.Get(steady.Event.EventID)
	if err != nil {
		t.Fatal(err)
	}
	m := changed.Event.Measurements[l8behaviour.MetricFollowingSpatialGapMin]
	u := *m.Uncertainty
	upper := *u.Upper + 0.001
	u.Upper = &upper
	m.Uncertainty = &u
	changed.Event.Measurements[l8behaviour.MetricFollowingSpatialGapMin] = m
	if err := changed.Validate(); err != nil {
		t.Fatalf("the changed interaction must itself be valid: %v", err)
	}
	other := analysedInteractions(t, l8behaviour.ScenarioOcclusion())[0]
	if err := store.Insert(other, changed); !errors.Is(err, ErrInteractionConflict) {
		t.Fatalf("conflicting insert: %v", err)
	}
	if interactionCounts(t, db) != before {
		t.Fatal("a refused batch wrote rows")
	}
	got, err := store.Get(steady.Event.EventID)
	if err != nil || !reflect.DeepEqual(got, steady) {
		t.Fatalf("stored interaction changed: %v", err)
	}
}

// Regeneration under a new version writes beside the old; every query
// selects exactly one version, and final never returns a fixed-lag version.
func TestInteractionStoreVersionsCoexist(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewInteractionStore(db)

	sc := l8behaviour.ScenarioSteadyApproach()
	first := analysedInteractions(t, sc)
	sc.Params.Exposure.MonteCarloSamples = 1000 // a parameter change is a method version change
	second := analysedInteractions(t, sc)
	provisional := analysedInteractions(t, fixedLag(l8behaviour.ScenarioSteadyApproach()))
	v1, v2 := first[0].Event.Version.InteractionVersion(), second[0].Event.Version.InteractionVersion()
	vp := provisional[0].Event.Version.InteractionVersion()
	if v1 == v2 || v1.MethodID == v2.MethodID || first[0].Event.EventID == second[0].Event.EventID {
		t.Fatal("a parameter change did not move the version")
	}
	for i, batch := range [][]l8behaviour.FollowingInteraction{first, provisional, second} {
		if err := insertInteractions(db, batch, int64(100+i)); err != nil {
			t.Fatal(err)
		}
	}
	if got := countRows(t, db, "lidar_interaction_events"); got != 3 {
		t.Fatalf("%d events, want all three versions kept", got)
	}

	versions, err := store.Versions(interactionTestSource)
	if err != nil {
		t.Fatal(err)
	}
	var got []l8behaviour.InteractionVersion
	for _, v := range versions {
		if v.Events != 1 {
			t.Errorf("version %+v has %d events", v.Version, v.Events)
		}
		got = append(got, v.Version)
	}
	if !reflect.DeepEqual(got, []l8behaviour.InteractionVersion{v2, vp, v1}) {
		t.Fatalf("versions %+v, want newest first", got)
	}
	latest, err := store.LatestVersion(interactionTestSource, l8behaviour.StageFinal)
	if err != nil || latest != v2 {
		t.Fatalf("latest final %+v, %v", latest, err)
	}
	if latest, err := store.LatestVersion(interactionTestSource, l8behaviour.StageFixedLag); err != nil || latest != vp {
		t.Fatalf("latest fixed-lag %+v, %v", latest, err)
	}
	if _, err := store.LatestVersion(interactionTestSource, l8behaviour.StageOnline); !errors.Is(err, ErrNotFound) {
		t.Fatalf("latest online: %v", err)
	}
	for _, c := range []struct {
		v    l8behaviour.InteractionVersion
		want l8behaviour.InteractionEvent
	}{{v1, first[0].Event}, {v2, second[0].Event}, {vp, provisional[0].Event}} {
		events, err := store.ListEvents(interactionTestSource, c.v)
		if err != nil {
			t.Fatal(err)
		}
		if len(events) != 1 || !reflect.DeepEqual(events[0], c.want) {
			t.Fatalf("version %+v: %d events", c.v, len(events))
		}
	}
	if events, err := store.ListEvents("source/v1/other", v1); err != nil || len(events) != 0 {
		t.Fatalf("another source: %d events, %v", len(events), err)
	}
	if _, err := store.ListEvents(interactionTestSource, l8behaviour.InteractionVersion{}); err == nil {
		t.Fatal("an unspecified version was queried")
	}

	// Removing a superseded version takes its instants and windows with it
	// and leaves the others whole.
	n, err := store.DeleteVersion(interactionTestSource, v1)
	if err != nil || n != 1 {
		t.Fatalf("delete removed %d, %v", n, err)
	}
	var orphans int
	if err := db.QueryRow(`
		SELECT (SELECT COUNT(*) FROM lidar_interaction_instants WHERE event_id = ?)
		     + (SELECT COUNT(*) FROM lidar_exposure_windows WHERE event_id = ?)`,
		first[0].Event.EventID, first[0].Event.EventID).Scan(&orphans); err != nil || orphans != 0 {
		t.Fatalf("%d rows of the deleted version remain, %v", orphans, err)
	}
	if _, err := store.Get(second[0].Event.EventID); err != nil {
		t.Fatalf("a kept version is damaged: %v", err)
	}
}

// Denominators read observed windows only; predicted-only windows are stored
// beside them, labelled, and sum to the predicted-only time.
func TestInteractionStoreWindowsKeepBasis(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewInteractionStore(db)
	fi := analysedInteractions(t, l8behaviour.ScenarioOcclusion())[0]
	if err := store.Insert(fi); err != nil {
		t.Fatal(err)
	}
	v := fi.Event.Version.InteractionVersion()
	observed, err := store.ListWindows(interactionTestSource, v, l8behaviour.ExposureValidFollowing, l8behaviour.BasisObserved)
	if err != nil {
		t.Fatal(err)
	}
	predicted, err := store.ListWindows(interactionTestSource, v, l8behaviour.ExposureValidFollowing, l8behaviour.BasisPredictedOnly)
	if err != nil {
		t.Fatal(err)
	}
	acc := fi.Event.Accounting
	if len(observed) != 2 || len(predicted) != 1 ||
		l8behaviour.OpportunityNanos(observed) != acc.ValidNanos ||
		l8behaviour.OpportunityNanos(predicted) != 0 ||
		predicted[0].DurationNanos != acc.PredictedOnlyNanos ||
		l8behaviour.OpportunityNanos(append(observed, predicted...)) != acc.ValidNanos {
		t.Fatalf("observed %+v predicted %+v, accounting %+v", observed, predicted, acc)
	}
	var sum int64
	if err := db.QueryRow(`
		SELECT SUM(duration_nanos) FROM lidar_exposure_windows
		 WHERE source_id = ? AND basis = 'observed'`, interactionTestSource).Scan(&sum); err != nil || sum != acc.ValidNanos {
		t.Fatalf("SQL denominator %d, %v", sum, err)
	}
	if _, err := store.ListWindows(interactionTestSource, v, l8behaviour.ExposureKindUnspecified, l8behaviour.BasisObserved); err == nil {
		t.Fatal("an unspecified kind was queried")
	}
}

// The schema refuses what Validate refuses, for a writer that bypasses the
// store: a valid predicted-only instant, a window whose duration disagrees
// with its span, and a key that disagrees with its payload.
func TestInteractionSchemaRefusesPredictedOpportunity(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	fi := analysedInteractions(t, l8behaviour.ScenarioOcclusion())[0]
	if err := NewInteractionStore(db).Insert(fi); err != nil {
		t.Fatal(err)
	}
	id := fi.Event.EventID
	for name, stmt := range map[string][]any{
		"valid predicted-only instant": {`INSERT INTO lidar_interaction_instants (event_id, capture_unix_nanos, instant_json) VALUES (?, 1, ?)`,
			id, `{"event_id":"` + id + `","capture_unix_nanos":1,"basis":"predicted_only","valid":true}`},
		"instant of another event": {`INSERT INTO lidar_interaction_instants (event_id, capture_unix_nanos, instant_json) VALUES (?, 1, ?)`,
			id, `{"event_id":"interaction/v1/other","capture_unix_nanos":1,"basis":"observed","valid":false}`},
		"window duration disagrees with its span": {`INSERT INTO lidar_exposure_windows (window_id, event_id, window_json, inserted_at_ns) VALUES ('w', ?, ?, 1)`,
			id, `{"window_id":"w","event_id":"` + id + `","source_id":"s","kind":"valid_following","basis":"observed",` +
				`"track_id":"a","counterpart_track_id":"b","start_unix_nanos":10,"end_unix_nanos":20,"duration_nanos":5,` +
				`"version":{"estimate_stage":"final","estimator_id":"e","obs_model_id":"o","method_id":"m","param_hash":"p"}}`},
		"event id disagrees with its payload": {`INSERT INTO lidar_interaction_events (event_id, event_json, inserted_at_ns) VALUES ('x', ?, 1)`,
			strings.Replace(mustEventJSON(t, fi), id, "interaction/v1/y", 1)},
	} {
		if _, err := db.Exec(stmt[0].(string), stmt[1:]...); err == nil || !strings.Contains(err.Error(), "constraint") {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func mustEventJSON(t *testing.T, fi l8behaviour.FollowingInteraction) string {
	t.Helper()
	raw, _, _, err := encodeInteraction(fi)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

// A stored row edited by hand, or an invalid interaction offered for
// writing, fails loudly.
func TestInteractionStoreValidatesBothWays(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	store := NewInteractionStore(db)
	fi := analysedInteractions(t, l8behaviour.ScenarioSteadyApproach())[0]
	bad := fi
	bad.Windows = nil
	if err := store.Insert(bad); err == nil || !strings.Contains(err.Error(), "refuse to store") {
		t.Fatalf("invalid interaction: %v", err)
	}
	if countRows(t, db, "lidar_interaction_events") != 0 {
		t.Fatal("an invalid interaction wrote rows")
	}
	if err := store.Insert(fi); err != nil {
		t.Fatal(err)
	}
	// Valid JSON, satisfying every constraint, and inconsistent with the
	// instants stored under it.
	if _, err := db.Exec(`
		UPDATE lidar_interaction_events
		   SET event_json = JSON_SET(event_json, '$.accounting.valid_nanos', 1)
		 WHERE event_id = ?`, fi.Event.EventID); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(fi.Event.EventID); err == nil || !strings.Contains(err.Error(), "does not validate") {
		t.Fatalf("edited row read back: %v", err)
	}
	if _, err := store.ListEvents(interactionTestSource, fi.Event.Version.InteractionVersion()); err == nil {
		t.Fatal("edited row listed")
	}
}

// The persistence surface names nothing the registry does not know: every
// column is a structural name or a payload column, and every stored payload
// passes the same audit as the records.
func TestInteractionTablesUseRegisteredNames(t *testing.T) {
	db, cleanup := setupTestDB(t)
	defer cleanup()
	storageOnly := map[string]bool{"event_json": true, "instant_json": true, "window_json": true, "inserted_at_ns": true}
	declared := map[string]bool{}
	for _, n := range l8behaviour.SurfaceFieldNames() {
		declared[n] = true
	}
	for _, table := range []string{"lidar_interaction_events", "lidar_interaction_instants", "lidar_exposure_windows"} {
		rows, err := db.Query(`SELECT name FROM pragma_table_xinfo(?)`, table)
		if err != nil {
			t.Fatal(err)
		}
		var cols []string
		for rows.Next() {
			var c string
			if err := rows.Scan(&c); err != nil {
				t.Fatal(err)
			}
			cols = append(cols, c)
		}
		rows.Close()
		if len(cols) == 0 {
			t.Fatalf("table %s has no columns", table)
		}
		sort.Strings(cols)
		for _, c := range cols {
			if !declared[c] && !storageOnly[c] {
				t.Errorf("%s.%s is not a registered structural name", table, c)
			}
		}
	}

	fis := analysedInteractions(t, l8behaviour.ScenarioOcclusion())
	fis = append(fis, analysedInteractions(t, fixedLag(l8behaviour.ScenarioStandstillQueue()))...)
	if err := NewInteractionStore(db).Insert(fis...); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`SELECT event_json FROM lidar_interaction_events`,
		`SELECT instant_json FROM lidar_interaction_instants`,
		`SELECT window_json FROM lidar_exposure_windows`,
	} {
		payloads, err := readPayloads(db, q)
		if err != nil || len(payloads) == 0 {
			t.Fatalf("%s: %d rows, %v", q, len(payloads), err)
		}
		for _, raw := range payloads {
			if err := l8behaviour.AuditSurfaceJSON(raw); err != nil {
				t.Fatalf("%s: %v", q, err)
			}
		}
	}
}
