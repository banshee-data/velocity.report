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

// setupGeoDB applies the replay-case fixture plus migrations 042 and 043.
func setupGeoDB(t *testing.T) *sql.DB {
	t.Helper()
	db := setupCaseFilesDB(t)
	applyMigrationScript(t, db, "000042_replay_case_geography.up.sql")
	applyMigrationScript(t, db, "000043_lidar_sites.up.sql")
	return db
}

// applyMigrationScript applies one migration file strictly, as one script,
// exactly as the migration runner does.
//
// An earlier version of this fixture split a migration and tolerated
// "duplicate column" errors, and that tolerance hid a genuine fault: the SQL
// formatter had mangled a comment into a stray comma, and the shipped
// migration would not apply to any database. Being lenient here meant the
// test passed while the product was broken.
func applyMigrationScript(t *testing.T, db *sql.DB, filename string) {
	t.Helper()
	path := filepath.Join("..", "..", "..", "db", "migrations", filename)
	schema, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read migration %s: %v", filename, err)
	}
	if _, err := db.Exec(string(schema)); err != nil {
		t.Fatalf("apply migration %s: %v", filename, err)
	}
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

func TestSceneAreasGroupsByCoarseCell(t *testing.T) {
	// The scene map's whole job: many visits to one junction collapse into one
	// site, and two junctions stay apart. Two junctions many km apart also
	// stay in separate areas.
	store := NewReplayCaseStore(setupGeoDB(t))
	for _, id := range []string{"case-1", "case-2", "case-3"} {
		insertCase(t, store, id, id+".pcap")
	}
	// Two at Broadway a few metres apart — one site, two visits — plus one
	// across town in its own area.
	if _, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}
	if _, err := store.SetCaseLocation("case-2", broadwayLat+0.0001, broadwayLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}
	if _, err := store.SetCaseLocation("case-3", kirkhamLat, kirkhamLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}

	areas, err := store.SceneAreas()
	if err != nil {
		t.Fatalf("SceneAreas: %v", err)
	}
	if len(areas) != 2 {
		t.Fatalf("got %d areas, want 2 junctions' worth", len(areas))
	}

	var broadway *SceneArea
	for i := range areas {
		if areas[i].CaseCount == 2 {
			broadway = &areas[i]
		}
	}
	if broadway == nil {
		t.Fatal("no area holds the two Broadway cases")
	}
	if broadway.L10Display == "" || !strings.Contains(broadway.L10Display, "-") {
		t.Errorf("area display %q is not a family display", broadway.L10Display)
	}
	if len(broadway.Sites) != 1 {
		t.Fatalf("area holds %d sites, want 1 — the two cases are metres apart", len(broadway.Sites))
	}
	if len(broadway.Sites[0].Cases) != 2 {
		t.Errorf("site lists %d cases, want 2", len(broadway.Sites[0].Cases))
	}
	// The cell's bounds must contain the positions it groups.
	if broadwayLat < broadway.SouthWestLat || broadwayLat > broadway.NorthEastLat {
		t.Errorf("area bounds do not contain the capture latitude")
	}
	if broadwayLng < broadway.SouthWestLon || broadwayLng > broadway.NorthEastLon {
		t.Errorf("area bounds do not contain the capture longitude")
	}
}

func TestSceneAreasCountsDistinctSites(t *testing.T) {
	// An area's site count says how many distinct junctions it holds, which
	// is what distinguishes one visit repeated from two different places.
	store := NewReplayCaseStore(setupGeoDB(t))
	for _, id := range []string{"case-1", "case-2"} {
		insertCase(t, store, id, id+".pcap")
	}
	// The same position twice: one site, two visits.
	if _, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}
	if _, err := store.SetCaseLocation("case-2", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}

	areas, err := store.SceneAreas()
	if err != nil {
		t.Fatalf("SceneAreas: %v", err)
	}
	if len(areas) != 1 {
		t.Fatalf("got %d areas, want 1", len(areas))
	}
	if areas[0].CaseCount != 2 {
		t.Errorf("case count = %d, want 2", areas[0].CaseCount)
	}
	if len(areas[0].Sites) != 1 {
		t.Errorf("site count = %d, want 1 for one junction visited twice", len(areas[0].Sites))
	}
	if areas[0].Sites[0].L16Token == "" {
		t.Error("site carries no L16 token")
	}
}

func TestSceneAreasOmitsUnlocatedCases(t *testing.T) {
	store := NewReplayCaseStore(setupGeoDB(t))
	insertCase(t, store, "located", "a.pcap")
	insertCase(t, store, "unlocated", "b.pcap")
	if _, err := store.SetCaseLocation("located", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}

	areas, err := store.SceneAreas()
	if err != nil {
		t.Fatalf("SceneAreas: %v", err)
	}
	if len(areas) != 1 || areas[0].CaseCount != 1 {
		t.Fatalf("got %+v, want only the located case", areas)
	}
	if areas[0].Sites[0].Cases[0].ReplayCaseID != "located" {
		t.Errorf("site lists %q, want the located case", areas[0].Sites[0].Cases[0].ReplayCaseID)
	}
}

func TestSceneAreasOnAnEmptyDatabase(t *testing.T) {
	store := NewReplayCaseStore(setupGeoDB(t))
	areas, err := store.SceneAreas()
	if err != nil {
		t.Fatalf("SceneAreas: %v", err)
	}
	if len(areas) != 0 {
		t.Errorf("got %d areas, want none", len(areas))
	}
}

func TestSetCaseLocationLinksTheCaseToItsSite(t *testing.T) {
	// "A scene should be at a site": locating a case must both create the
	// site (if this is the first visit) and record which one the case is at.
	store := NewReplayCaseStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")

	loc, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, "")
	if err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}
	if loc.SiteID != loc.L16Token {
		t.Errorf("location site_id = %q, want the L16 token %q", loc.SiteID, loc.L16Token)
	}

	sites := NewSiteStore(store.db)
	site, err := sites.ByToken(loc.L16Token)
	if err != nil {
		t.Fatalf("ByToken: %v", err)
	}
	if site.L13Token != loc.L13Token || site.L10Token != loc.L10Token {
		t.Errorf("site ancestors %+v do not match the case's own tokens", site)
	}
	if site.CanonicalLat != nil {
		t.Error("a freshly created site should have no canonical pose yet")
	}

	// A second case at the same junction shares the site rather than
	// duplicating it.
	insertCase(t, store, "case-2", "b.pcap")
	loc2, err := store.SetCaseLocation("case-2", broadwayLat, broadwayLng, "")
	if err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}
	if loc2.SiteID != loc.SiteID {
		t.Errorf("second visit got site %q, want the same site %q", loc2.SiteID, loc.SiteID)
	}
}

func TestClearCaseLocationLeavesTheSiteInPlace(t *testing.T) {
	// Unlocating a case must not delete a site other cases, or an
	// already-set canonical pose, may still depend on.
	store := NewReplayCaseStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")
	loc, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, "")
	if err != nil {
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

	sites := NewSiteStore(store.db)
	if _, err := sites.ByToken(loc.SiteID); err != nil {
		t.Errorf("the site should survive clearing the case: %v", err)
	}
}
