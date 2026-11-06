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
