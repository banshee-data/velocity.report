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

// CaseLocation is where a replay case was captured — its sensor pose: where
// the car was that day, which is free to differ between visits to the same
// site.
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

	// SiteID is the site this case is at — the L16 token, unless something has
	// gone wrong, in which case they would disagree and that is a provenance
	// fault rather than something to reconcile silently.
	SiteID string `json:"site_id"`

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
// from the position and linking the case to the site it names — creating that
// site, with no canonical pose yet, the first time anything visits it.
//
// The three tokens are derived together with Parent rather than accepted from
// the caller, so a stored family cannot disagree with itself and no caller can
// introduce a mismatch the guide would treat as a provenance error. The site
// link and the case's own fields are written in one transaction: a case
// cannot end up naming a site that was not also recorded.
func (s *ReplayCaseStore) SetCaseLocation(replayCaseID string, lat, lon float64, source string) (CaseLocation, error) {
	tokens, err := geoindex.FromLatLng(lat, lon)
	if err != nil {
		return CaseLocation{}, err
	}
	if source == "" {
		source = GeoSourceOperator
	}

	tx, err := s.db.Begin()
	if err != nil {
		return CaseLocation{}, fmt.Errorf("begin set case location: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if err := upsertSite(tx, tokens.Precise, tokens.Fine, tokens.Coarse); err != nil {
		return CaseLocation{}, err
	}

	res, err := tx.Exec(`
		UPDATE lidar_replay_cases
		   SET origin_lat = ?, origin_lon = ?,
		       s2_l10_token = ?, s2_l13_token = ?, s2_l16_token = ?, site_id = ?,
		       geographic_source = ?, geographic_status = ?, updated_at_ns = ?
		 WHERE replay_case_id = ?`,
		lat, lon, tokens.Coarse, tokens.Fine, tokens.Precise, tokens.Precise,
		source, GeoLocated, time.Now().UnixNano(), replayCaseID)
	if err != nil {
		return CaseLocation{}, fmt.Errorf("set case location: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return CaseLocation{}, ErrNotFound
	}

	if err := tx.Commit(); err != nil {
		return CaseLocation{}, fmt.Errorf("commit set case location: %w", err)
	}

	loc := CaseLocation{
		Lat: lat, Lon: lon,
		L10Token: tokens.Coarse, L13Token: tokens.Fine, L16Token: tokens.Precise,
		SiteID: tokens.Precise,
		Source: source, Status: GeoLocated,
	}
	loc.withDisplays()
	return loc, nil
}

// ClearCaseLocation removes a case's position, returning it to the ordinary
// unlocated state. The site itself is left in place — other cases, or a
// canonical pose someone has already set, may still depend on it.
func (s *ReplayCaseStore) ClearCaseLocation(replayCaseID string) error {
	res, err := s.db.Exec(`
		UPDATE lidar_replay_cases
		   SET origin_lat = NULL, origin_lon = NULL,
		       s2_l10_token = NULL, s2_l13_token = NULL, s2_l16_token = NULL, site_id = NULL,
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
		       site_id, geographic_source, geographic_status
		  FROM lidar_replay_cases WHERE replay_case_id = ?`, replayCaseID)

	var lat, lon *float64
	var l10, l13, l16, siteID, source *string
	var status string
	if err := row.Scan(&lat, &lon, &l10, &l13, &l16, &siteID, &source, &status); err != nil {
		return nil, err
	}
	if status != GeoLocated || lat == nil || lon == nil || l10 == nil {
		return nil, nil
	}
	loc := &CaseLocation{
		Lat: *lat, Lon: *lon,
		L10Token: derefString(l10), L13Token: derefString(l13), L16Token: derefString(l16),
		SiteID: derefString(siteID),
		Source: derefString(source), Status: status,
	}
	loc.withDisplays()
	return loc, nil
}

// SceneArea is one area captures were taken in, as the scene map shows it. The
// grouping is the L10 cell, which is district-scale — about 12 km by 8 km at
// San Francisco's latitude. That is the archive-scale roll-up, not a
// junction: the junction is a Site (the L16 cell), and SiteCount is how many
// of them the area holds. A single entry covering several distinct sites is
// expected and is why Sites is a list, not a single spot.
type SceneArea struct {
	L10Token   string `json:"s2_l10_token"`
	L10Display string `json:"s2_l10_display"`

	// CentreLat and CentreLon are the L10 cell's centre, for placing the area.
	CentreLat float64 `json:"centre_lat"`
	CentreLon float64 `json:"centre_lon"`
	// The cell's bounding box, so a map can draw the area rather than a pin.
	SouthWestLat float64 `json:"sw_lat"`
	SouthWestLon float64 `json:"sw_lon"`
	NorthEastLat float64 `json:"ne_lat"`
	NorthEastLon float64 `json:"ne_lon"`

	CaseCount          int `json:"case_count"`
	NeighbourhoodCount int `json:"neighbourhood_count"`

	// Sites are the area's distinct junctions, each with the cases captured
	// there, newest first within a site and by site's most recent case
	// otherwise.
	Sites []Site `json:"sites"`
}

// SceneAreas returns every located area, coarsest grouping first, each
// holding the sites within it and the cases captured at each one.
//
// This is the scene map: one entry per L10 cell, containing the L16 sites
// visited inside it. A site with a canonical pose already set carries it
// here; one without is the ordinary case, not a defect.
func (s *ReplayCaseStore) SceneAreas() ([]SceneArea, error) {
	siteRows, err := s.db.Query(`SELECT ` + siteSelectColumns + ` FROM lidar_sites`)
	if err != nil {
		return nil, fmt.Errorf("list sites: %w", err)
	}
	sitesByToken := map[string]*Site{}
	for siteRows.Next() {
		site, err := scanSite(siteRows)
		if err != nil {
			siteRows.Close()
			return nil, fmt.Errorf("scan site: %w", err)
		}
		s := site
		sitesByToken[s.L16Token] = &s
	}
	if err := siteRows.Err(); err != nil {
		siteRows.Close()
		return nil, err
	}
	siteRows.Close()

	caseRows, err := s.db.Query(`
		SELECT replay_case_id, COALESCE(description, ''), sensor_id,
		       s2_l10_token, s2_l13_token, s2_l16_token,
		       origin_lat, origin_lon, created_at_ns
		  FROM lidar_replay_cases
		 WHERE geographic_status = ?
		   AND s2_l10_token IS NOT NULL
		 ORDER BY s2_l10_token, s2_l16_token, created_at_ns DESC`, GeoLocated)
	if err != nil {
		return nil, fmt.Errorf("list located cases: %w", err)
	}
	defer caseRows.Close()

	areaByToken := map[string]*SceneArea{}
	areaOrder := []string{}
	// A site's parent area is unambiguous from its own tokens, so sites are
	// attached to their area the first time a case under them is seen —
	// there is no need to visit lidar_sites again for that.
	siteAttached := map[string]bool{}
	neighbourhoodsSeen := map[string]map[string]struct{}{}

	for caseRows.Next() {
		var c SiteCase
		var l10, l13, l16 string
		if err := caseRows.Scan(&c.ReplayCaseID, &c.Description, &c.SensorID,
			&l10, &l13, &l16, &c.Lat, &c.Lon, &c.CreatedAtNs); err != nil {
			return nil, fmt.Errorf("scan located case: %w", err)
		}

		area, ok := areaByToken[l10]
		if !ok {
			centreLat, centreLon, err := geoindex.CentreOf(l10)
			if err != nil {
				// A row whose token will not parse is a provenance fault, not
				// an area. Skipping it keeps one bad row from emptying the map.
				continue
			}
			swLat, swLon, neLat, neLon, err := geoindex.BoundOf(l10)
			if err != nil {
				continue
			}
			area = &SceneArea{
				L10Token: l10, L10Display: geoindex.FamilyDisplay(l10),
				CentreLat: centreLat, CentreLon: centreLon,
				SouthWestLat: swLat, SouthWestLon: swLon,
				NorthEastLat: neLat, NorthEastLon: neLon,
			}
			areaByToken[l10] = area
			areaOrder = append(areaOrder, l10)
			neighbourhoodsSeen[l10] = map[string]struct{}{}
		}

		site := sitesByToken[l16]
		if site == nil {
			// A located case always upserts its site in the same transaction
			// (SetCaseLocation), so this is defensive rather than expected —
			// a case whose site row is missing still gets one here, unlabelled
			// and without a canonical pose, so it is not silently dropped.
			site = &Site{L16Token: l16, L13Token: l13, L10Token: l10}
			sitesByToken[l16] = site
		}
		if !siteAttached[l16] {
			area.Sites = append(area.Sites, *site)
			siteAttached[l16] = true
		}
		// area.Sites holds copies; index back in to append this case to the
		// right one rather than the local site pointer, which is not shared
		// with what was just appended.
		for i := range area.Sites {
			if area.Sites[i].L16Token == l16 {
				area.Sites[i].Cases = append(area.Sites[i].Cases, c)
				break
			}
		}
		area.CaseCount++
		neighbourhoodsSeen[l10][l13] = struct{}{}
	}
	if err := caseRows.Err(); err != nil {
		return nil, err
	}

	areas := make([]SceneArea, 0, len(areaOrder))
	for _, l10 := range areaOrder {
		area := areaByToken[l10]
		area.NeighbourhoodCount = len(neighbourhoodsSeen[l10])
		areas = append(areas, *area)
	}
	return areas, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
