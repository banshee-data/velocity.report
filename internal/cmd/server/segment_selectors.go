package server

import (
	radarassets "github.com/banshee-data/velocity.report"
	"github.com/banshee-data/velocity.report/internal/lidar/segments"
)

// loadSegmentSelectors reads the catalogue the segments API ranks with. With
// no path it is the repository's file when the server runs from the
// repository, and the binary's own copy anywhere else. A path that is given
// must name a file that parses: a typing mistake must not quietly rank with
// the defaults.
func loadSegmentSelectors(path string) (*segments.Catalogue, error) {
	if path == "" {
		return segments.LoadSelectorsOrEmbedded(segments.DefaultSelectorsPath, radarassets.SegmentSelectorDefaults)
	}
	return segments.LoadSelectors(path)
}

// mustLoadSegmentSelectors loads the catalogue for the server, or stops it:
// ranking with some other catalogue than the one named would be worse than
// not starting. It says where the catalogue came from, and its digest.
func mustLoadSegmentSelectors(path string, fatalf, logf logfFunc) *segments.Catalogue {
	selectors, err := loadSegmentSelectors(path)
	if err != nil {
		fatalf("Failed to load segment selectors: %v", err)
		return nil
	}
	logf("Loaded %d segment selectors from %s (%s)", len(selectors.Selectors), selectors.Source, selectors.Digest)
	return selectors
}
