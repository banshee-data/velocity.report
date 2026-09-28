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
