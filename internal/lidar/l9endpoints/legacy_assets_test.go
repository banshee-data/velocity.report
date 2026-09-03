package l9endpoints

import (
	"io/fs"
	"strings"
	"testing"
)

func TestLegacyAssetsFS(t *testing.T) {
	fsys := LegacyAssetsFS()

	// The embedded tree must contain at least the ECharts library.
	info, err := fs.Stat(fsys, "echarts.min.js")
	if err != nil {
		t.Fatalf("stat echarts.min.js: %v", err)
	}
	if info.Size() == 0 {
		t.Error("echarts.min.js is empty")
	}
}

func TestLegacyStatusFS(t *testing.T) {
	fsys := LegacyStatusFS()

	info, err := fs.Stat(fsys, "status.html")
	if err != nil {
		t.Fatalf("stat status.html: %v", err)
	}
	if info.Size() == 0 {
		t.Error("status.html is empty")
	}
}

func TestReadLegacyAsset(t *testing.T) {
	data, err := ReadLegacyAsset("common.css")
	if err != nil {
		t.Fatalf("ReadLegacyAsset(common.css): %v", err)
	}
	if len(data) == 0 {
		t.Error("common.css content is empty")
	}
}

func TestLegacyDashboardHTML(t *testing.T) {
	if len(LegacyDashboardHTML) == 0 {
		t.Error("LegacyDashboardHTML is empty")
	}
}

func TestLegacyRegionsDashboardHTML(t *testing.T) {
	if len(LegacyRegionsDashboardHTML) == 0 {
		t.Error("LegacyRegionsDashboardHTML is empty")
	}
}

func TestLegacySweepDashboardHTML(t *testing.T) {
	if len(LegacySweepDashboardHTML) == 0 {
		t.Error("LegacySweepDashboardHTML is empty")
	}
}

// TestMustSubFSPanicsOnInvalidPath covers the failure mode the exported
// accessors no longer expose. Rooting an embedded tree cannot fail for the
// constant paths this package uses, which is why the accessors return a plain
// fs.FS — but the helper itself has to say so loudly if that ever stops being
// true, rather than handing back a nil filesystem.
func TestMustSubFSPanicsOnInvalidPath(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected a panic for a path fs.Sub rejects")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is %T, want a string", r)
		}
		if !strings.Contains(msg, "l9endpoints") {
			t.Errorf("panic message %q should name the package", msg)
		}
	}()

	// A path with a parent traversal is not a valid fs path.
	mustSubFS(legacyAssetsRaw, "../escape")
}
