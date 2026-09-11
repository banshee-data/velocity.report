package sqlite

import (
	"errors"
	"testing"
)

func TestSetCanonicalPoseOnAnUnknownSite(t *testing.T) {
	// A site only exists once a case has visited it; setting a canonical pose
	// on a token nothing has located yet must not silently create one.
	sites := NewSiteStore(setupGeoDB(t))
	if _, err := sites.SetCanonicalPose("nope", broadwayLat, broadwayLng, SiteSourceOperator, ""); !errors.Is(err, ErrNotFound) {
		t.Fatalf("SetCanonicalPose = %v, want ErrNotFound", err)
	}
}

func TestSetCanonicalPoseRejectsAnUnknownSource(t *testing.T) {
	db := setupGeoDB(t)
	store := NewReplayCaseStore(db)
	insertCase(t, store, "case-1", "a.pcap")
	loc, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, "")
	if err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}

	sites := NewSiteStore(db)
	if _, err := sites.SetCanonicalPose(loc.SiteID, broadwayLat, broadwayLng, "fix", ""); err == nil {
		t.Fatal("SetCanonicalPose accepted \"fix\", which is a case provenance, not a site one")
	}
}

func TestSetCanonicalPoseRoundTrip(t *testing.T) {
	// The midpoint of the intersection, distinct from any one visit's own
	// sensor pose — the whole point of separating the two.
	const midpointLat, midpointLon = 37.79875, -122.40735

	db := setupGeoDB(t)
	store := NewReplayCaseStore(db)
	insertCase(t, store, "case-1", "a.pcap")
	loc, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, "")
	if err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}

	sites := NewSiteStore(db)
	updated, err := sites.SetCanonicalPose(loc.SiteID, midpointLat, midpointLon, SiteSourceSurveyed, "Broadway & Columbus")
	if err != nil {
		t.Fatalf("SetCanonicalPose: %v", err)
	}
	if updated.CanonicalLat == nil || *updated.CanonicalLat != midpointLat {
		t.Errorf("canonical lat = %v, want %v", updated.CanonicalLat, midpointLat)
	}
	if updated.CanonicalSource != SiteSourceSurveyed {
		t.Errorf("canonical source = %q, want %q", updated.CanonicalSource, SiteSourceSurveyed)
	}
	if updated.Label != "Broadway & Columbus" {
		t.Errorf("label = %q, want the given name", updated.Label)
	}

	// The case's own sensor pose is untouched by the site's canonical one.
	again, err := store.CaseLocationOf("case-1")
	if err != nil {
		t.Fatalf("CaseLocationOf: %v", err)
	}
	if again.Lat != broadwayLat || again.Lon != broadwayLng {
		t.Errorf("case sensor pose changed to %v,%v, want the original fix", again.Lat, again.Lon)
	}
}

func TestSetCanonicalPoseWithNoLabelLeavesTheExistingOne(t *testing.T) {
	db := setupGeoDB(t)
	store := NewReplayCaseStore(db)
	insertCase(t, store, "case-1", "a.pcap")
	loc, err := store.SetCaseLocation("case-1", broadwayLat, broadwayLng, "")
	if err != nil {
		t.Fatalf("SetCaseLocation: %v", err)
	}

	sites := NewSiteStore(db)
	if _, err := sites.SetCanonicalPose(loc.SiteID, broadwayLat, broadwayLng, SiteSourceOperator, "Broadway & Columbus"); err != nil {
		t.Fatalf("SetCanonicalPose (with label): %v", err)
	}
	updated, err := sites.SetCanonicalPose(loc.SiteID, broadwayLat+0.00002, broadwayLng, SiteSourceOperator, "")
	if err != nil {
		t.Fatalf("SetCanonicalPose (no label): %v", err)
	}
	if updated.Label != "Broadway & Columbus" {
		t.Errorf("label = %q, want the previously set name to survive", updated.Label)
	}
}

// TestMigration046BackfillsSitesForAlreadyLocatedCases guards the upgrade
// path: a deployment that already has located replay cases from migration 45
// must not lose that data when 46 introduces sites — every located case
// should come out the other side naming a site.
func TestMigration046BackfillsSitesForAlreadyLocatedCases(t *testing.T) {
	db := setupCaseFilesDB(t)
	applyMigrationScript(t, db, "000045_replay_case_geography.up.sql")

	// Simulate a pre-46 deployment: located via raw SQL, exactly the shape
	// SetCaseLocation wrote before site_id existed.
	store := NewReplayCaseStore(db)
	insertCase(t, store, "case-1", "a.pcap")
	if _, err := db.Exec(`
		UPDATE lidar_replay_cases
		   SET origin_lat = ?, origin_lon = ?,
		       s2_l10_token = '808581', s2_l13_token = '80858004', s2_l16_token = '8085800c',
		       geographic_source = 'operator', geographic_status = 'located'
		 WHERE replay_case_id = 'case-1'`, broadwayLat, broadwayLng); err != nil {
		t.Fatalf("simulate pre-43 located case: %v", err)
	}

	applyMigrationScript(t, db, "000046_lidar_sites.up.sql")

	var siteID string
	if err := db.QueryRow(`SELECT site_id FROM lidar_replay_cases WHERE replay_case_id = 'case-1'`).
		Scan(&siteID); err != nil {
		t.Fatalf("read back site_id: %v", err)
	}
	if siteID != "8085800c" {
		t.Errorf("backfilled site_id = %q, want the case's own L16 token", siteID)
	}

	sites := NewSiteStore(db)
	site, err := sites.ByToken("8085800c")
	if err != nil {
		t.Fatalf("the backfill should have created the site: %v", err)
	}
	if site.L13Token != "80858004" || site.L10Token != "808581" {
		t.Errorf("backfilled site ancestors %+v do not match the case's tokens", site)
	}
}
