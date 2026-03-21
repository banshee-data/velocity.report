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

// LegacyAssetsFS returns the embedded ECharts asset tree rooted at assets/.
// Callers use http.StripPrefix to serve at /assets/.
func LegacyAssetsFS() (fs.FS, error) {
	return fs.Sub(legacyAssetsRaw, "l10clients/assets")
}

// LegacyStatusFS returns the embedded status HTML tree rooted at html/.
func LegacyStatusFS() (fs.FS, error) {
	return fs.Sub(legacyStatusRaw, "l10clients/html")
}

// ReadLegacyAsset reads a single file from the embedded assets directory.
func ReadLegacyAsset(name string) ([]byte, error) {
	return legacyAssetsRaw.ReadFile("l10clients/assets/" + name)
}
