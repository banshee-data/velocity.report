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

// setupGeoDB applies the clip fixture plus migrations 045 and 046.
func setupGeoDB(t *testing.T) *sql.DB {
	t.Helper()
	db := setupClipFilesDB(t)
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

func TestSetClipLocationDerivesTheFamily(t *testing.T) {
	store := NewClipStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")

	loc, err := store.SetClipLocation("case-1", broadwayLat, broadwayLng, GeoSourceOperator)
	if err != nil {
		t.Fatalf("SetClipLocation: %v", err)
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

func TestSetClipLocationStoresTokensNotDisplays(t *testing.T) {
	// The guide forbids persisting the family display. A hyphen in a column
	// would mean two spellings of one cell and so two keys for one place.
	db := setupGeoDB(t)
	store := NewClipStore(db)
	insertCase(t, store, "case-1", "a.pcap")
	if _, err := store.SetClipLocation("case-1", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}

	var l10, l13, l16 string
	if err := db.QueryRow(`
		SELECT s2_l10_token, s2_l13_token, s2_l16_token
		  FROM lidar_clips WHERE clip_id = ?`, "case-1").
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

func TestSetClipLocationDefaultsTheSource(t *testing.T) {
	store := NewClipStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")
	loc, err := store.SetClipLocation("case-1", broadwayLat, broadwayLng, "")
	if err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}
	if loc.Source != GeoSourceOperator {
		t.Errorf("source = %q, want %q", loc.Source, GeoSourceOperator)
	}
}

func TestSetClipLocationRejectsNonPositions(t *testing.T) {
	store := NewClipStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")
	for _, tc := range []struct {
		name     string
		lat, lng float64
	}{
		{"past the pole", 91, 0},
		{"null island", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := store.SetClipLocation("case-1", tc.lat, tc.lng, ""); !errors.Is(err, geoindex.ErrNoPosition) {
				t.Fatalf("SetClipLocation = %v, want ErrNoPosition", err)
			}
		})
	}
}

func TestSetClipLocationOnAnUnknownCase(t *testing.T) {
	store := NewClipStore(setupGeoDB(t))
	if _, err := store.SetClipLocation("case-nope", broadwayLat, broadwayLng, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetClipLocation = %v, want ErrNotFound", err)
	}
}

func TestClipLocationRoundTrip(t *testing.T) {
	store := NewClipStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")

	// A case with no position is the ordinary state, not a defect.
	before, err := store.ClipLocationOf("case-1")
	if err != nil {
		t.Fatalf("ClipLocationOf: %v", err)
	}
	if before != nil {
		t.Errorf("an unlocated case reported a location: %+v", before)
	}

	set, err := store.SetClipLocation("case-1", broadwayLat, broadwayLng, GeoSourceSurveyed)
	if err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}
	got, err := store.ClipLocationOf("case-1")
	if err != nil {
		t.Fatalf("ClipLocationOf: %v", err)
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

func TestClearClipLocation(t *testing.T) {
	store := NewClipStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")
	if _, err := store.SetClipLocation("case-1", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}
	if err := store.ClearClipLocation("case-1"); err != nil {
		t.Fatalf("ClearClipLocation: %v", err)
	}
	got, err := store.ClipLocationOf("case-1")
	if err != nil {
		t.Fatalf("ClipLocationOf: %v", err)
	}
	if got != nil {
		t.Errorf("a cleared case still reports a location: %+v", got)
	}
	if err := store.ClearClipLocation("case-nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("clearing an unknown case = %v, want ErrNotFound", err)
	}
}

func TestDistrictsGroupsByCoarseCell(t *testing.T) {
	// The scene map's whole job: many visits to one junction collapse into one
	// site, and two junctions stay apart. Two junctions many km apart also
	// stay in separate areas.
	store := NewClipStore(setupGeoDB(t))
	for _, id := range []string{"case-1", "case-2", "case-3"} {
		insertCase(t, store, id, id+".pcap")
	}
	// Two at Broadway a few metres apart — one site, two visits — plus one
	// across town in its own area.
	if _, err := store.SetClipLocation("case-1", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}
	if _, err := store.SetClipLocation("case-2", broadwayLat+0.0001, broadwayLng, ""); err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}
	if _, err := store.SetClipLocation("case-3", kirkhamLat, kirkhamLng, ""); err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}

	areas, err := store.Districts()
	if err != nil {
		t.Fatalf("Districts: %v", err)
	}
	if len(areas) != 2 {
		t.Fatalf("got %d areas, want 2 junctions' worth", len(areas))
	}

	var broadway *District
	for i := range areas {
		if areas[i].ClipCount == 2 {
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
	if len(broadway.Sites[0].Clips) != 2 {
		t.Errorf("site lists %d cases, want 2", len(broadway.Sites[0].Clips))
	}
	// The cell's bounds must contain the positions it groups.
	if broadwayLat < broadway.SouthWestLat || broadwayLat > broadway.NorthEastLat {
		t.Errorf("area bounds do not contain the capture latitude")
	}
	if broadwayLng < broadway.SouthWestLon || broadwayLng > broadway.NorthEastLon {
		t.Errorf("area bounds do not contain the capture longitude")
	}
}

func TestDistrictsCountsDistinctSites(t *testing.T) {
	// An area's site count says how many distinct junctions it holds, which
	// is what distinguishes one visit repeated from two different places.
	store := NewClipStore(setupGeoDB(t))
	for _, id := range []string{"case-1", "case-2"} {
		insertCase(t, store, id, id+".pcap")
	}
	// The same position twice: one site, two visits.
	if _, err := store.SetClipLocation("case-1", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}
	if _, err := store.SetClipLocation("case-2", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}

	areas, err := store.Districts()
	if err != nil {
		t.Fatalf("Districts: %v", err)
	}
	if len(areas) != 1 {
		t.Fatalf("got %d areas, want 1", len(areas))
	}
	if areas[0].ClipCount != 2 {
		t.Errorf("case count = %d, want 2", areas[0].ClipCount)
	}
	if len(areas[0].Sites) != 1 {
		t.Errorf("site count = %d, want 1 for one junction visited twice", len(areas[0].Sites))
	}
	if areas[0].Sites[0].L16Token == "" {
		t.Error("site carries no L16 token")
	}
}

func TestDistrictsOmitsUnlocatedCases(t *testing.T) {
	store := NewClipStore(setupGeoDB(t))
	insertCase(t, store, "located", "a.pcap")
	insertCase(t, store, "unlocated", "b.pcap")
	if _, err := store.SetClipLocation("located", broadwayLat, broadwayLng, ""); err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}

	areas, err := store.Districts()
	if err != nil {
		t.Fatalf("Districts: %v", err)
	}
	if len(areas) != 1 || areas[0].ClipCount != 1 {
		t.Fatalf("got %+v, want only the located case", areas)
	}
	if areas[0].Sites[0].Clips[0].ClipID != "located" {
		t.Errorf("site lists %q, want the located case", areas[0].Sites[0].Clips[0].ClipID)
	}
}

func TestDistrictsOnAnEmptyDatabase(t *testing.T) {
	store := NewClipStore(setupGeoDB(t))
	areas, err := store.Districts()
	if err != nil {
		t.Fatalf("Districts: %v", err)
	}
	if len(areas) != 0 {
		t.Errorf("got %d areas, want none", len(areas))
	}
}

func TestSetClipLocationLinksTheCaseToItsSite(t *testing.T) {
	// "A scene should be at a site": locating a case must both create the
	// site (if this is the first visit) and record which one the case is at.
	store := NewClipStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")

	loc, err := store.SetClipLocation("case-1", broadwayLat, broadwayLng, "")
	if err != nil {
		t.Fatalf("SetClipLocation: %v", err)
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
	loc2, err := store.SetClipLocation("case-2", broadwayLat, broadwayLng, "")
	if err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}
	if loc2.SiteID != loc.SiteID {
		t.Errorf("second visit got site %q, want the same site %q", loc2.SiteID, loc.SiteID)
	}
}

func TestClearClipLocationLeavesTheSiteInPlace(t *testing.T) {
	// Unlocating a case must not delete a site other cases, or an
	// already-set canonical pose, may still depend on.
	store := NewClipStore(setupGeoDB(t))
	insertCase(t, store, "case-1", "a.pcap")
	loc, err := store.SetClipLocation("case-1", broadwayLat, broadwayLng, "")
	if err != nil {
		t.Fatalf("SetClipLocation: %v", err)
	}

	if err := store.ClearClipLocation("case-1"); err != nil {
		t.Fatalf("ClearClipLocation: %v", err)
	}

	got, err := store.ClipLocationOf("case-1")
	if err != nil {
		t.Fatalf("ClipLocationOf: %v", err)
	}
	if got != nil {
		t.Errorf("a cleared case still reports a location: %+v", got)
	}

	sites := NewSiteStore(store.db)
	if _, err := sites.ByToken(loc.SiteID); err != nil {
		t.Errorf("the site should survive clearing the case: %v", err)
	}
}

// setupHistoricalClipDB stops at the pre-geography schema for the migration test.
func setupHistoricalClipDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	files, err := filepath.Glob("../../../db/migrations/*.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range files {
		if filepath.Base(path)[:6] > "000044" {
			break
		}
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(string(sql)); err != nil {
			t.Fatal(err)
		}
	}
	return db
}
