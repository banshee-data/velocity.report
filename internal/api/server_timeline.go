package api

import (
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strconv"

	"github.com/banshee-data/velocity.report/internal/db"
)

const maxUnixTime = 32503680000.0

func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if r.Method != http.MethodGet {
		s.writeJSONError(w, http.StatusMethodNotAllowed, "Method not allowed")
		return
	}

	siteIDStr := r.URL.Query().Get("site_id")
	if siteIDStr == "" {
		s.writeJSONError(w, http.StatusBadRequest, "site_id is required")
		return
	}
	siteID, err := strconv.Atoi(siteIDStr)
	if err != nil || siteID <= 0 {
		s.writeJSONError(w, http.StatusBadRequest, "Invalid 'site_id' parameter; must be a positive integer")
		return
	}

	dataRange, err := s.db.RadarDataRange()
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve data range: %v", err))
		return
	}

	periods, err := s.db.ListSiteConfigPeriods(&siteID)
	if err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, fmt.Sprintf("Failed to retrieve site config periods: %v", err))
		return
	}

	unconfigured := computeUnconfiguredGaps(dataRange, periods)
	resp := map[string]interface{}{
		"site_id":              siteID,
		"data_range":           dataRange,
		"config_periods":       periods,
		"unconfigured_periods": unconfigured,
	}
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		s.writeJSONError(w, http.StatusInternalServerError, "Failed to encode timeline response")
	}
}

func uniqueAnglesForRange(periods []db.SiteConfigPeriod, startUnix, endUnix float64) []float64 {
	angleMap := make(map[float64]struct{})
	for _, period := range periods {
		periodEnd := maxUnixTime
		if period.EffectiveEndUnix != nil {
			periodEnd = *period.EffectiveEndUnix
		}
		if period.EffectiveStartUnix < endUnix && periodEnd > startUnix {
			angleMap[period.CosineErrorAngle] = struct{}{}
		}
	}
	angles := make([]float64, 0, len(angleMap))
	for angle := range angleMap {
		angles = append(angles, angle)
	}
	sort.Float64s(angles)
	return angles
}

func computeUnconfiguredGaps(dataRange *db.DataRange, periods []db.SiteConfigPeriod) []map[string]float64 {
	if dataRange == nil || dataRange.StartUnix == 0 || dataRange.EndUnix == 0 {
		return []map[string]float64{}
	}
	startUnix := dataRange.StartUnix
	endUnix := dataRange.EndUnix
	if endUnix <= startUnix {
		return []map[string]float64{}
	}

	sort.Slice(periods, func(i, j int) bool {
		return periods[i].EffectiveStartUnix < periods[j].EffectiveStartUnix
	})

	current := startUnix
	gaps := []map[string]float64{}
	for _, period := range periods {
		periodEnd := maxUnixTime
		if period.EffectiveEndUnix != nil {
			periodEnd = *period.EffectiveEndUnix
		}
		if periodEnd <= startUnix || period.EffectiveStartUnix >= endUnix {
			continue
		}
		if period.EffectiveStartUnix > current {
			gaps = append(gaps, map[string]float64{
				"start_unix": current,
				"end_unix":   math.Min(period.EffectiveStartUnix, endUnix),
			})
		}
		if periodEnd > current {
			current = math.Min(periodEnd, endUnix)
		}
		if current >= endUnix {
			break
		}
	}

	if current < endUnix {
		gaps = append(gaps, map[string]float64{
			"start_unix": current,
			"end_unix":   endUnix,
		})
	}

	return gaps
}
