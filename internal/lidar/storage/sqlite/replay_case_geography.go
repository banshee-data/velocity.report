package sqlite

import (
	"fmt"
	"time"

	"github.com/banshee-data/velocity.report/internal/lidar/geoindex"
)

// Geographic statuses a replay case can be in.
const (
	// GeoLocated means the case carries an accepted WGS84 position and the S2
	// family derived from it.
	GeoLocated = "located"
	// GeoUnavailable means no accepted position. This is the ordinary state for
	// a sensor-local capture, not a defect.
	GeoUnavailable = "unavailable"
)

// How a position was established.
const (
	GeoSourceSurveyed = "surveyed"
	GeoSourceOperator = "operator"
	GeoSourceFix      = "fix"
)

// CaseLocation is where a replay case was captured.
//
// The tokens are the identity; Lat and Lon are what they were derived from and
// what a map draws. The family displays are presentation and are filled in on
// read rather than stored, because the guide forbids persisting them.
type CaseLocation struct {
	Lat float64 `json:"origin_lat"`
	Lon float64 `json:"origin_lon"`

	// Canonical S2 tokens. These are the identifiers.
	L10Token string `json:"s2_l10_token"`
	L13Token string `json:"s2_l13_token"`
	L16Token string `json:"s2_l16_token"`

	// Family displays, derived on read for human-facing surfaces. Never stored.
	L10Display string `json:"s2_l10_display"`
	L13Display string `json:"s2_l13_display"`
	L16Display string `json:"s2_l16_display"`

	Source string `json:"geographic_source,omitempty"`
	Status string `json:"geographic_status"`
}

// withDisplays fills in the derived presentation forms.
func (l *CaseLocation) withDisplays() {
	l.L10Display = geoindex.FamilyDisplay(l.L10Token)
	l.L13Display = geoindex.FamilyDisplay(l.L13Token)
	l.L16Display = geoindex.FamilyDisplay(l.L16Token)
}

