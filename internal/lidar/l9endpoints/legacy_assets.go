// Package l9endpoints embeds deprecated dashboard assets from the l10clients
// subtree. These assets are transitional and will be removed once the
// consolidated frontend replaces them.
package l9endpoints

import (
	"embed"
	"io/fs"
)

//go:embed l10clients/assets/*
var legacyAssetsRaw embed.FS

//go:embed l10clients/html/status.html
var legacyStatusRaw embed.FS

//go:embed l10clients/html/dashboard.html
var LegacyDashboardHTML string

//go:embed l10clients/html/regions_dashboard.html
var LegacyRegionsDashboardHTML string

//go:embed l10clients/html/sweep_dashboard.html
var LegacySweepDashboardHTML string

// The rooted views of the embedded trees. Both are resolved once at
// initialisation: the directories are compile-time constants over trees
// go:embed has already guaranteed exist, so a failure here is a malformed
// constant — a build mistake, not a runtime condition callers can act on.
var (
	legacyAssetsFS = mustSubFS(legacyAssetsRaw, "l10clients/assets")
	legacyStatusFS = mustSubFS(legacyStatusRaw, "l10clients/html")
)

// mustSubFS roots an embedded tree at dir, panicking if the path is not one
// fs.Sub accepts. Returning an error instead pushed an impossible failure onto
// every call site, where it became an untestable branch in each HTTP handler.
func mustSubFS(fsys fs.FS, dir string) fs.FS {
	sub, err := fs.Sub(fsys, dir)
	if err != nil {
		panic("l9endpoints: embedded asset tree " + dir + ": " + err.Error())
	}
	return sub
}

// LegacyAssetsFS returns the embedded ECharts asset tree rooted at assets/.
// Callers use http.StripPrefix to serve at /assets/.
func LegacyAssetsFS() fs.FS {
	return legacyAssetsFS
}

// LegacyStatusFS returns the embedded status HTML tree rooted at html/.
func LegacyStatusFS() fs.FS {
	return legacyStatusFS
}

// ReadLegacyAsset reads a single file from the embedded assets directory.
func ReadLegacyAsset(name string) ([]byte, error) {
	return legacyAssetsRaw.ReadFile("l10clients/assets/" + name)
}
