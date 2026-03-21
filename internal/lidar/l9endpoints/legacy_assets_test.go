package l9endpoints

import (
	"io/fs"
	"testing"
)

func TestLegacyAssetsFS(t *testing.T) {
	fsys, err := LegacyAssetsFS()
	if err != nil {
		t.Fatalf("LegacyAssetsFS: %v", err)
	}

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
	fsys, err := LegacyStatusFS()
	if err != nil {
		t.Fatalf("LegacyStatusFS: %v", err)
	}

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