// SetCaseLocation records where a case was captured, deriving the S2 family
// from the position.
//
// The three tokens are derived together with Parent rather than accepted from
// the caller, so a stored family cannot disagree with itself and no caller can
// introduce a mismatch the guide would treat as a provenance error.
func (s *ReplayCaseStore) SetCaseLocation(replayCaseID string, lat, lon float64, source string) (CaseLocation, error) {
	tokens, err := geoindex.FromLatLng(lat, lon)
	if err != nil {
		return CaseLocation{}, err
	}
	if source == "" {
		source = GeoSourceOperator
	}

	res, err := s.db.Exec(`
		UPDATE lidar_replay_cases
		   SET origin_lat = ?, origin_lon = ?,
		       s2_l10_token = ?, s2_l13_token = ?, s2_l16_token = ?,
		       geographic_source = ?, geographic_status = ?, updated_at_ns = ?
		 WHERE replay_case_id = ?`,
		lat, lon, tokens.Coarse, tokens.Fine, tokens.Precise,
		source, GeoLocated, time.Now().UnixNano(), replayCaseID)
	if err != nil {
		return CaseLocation{}, fmt.Errorf("set case location: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return CaseLocation{}, ErrNotFound
	}

	loc := CaseLocation{
		Lat: lat, Lon: lon,
		L10Token: tokens.Coarse, L13Token: tokens.Fine, L16Token: tokens.Precise,
		Source: source, Status: GeoLocated,
	}
	loc.withDisplays()
	return loc, nil
}

// ClearCaseLocation removes a case's position, returning it to the ordinary
// unlocated state.
func (s *ReplayCaseStore) ClearCaseLocation(replayCaseID string) error {
	res, err := s.db.Exec(`
		UPDATE lidar_replay_cases
		   SET origin_lat = NULL, origin_lon = NULL,
		       s2_l10_token = NULL, s2_l13_token = NULL, s2_l16_token = NULL,
		       geographic_source = NULL, geographic_status = ?, updated_at_ns = ?
		 WHERE replay_case_id = ?`,
		GeoUnavailable, time.Now().UnixNano(), replayCaseID)
	if err != nil {
		return fmt.Errorf("clear case location: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrNotFound
	}
	return nil
}

// CaseLocationOf returns a case's location, or a nil location when it has none.
func (s *ReplayCaseStore) CaseLocationOf(replayCaseID string) (*CaseLocation, error) {
	row := s.db.QueryRow(`
		SELECT origin_lat, origin_lon, s2_l10_token, s2_l13_token, s2_l16_token,
		       geographic_source, geographic_status
		  FROM lidar_replay_cases WHERE replay_case_id = ?`, replayCaseID)

	var lat, lon *float64
	var l10, l13, l16, source *string
	var status string
	if err := row.Scan(&lat, &lon, &l10, &l13, &l16, &source, &status); err != nil {
		return nil, err
	}
	if status != GeoLocated || lat == nil || lon == nil || l10 == nil {
		return nil, nil
	}
	loc := &CaseLocation{
		Lat: *lat, Lon: *lon,
		L10Token: derefString(l10), L13Token: derefString(l13), L16Token: derefString(l16),
		Source: derefString(source), Status: status,
	}
	loc.withDisplays()
	return loc, nil
}

// SceneSite is one area captures were taken in, as the scene map shows it.
//
// The grouping is the L10 cell, which is district-scale — about 12 km by 8 km
// at San Francisco's latitude. That is the archive-scale roll-up, not a
// junction: the junction is the L16 cell, and L16Count is how many of them the
// area holds. A single entry covering several distinct sites is expected and
// is why the finer counts are carried alongside.
type SceneSite struct {
	L10Token   string `json:"s2_l10_token"`
	L10Display string `json:"s2_l10_display"`

	// CentreLat and CentreLon are the L10 cell's centre, for placing the site.
	CentreLat float64 `json:"centre_lat"`
	CentreLon float64 `json:"centre_lon"`
	// The cell's bounding box, so a map can draw the site rather than a pin.
	SouthWestLat float64 `json:"sw_lat"`
	SouthWestLon float64 `json:"sw_lon"`
	NorthEastLat float64 `json:"ne_lat"`
	NorthEastLon float64 `json:"ne_lon"`

	CaseCount int `json:"case_count"`
	// L13Count is how many neighbourhood cells the area holds and L16Count how
	// many sites — the latter is the count an operator reads as "places we have
	// captured here".
	L13Count int `json:"l13_count"`
	L16Count int `json:"l16_count"`

	// Cases are the located replay cases at this site, newest first.
	Cases []SceneSiteCase `json:"cases"`
}

// SceneSiteCase is one replay case as the scene map lists it.
type SceneSiteCase struct {
	ReplayCaseID string  `json:"replay_case_id"`
	Description  string  `json:"description,omitempty"`
	SensorID     string  `json:"sensor_id,omitempty"`
	L13Token     string  `json:"s2_l13_token"`
	L13Display   string  `json:"s2_l13_display"`
	L16Token     string  `json:"s2_l16_token"`
	L16Display   string  `json:"s2_l16_display"`
	Lat          float64 `json:"origin_lat"`
	Lon          float64 `json:"origin_lon"`
	CreatedAtNs  int64   `json:"created_at_ns"`
}

// SceneSites returns every located area, coarsest grouping first.
//
// This is the scene map: one entry per L10 cell, each linking to the cases
// captured there and counting the distinct sites inside it.
func (s *ReplayCaseStore) SceneSites() ([]SceneSite, error) {
	rows, err := s.db.Query(`
		SELECT replay_case_id, COALESCE(description, ''), sensor_id,
		       s2_l10_token, s2_l13_token, s2_l16_token,
		       origin_lat, origin_lon, created_at_ns
		  FROM lidar_replay_cases
		 WHERE geographic_status = ?
		   AND s2_l10_token IS NOT NULL
		 ORDER BY s2_l10_token, created_at_ns DESC`, GeoLocated)
	if err != nil {
		return nil, fmt.Errorf("list located cases: %w", err)
	}
	defer rows.Close()

	byCell := map[string]*SceneSite{}
	order := []string{}
	fineSeen := map[string]map[string]struct{}{}
	preciseSeen := map[string]map[string]struct{}{}

	for rows.Next() {
		var c SceneSiteCase
		var l10 string
		if err := rows.Scan(&c.ReplayCaseID, &c.Description, &c.SensorID,
			&l10, &c.L13Token, &c.L16Token, &c.Lat, &c.Lon, &c.CreatedAtNs); err != nil {
			return nil, fmt.Errorf("scan located case: %w", err)
		}
		c.L13Display = geoindex.FamilyDisplay(c.L13Token)
		c.L16Display = geoindex.FamilyDisplay(c.L16Token)

		site, ok := byCell[l10]
		if !ok {
			centreLat, centreLon, err := geoindex.CentreOf(l10)
			if err != nil {
				// A row whose token will not parse is a provenance fault, not a
				// site. Skipping it keeps one bad row from emptying the map.
				continue
			}
			swLat, swLon, neLat, neLon, err := geoindex.BoundOf(l10)
			if err != nil {
				continue
			}
			site = &SceneSite{
				L10Token: l10, L10Display: geoindex.FamilyDisplay(l10),
				CentreLat: centreLat, CentreLon: centreLon,
				SouthWestLat: swLat, SouthWestLon: swLon,
				NorthEastLat: neLat, NorthEastLon: neLon,
			}
			byCell[l10] = site
			order = append(order, l10)
			fineSeen[l10] = map[string]struct{}{}
			preciseSeen[l10] = map[string]struct{}{}
		}
		site.Cases = append(site.Cases, c)
		site.CaseCount++
		if c.L13Token != "" {
			fineSeen[l10][c.L13Token] = struct{}{}
		}
		if c.L16Token != "" {
			preciseSeen[l10][c.L16Token] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sites := make([]SceneSite, 0, len(order))
	for _, l10 := range order {
		site := byCell[l10]
		site.L13Count = len(fineSeen[l10])
		site.L16Count = len(preciseSeen[l10])
		sites = append(sites, *site)
	}
	return sites, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
