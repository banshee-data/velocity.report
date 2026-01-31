package db

import (
	"fmt"
	"time"
)

// TransitGap represents an hourly period with radar data but missing transit records.
type TransitGap struct {
	Start       time.Time
	End         time.Time
	RecordCount int
}

// FindTransitGaps finds hourly periods where radar_data exists but no transits have been computed.
func (db *DB) FindTransitGaps() ([]TransitGap, error) {
	// Query to find hourly periods with radar_data but no transits
	// We check for each hour bucket if there are radar_data records but no transit records
	query := `
	WITH hourly_data AS (
		-- Get all hourly buckets that have radar_data
		SELECT
			CAST(write_timestamp / 3600 AS INTEGER) * 3600 as hour_start,
			COUNT(*) as data_count
		FROM radar_data
		WHERE write_timestamp IS NOT NULL
		  AND (speed IS NOT NULL OR magnitude IS NOT NULL)
		GROUP BY hour_start
	),
	hourly_transits AS (
		-- Get all hourly buckets that have transits
		SELECT
			CAST(transit_start_unix / 3600 AS INTEGER) * 3600 as hour_start,
			COUNT(*) as transit_count
		FROM radar_data_transits
		WHERE transit_start_unix IS NOT NULL
		GROUP BY hour_start
	)
	SELECT
		hd.hour_start,
		hd.data_count
	FROM hourly_data hd
	WHERE NOT EXISTS (
		SELECT 1 FROM hourly_transits ht
		WHERE ht.hour_start = hd.hour_start
	)
	ORDER BY hd.hour_start
	`

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var gaps []TransitGap
	for rows.Next() {
		var hourStartUnix int64
		var recordCount int64
		if err := rows.Scan(&hourStartUnix, &recordCount); err != nil {
			return nil, fmt.Errorf("scan failed: %w", err)
		}

		start := time.Unix(hourStartUnix, 0).UTC()
		end := start.Add(1 * time.Hour)

		gaps = append(gaps, TransitGap{
			Start:       start,
			End:         end,
			RecordCount: int(recordCount),
		})
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("rows iteration failed: %w", err)
	}

	return gaps, nil
}
