package sqlite

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/banshee-data/velocity.report/internal/lidar/geoindex"
)

// Broadway & Columbus and a second junction a few blocks away.
const (
	broadwayLat = 37.7987
	broadwayLng = -122.4073
	kirkhamLat  = 37.7601
	kirkhamLng  = -122.4694
)

// setupGeoDB applies the replay-case fixture plus migration 042.
func setupGeoDB(t *testing.T) *sql.DB {
	t.Helper()
	db := setupCaseFilesDB(t)
	migration := filepath.Join("..", "..", "..", "db", "migrations",
		"000042_replay_case_geography.up.sql")
	schema, err := os.ReadFile(migration)
	if err != nil {
		t.Fatalf("read migration 042: %v", err)
	}
	for _, stmt := range splitSQLStatements(string(schema)) {
		if _, err := db.Exec(stmt); err != nil &&
			!strings.Contains(err.Error(), "duplicate column") &&
			!strings.Contains(err.Error(), "already exists") {
			t.Fatalf("apply migration 042 statement %q: %v", firstLine(stmt), err)
		}
	}
	return db
}

func TestSetCaseLocationDerivesTheFamily(t *testing.T) {
	store := NewReplayCaseStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")

	loc, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, GeoSourceOperator)
	if err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}
	if loc.Status != GeoLocated {
		t.Errorf("status = %q, want %q", loc.Status, GeoLocated)
	}

	// The stored identity is the canonical tokens, and they must be one family.
	if err := geoindex.Validate(geoindex.Tokens{
		Coarse: loc.L10Token, Fine: loc.L13Token, Precise: loc.L16Token,
	}); err != nil {
		t.Errorf("stored tokens are not one family: %v", err)
	}

	// The displays are derived presentation, in the guide's 5+n grouping.
	for _, tc := range []struct{ token, display string }{
		{loc.L10Token, loc.L10Display},
		{loc.L13Token, loc.L13Display},
		{loc.L16Token, loc.L16Display},
	} {
		if tc.display != geoindex.FamilyDisplay(tc.token) {
			t.Errorf("display %q does not match FamilyDisplay(%q)", tc.display, tc.token)
		}
		if !strings.Contains(tc.display, "-") {
			t.Errorf("display %q carries no family hyphen", tc.display)
		}
	}
}

func TestSetCaseLocationStoresTokensNotDisplays(t *testing.T) {
	// The guide forbids persisting the family display. A hyphen in a column
	// would mean two spellings of one cell and so two keys for one place.
	db := setupGeoDB(t)
	store := NewReplayCaseStore(db)
	insertCase(t, store, "case-1", "a.pcap")
	if _, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}

	var l10, l13, l16 string
	if err := db.QueryRow(`
		SELECT s2_l10_token, s2_l13_token, s2_l16_token
		  FROM lidar_replay_cases WHERE replay_case_id = ?`, "case-1").
		Scan(&l10, &l13, &l16); err != nil {
		t.Fatalf("read back tokens: %v", err)
	}
	for _, token := range []string{l10, l13, l16} {
		if strings.Contains(token, "-") {
			t.Errorf("stored %q, which is a family display rather than a token", token)
		}
		if _, err := geoindex.ParseToken(token); err != nil {
			t.Errorf("stored %q is not a canonical token: %v", token, err)
		}
	}
}

func TestSetCaseLocationDefaultsTheSource(t *testing.T) {
	store := NewReplayCaseStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")
	loc, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, "")
	if err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}
	if loc.Source != GeoSourceOperator {
		t.Errorf("source = %q, want %q", loc.Source, GeoSourceOperator)
	}
}

func TestSetCaseLocationRejectsNonPositions(t *testing.T) {
	store := NewReplayCaseStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")
	for _, tc := range []struct {
		name     string
		lat, lng float64
	}{
		{"past the pole", 91, 0},
		{"null island", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.SetCaseLocation("case-1", tc.lat, tc.lng, ""); !errors.Is(err, geoindex.ErrNoPosition) {
				t.Fatalf("SetCaseLocation = %v, want ErrNoPosition", err)
			}
		})
	}
}

func TestSetCaseLocationOnAnUnknownCase(t *testing.T) {
	store := NewReplayCaseStore(setupGeoDB(t))
	if _, err := store.SetCaseLocation("case-nope", broadwayLat, broadwayLng, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetCaseLocation = %v, want ErrNotFound", err)
	}
}

func TestCaseLocationRoundTrip(t *testing.T) {
	store := NewReplayCaseStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")

	// A case with no position is the ordinary state, not a defect.
	before, err := store.CaseLocationOf("case-1")
	if err != nil {
		t.Fatalf("CaseLocationOf: %v", err)
	}
	if before != nil {
		t.Errorf("an unlocated case reported a location: %+v", before)
	}

	set, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, GeoSourceSurveyed)
	if err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}
	got, err := store.CaseLocationOf("case-1")
	if err != nil {
		t.Fatalf("CaseLocationOf: %v", err)
	}
	if got == nil {
		t.Fatal("a located case reported no location")
	}
	if got.L16Token != set.L16Token || got.Source != GeoSourceSurveyed {
		t.Errorf("read back %+v, want the values written", got)
	}
	if got.L10Display == "" {
		t.Error("the family display was not derived on read")
	}
}

