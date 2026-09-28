package segments

import (
	"encoding/json"
	"fmt"
	"testing"

	dbpkg "github.com/banshee-data/velocity.report/internal/db"
)

// The database holds the finder catalogue and the held-out rule as CHECK
// constraints, which a migration has to change when the catalogue does. This
// test is what says so: it fails when Finders or Allowed says one thing and
// the schema another, in either direction.
func TestTheSchemaHoldsTheFinderCatalogue(t *testing.T) {
	db, cleanup := dbpkg.NewTestDB(t)
	defer cleanup()
	if _, err := db.Exec(`INSERT INTO lidar_run_records(run_id,created_at,source_type,sensor_id,status) VALUES('run',1,'pcap','sensor','completed')`); err != nil {
		t.Fatal(err)
	}
	finders := []string{"manual", "unknown", ""}
	for _, f := range Finders() {
		finders = append(finders, f.Name)
	}
	roles := []string{"tuning", "held_out", "validation", ""}
	chosen := 0
	for _, finder := range finders {
		for _, role := range roles {
			name := fmt.Sprintf("%s as %s", finder, role)
			chosen++
			replayCase := fmt.Sprintf("case-%d", chosen)
			if _, err := db.Exec(`INSERT INTO lidar_replay_cases(replay_case_id,sensor_id,pcap_file,created_at_ns) VALUES(?,'sensor','a.pcap',1)`, replayCase); err != nil {
				t.Fatal(err)
			}
			p := DefaultParams()
			window := Window{
				ID: Identity(finder, "run", role, p, testBase), Finder: finder, Version: Version, Source: "run", Role: role,
				StartNs: testBase, EndNs: testBase + int64(p.WindowSeconds*1e9), PeakNs: testBase,
				Capture: "/captures/a.pcap", Status: "candidate",
			}
			// A window with no role or finder omits the field, as the
			// encoder does, and the schema must refuse that as well.
			document, err := json.Marshal(window)
			if err != nil {
				t.Fatal(err)
			}
			parameters, err := json.Marshal(p)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(`INSERT INTO lidar_segment_selections(segment_id,run_id,replay_case_id,parameters_json,window_json,created_at_ns) VALUES(?,?,?,?,?,1)`,
				window.ID, "run", replayCase, string(parameters), string(document))
			if accepted, allowed := err == nil, Allowed(finder, role); accepted != allowed {
				t.Errorf("%s: the schema accepted it: %t; Allowed says %t (%v)", name, accepted, allowed, err)
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
