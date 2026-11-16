package eligibility

import (
	"path/filepath"
	"testing"
)

// The committed cohort manifest (D-40) must load: a malformed one would refuse
// every development use of the captures it assigns.
func TestRepositoryCohortManifestLoads(t *testing.T) {
	if _, err := LoadManifest(filepath.Join("..", "..", "..", "config", "cohorts", "cohort-manifest.json")); err != nil {
		t.Fatal(err)
	}
}

func TestCurrentFullReviewCapturesRetainTheirSiteCohorts(t *testing.T) {
	m, err := LoadManifest(filepath.Join("..", "..", "..", "config", "cohorts", "cohort-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		site, sha, name string
		cohort          Cohort
	}{
		{"ashbury-downey", "sha256:2eec35d381fa4c4ba5a6bf1218982c76e53749287d53691115261f14d8aefbed", "s2-sf-0.pcapng", CohortDevelopment},
		{"columbus-broadway", "sha256:aa69875be4dd45094d79c371ca25e434b3a4d8bec0966c116931720cc1653b66", "20260902T1320_80858-0f4_columbus-broadway.pcapng", CohortDevelopment},
		{"embarcadero-bryant", "sha256:03e10c86b718f068c8733559b1020b56a4e7ae1ec0361529f4fb19e75090fdf3", "20260903T1440_80858-074_embarcadero-bryant.pcapng", CohortDevelopment},
		{"lombard-broderick", "sha256:1eb08d9e6ced4e20bf07db2140434fbff314b779beb909d602929d44b4c34643", "20260902T0938_80858-0d4_lombard-broderick.pcapng", CohortValidation},
	} {
		a := m.Capture(c.sha, c.name)
		if a.SiteID != c.site || a.Cohort != c.cohort || a.Conflict != "" {
			t.Fatalf("%s: %+v", c.site, a)
		}
		if renamed := m.Capture(c.sha, "renamed.pcapng"); renamed != a {
			t.Fatalf("renaming changed the byte identity: %+v", renamed)
		}
		if other := m.Capture("sha256:0000000000000000000000000000000000000000000000000000000000000000", c.name); other.Conflict == "" {
			t.Fatal("a matching basename admitted different bytes")
		}
	}
	old := m.Capture("sha256:072e9801b7df36a7421f05b9a8065b436ab26441824c214b670ad1185be448ac", "s2-sf-0.pcapng")
	if old.CaptureID != "ashbury-downey" || old.SiteID != "ashbury-downey" || old.Cohort != CohortDevelopment {
		t.Fatalf("replaced the existing Ashbury identity: %+v", old)
	}
}