func TestClearCaseLocation(t *testing.T) {
	store := NewReplayCaseStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")
	if _, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}
	if err := store.ClearCaseLocation("case-1"); err != nil {
		t.Fatalf("ClearCaseLocation: %v", err)
	}
	got, err := store.CaseLocationOf("case-1")
	if err != nil {
		t.Fatalf("CaseLocationOf: %v", err)
	}
	if got != nil {
		t.Errorf("a cleared case still reports a location: %+v", got)
	}
	if err := store.ClearCaseLocation("case-nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("clearing an unknown case = %v, want ErrNotFound", err)
	}
}

func TestSceneSitesGroupsByCoarseCell(t *testing.T) {
	// The scene map's whole job: many visits to one junction collapse into one
	// site, and two junctions stay apart.
	store := NewReplayCaseStore(setupGeoDB(t))
	for _, id := range []string{"case-1", "case-2", "case-3"} {
		insertCase(t, store, id, id+".pcap")
	}
	// Two at Broadway a few metres apart, one across town.
	if _, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}
	if _, err := store.SetCaseLocation("case-2", broadwayLat+0.0001, broadwayLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}
	if _, err := store.SetCaseLocation("case-3", kirkhamLat, kirkhamLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}

	sites, err := store.SceneSites()
	if err != nil {
		t.Fatalf("SceneSites: %v", err)
	}
	if len(sites) != 2 {
		t.Fatalf("got %d sites, want 2 junctions", len(sites))
	}

	var broadway *SceneSite
	for i := range sites {
		if sites[i].CaseCount == 2 {
			broadway = &sites[i]
		}
	}
	if broadway == nil {
		t.Fatal("no site holds the two Broadway cases")
	}
	if broadway.L10Display == "" || !strings.Contains(broadway.L10Display, "-") {
		t.Errorf("site display %q is not a family display", broadway.L10Display)
	}
	if len(broadway.Cases) != 2 {
		t.Errorf("site lists %d cases, want 2", len(broadway.Cases))
	}
	// The cell's bounds must contain the positions it groups.
	if broadwayLat < broadway.SouthWestLat || broadwayLat > broadway.NorthEastLat {
		t.Errorf("site bounds do not contain the capture latitude")
	}
	if broadwayLng < broadway.SouthWestLon || broadwayLng > broadway.NorthEastLon {
		t.Errorf("site bounds do not contain the capture longitude")
	}
}

func TestSceneSitesCountsDistinctFinerCells(t *testing.T) {
	// A site's finer counts say how many deployments and sensor positions it
	// holds, which is what distinguishes one visit from ten.
	store := NewReplayCaseStore(setupGeoDB(t))
	for _, id := range []string{"case-1", "case-2"} {
		insertCase(t, store, id, id+".pcap")
	}
	// The same position twice: one deployment, one sensor position.
	if _, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}
	if _, err := store.SetCaseLocation("case-2", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}

	sites, err := store.SceneSites()
	if err != nil {
		t.Fatalf("SceneSites: %v", err)
	}
	if len(sites) != 1 {
		t.Fatalf("got %d sites, want 1", len(sites))
	}
	if sites[0].CaseCount != 2 {
		t.Errorf("case count = %d, want 2", sites[0].CaseCount)
	}
	if sites[0].L16Count != 1 {
		t.Errorf("precise cell count = %d, want 1 for one sensor position", sites[0].L16Count)
	}
}

func TestSceneSitesOmitsUnlocatedCases(t *testing.T) {
	store := NewReplayCaseStore(setupGeoDB(t))
	insertCase(t, store, "located", "a.pcap")
	insertCase(t, store, "unlocated", "b.pcap")
	if _, err := store.SetCaseLocation("located", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}

	sites, err := store.SceneSites()
	if err != nil {
		t.Fatalf("SceneSites: %v", err)
	}
	if len(sites) != 1 || sites[0].CaseCount != 1 {
		t.Fatalf("got %+v, want only the located case", sites)
	}
	if sites[0].Cases[0].ReplayCaseID != "located" {
		t.Errorf("site lists %q, want the located case", sites[0].Cases[0].ReplayCaseID)
	}
}

func TestSceneSitesOnAnEmptyDatabase(t *testing.T) {
	store := NewReplayCaseStore(setupGeoDB(t))
	sites, err := store.SceneSites()
	if err != nil {
		t.Fatalf("SceneSites: %v", err)
	}
	if len(sites) != 0 {
		t.Errorf("got %d sites, want none", len(sites))
	}
}
