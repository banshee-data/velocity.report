package report

import (
	"context"

	"github.com/banshee-data/velocity.report/internal/db"
)

// DB defines the database operations needed by the report package.
// This interface allows mocking in tests without requiring the full db.DB.
type DB interface {
	RadarObjectRollupRange(startUnix, endUnix, groupSeconds int64, minSpeed float64, dataSource string, modelVersion string, histBucketSize, histMax float64, siteID int, boundaryThreshold int) (*db.RadarStatsResult, error)
	GetSite(ctx context.Context, id int) (*db.Site, error)
}
