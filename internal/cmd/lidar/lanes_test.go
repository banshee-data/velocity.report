//go:build pcap

package lidar

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	coredb "github.com/banshee-data/velocity.report/internal/db"
	sqlite "github.com/banshee-data/velocity.report/internal/lidar/storage/sqlite"
)

const pullGeoJSON = `{"type":"FeatureCollection","features":[
 {"type":"Feature","id":"gms:e","geometry":{"type":"LineString","coordinates":[[0,-2,21],[50,-2,21]]},
  "properties":{"layer":"street_use","id":"gms:e","kind":"travel_lane","row_id":"row1","author":"op",
   "street":"KIRKHAM ST","width_m":"3","speed_limit":"25","speed_limit_unit":"mph","speed_limit_source":"statutory",
   "speed_limit_effective_from":"2026-09-02"}}]}`

func TestLanesPullWritesASnapshotOnce(t *testing.T) {
	gerardus := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/sites/808580f47/geojson" || r.URL.Query().Get("layers") != "street_use" || r.URL.Query().Get("frame") != "site" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(pullGeoJSON))
	}))
	defer gerardus.Close()
	path := filepath.Join(t.TempDir(), "sensor.db")
	d, err := coredb.NewDB(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`INSERT INTO lidar_sites (site_id, s2_l13_token, s2_l10_token, created_at_ns) VALUES ('808580f47', '808580f4', '80858', 1)`); err != nil {
		t.Fatal(err)
	}
	d.Close()
	args := []string{"pull", "--db", path, "--site", "808580f47", "--sensor-id", "s1", "--record-id", "gms:rec",
		"--pose", "0,0,0,1,0,0,0", "--gerardus", gerardus.URL, "--author", "test"}
	if code := LanesMain(args); code != 0 {
		t.Fatalf("pull: exit %d", code)
	}
	if code := LanesMain(args); code != 0 {
		t.Fatalf("pull again: exit %d", code)
	}
	d, _ = coredb.NewDB(path)
	defer d.Close()
	all, err := sqlite.NewLaneSnapshotStore(d).All("808580f47", "s1")
	if err != nil || len(all) != 1 {
		t.Fatalf("snapshots: %v %d (unchanged street use writes once)", err, len(all))
	}
	s := all[0]
	if s.Writer != "pull" || len(s.Lanes) != 1 || s.Lanes[0].Limit.Value != 25 || s.Directions[0].Name != "Kirkham St eastbound" {
		t.Fatalf("snapshot %+v", s)
	}
	for _, bad := range [][]string{
		{"pull", "--db", path},
		{"pull", "--db", path, "--site", "808580f47", "--sensor-id", "s1", "--record-id", "r", "--pose", "1,2"},
		{"pull", "--db", path, "--site", "nowhere", "--sensor-id", "s1", "--record-id", "r", "--pose", "0,0,0,1,0,0,0", "--gerardus", gerardus.URL},
		{"push"},
	} {
		if code := LanesMain(bad); code == 0 {
			t.Errorf("%v: exit 0", bad)
		}
	}
}
