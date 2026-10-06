package segments

import (
	"encoding/json"
	"fmt"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

// schemaFixture is a database with one run, for rows that name it.
func schemaFixture(t *testing.T) *dbpkg.DB {
	t.Helper()
	db, cleanup := dbpkg.NewTestDB(t)
	t.Cleanup(cleanup)
	if _, err := db.Exec(`INSERT INTO lidar_run_records(run_id,created_at,source_type,sensor_id,status) VALUES('run',1,'pcap','sensor','completed')`); err != nil {
		t.Fatal(err)
	}
	return db
}

// insertChoice stores a window chosen by a finder for a role, with the
// selector that chose it, the way the segments API does, and says whether
// the schema accepted it.
func insertChoice(t *testing.T, db *dbpkg.DB, n int, finder, role string, chosenBy SelectorProvenance) error {
	t.Helper()
	replayCase := fmt.Sprintf("case-%d", n)
	if _, err := db.Exec(`INSERT INTO lidar_replay_cases(replay_case_id,sensor_id,pcap_file,created_at_ns) VALUES(?,'sensor','a.pcap',1)`, replayCase); err != nil {
		t.Fatal(err)
	}
	p := chosenBy.Parameters
	window := Window{
		ID: Identity(finder, "run", role, p, testBase), Finder: finder, Version: Version, Source: "run", Role: role,
		StartNs: testBase, EndNs: testBase + int64(p.WindowSeconds*1e9), PeakNs: testBase,
		Capture: "/captures/a.pcap", Status: "candidate",
	}
	// A window with no role or finder omits the field, as the encoder does,
	// and the schema must refuse that as well.
	document, err := json.Marshal(window)
	if err != nil {
		t.Fatal(err)
	}
	parameters, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	selector, err := json.Marshal(chosenBy)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`INSERT INTO lidar_segment_selections(segment_id,run_id,replay_case_id,parameters_json,window_json,selector_json,created_at_ns) VALUES(?,?,?,?,?,?,1)`,
		window.ID, "run", replayCase, string(parameters), string(document), string(selector))
	return err
}

// The database holds the finder catalogue and the held-out rule as CHECK
// constraints, which a migration has to change when the catalogue does. This
// test is what says so: it fails when Finders or Allowed says one thing and
// the schema another, in either direction.
func TestTheSchemaHoldsTheFinderCatalogue(t *testing.T) {
	db := schemaFixture(t)
	c := shippedCatalogue(t)
	finders := []string{"manual", "unknown", ""}
	for _, f := range Finders() {
		finders = append(finders, f.Name)
	}
	roles := []string{"tuning", "held_out", "validation", ""}
	chosen := 0
	for _, finder := range finders {
		// The finder's standard selector where it has one, and otherwise a
		// selector that ran the finder named, so that only the finder and the
		// role decide.
		chosenBy := SelectorProvenance{ID: "other", Digest: "sha256:" + fmt.Sprintf("%064d", 0), HeldOutEligible: true, Finder: finder, Parameters: DefaultParams()}
		if s, ok := c.Selector(finder); ok {
			chosenBy = s.Provenance()
		}
		for _, role := range roles {
			chosen++
			err := insertChoice(t, db, chosen, finder, role, chosenBy)
			if accepted, allowed := err == nil, Allowed(finder, role); accepted != allowed {
				t.Errorf("%s as %s: the schema accepted it: %t; Allowed says %t (%v)", finder, role, accepted, allowed, err)
			}
		}
	}
	// Every finder in the catalogue may choose a tuning window, so none of
	// them can have been left out of the schema unnoticed.
	var stored int
	if err := db.QueryRow(`SELECT COUNT(DISTINCT finder) FROM lidar_segment_selections WHERE role='tuning'`).Scan(&stored); err != nil || stored != len(Finders()) {
		t.Fatalf("%d of %d finders could be stored: %v", stored, len(Finders()), err)
	}
	// What the finder returns is what the schema reads its columns from.
	var finder, role, capture string
	var version int
	var start, end int64
	if err := db.QueryRow(`SELECT finder,role,finder_version,capture,window_start_ns,window_end_ns FROM lidar_segment_selections WHERE finder='random' AND role='held_out'`).
		Scan(&finder, &role, &version, &capture, &start, &end); err != nil {
		t.Fatal(err)
	}
	if version != Version || capture != "/captures/a.pcap" || start != testBase || end-start != int64(DefaultParams().WindowSeconds*1e9) {
		t.Fatalf("columns read from the window: version %d capture %q [%d, %d]", version, capture, start, end)
	}
}

// The schema holds the selector rule too: a held-out window is stored only
// when the selector that chose it was eligible, as HeldOut decides. The test
// fails when the two disagree in either direction.
func TestTheSchemaHoldsTheSelectorRule(t *testing.T) {
	db := schemaFixture(t)
	c := shippedCatalogue(t)
	selectors := append([]Selector(nil), c.Selectors...)
	for _, change := range []func(*Selector){
		func(s *Selector) { s.ID = "wide_following"; s.Parameters.WindowSeconds = 20 },
		func(s *Selector) {
			s.ID = "fast_exposure"
			s.Finder, s.Score = "exposure", Score{"events", "descending"}
			s.Parameters.MinSpeed = 5
		},
		func(s *Selector) {
			s.ID = "reseeded"
			s.Finder, s.Score = "random", Score{"draw", "descending"}
			s.Parameters.RandomSeed = 2
		},
	} {
		s := selectorFrom(t, c, "following")
		change(&s)
		selectors = append(selectors, s)
	}
	chosen := 0
	for _, s := range selectors {
		for _, role := range []string{"tuning", "held_out"} {
			chosen++
			err := insertChoice(t, db, chosen, s.Finder, role, s.Provenance())
			want := Allowed(s.Finder, role) && (role == "tuning" || s.HeldOut())
			if accepted := err == nil; accepted != want {
				t.Errorf("%s as %s: the schema accepted it: %t; the code says %t (%v)", s.ID, role, accepted, want, err)
			}
		}
	}
	// A window already chosen keeps the selector that chose it.
	if _, err := db.Exec(`UPDATE lidar_segment_selections SET selector_json = NULL`); err == nil {
		t.Fatal("a selection's selector was erased")
	}
	var row *string
	if err := db.QueryRow(`SELECT selector_json FROM lidar_segment_selections WHERE JSON_EXTRACT(selector_json, '$.id') = 'close_following'`).Scan(&row); err != nil || row == nil {
		t.Fatalf("close_following's tuning window: %v", err)
	}
}
