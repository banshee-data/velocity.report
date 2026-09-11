package sqlite

import (
	"database/sql"
	"fmt"
	"time"
)

// Site is a place captures have been taken, at L16 — a junction and its
// approaches, per docs/lidar/architecture/geographic-indexing.md. It is what
// "a scene should be at a site" refers to: many replay cases can visit the
// same site, each recording its own sensor pose (CaseLocation's lat/lon —
// where the car was that day, which moves visit to visit), while the site
// itself optionally carries a canonical pose: a fixed point such as the
// midpoint of the intersection, set deliberately rather than derived from any
// one visit.
//
// A site is created the first time a case is located there (see
// SetCaseLocation) and its identity is the L16 token itself — consistent with
// the geographic-indexing guide's rule that only the canonical token is an
// identifier. Its canonical pose starts unset: a site with several visits and
// no canonical pose yet is the ordinary case, not a defect.
type Site struct {
	L16Token string `json:"s2_l16_token"`
	L13Token string `json:"s2_l13_token"`
	L10Token string `json:"s2_l10_token"`

	Label string `json:"label,omitempty"`

	CanonicalLat *float64 `json:"canonical_lat,omitempty"`
	CanonicalLon *float64 `json:"canonical_lon,omitempty"`
	// CanonicalSource is surveyed or operator; empty when no canonical pose
	// has been set.
	CanonicalSource string `json:"canonical_source,omitempty"`

	CreatedAtNs int64  `json:"created_at_ns"`
	UpdatedAtNs *int64 `json:"updated_at_ns,omitempty"`

	// Cases are the located replay cases visited at this site, newest first.
	// Not a stored column — populated by SceneAreas when it assembles the
	// scene map.
	Cases []SiteCase `json:"cases,omitempty"`
}

// SiteCase is one replay case as a site lists it: its own sensor pose (where
// the car was that visit) alongside the site's shared, possibly-canonical
// one. It does not repeat the site's own S2 tokens — the parent Site already
// carries those.
type SiteCase struct {
	ReplayCaseID string  `json:"replay_case_id"`
	Description  string  `json:"description,omitempty"`
	SensorID     string  `json:"sensor_id,omitempty"`
	Lat          float64 `json:"origin_lat"`
	Lon          float64 `json:"origin_lon"`
	CreatedAtNs  int64   `json:"created_at_ns"`
}

// How a site's canonical pose was established. Unlike a case's
// geographic_source, there is no "fix" here: a canonical pose is by
// definition not one sensor's fix on one visit.
const (
	SiteSourceSurveyed = "surveyed"
	SiteSourceOperator = "operator"
)

// execer is the sliver of DBClient that both a *sql.DB-backed store and a
// *sql.Tx satisfy, so upsertSite can run standalone or inside a caller's
// transaction.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// upsertSite records that a site exists, denormalizing its L13/L10 ancestors
// for the scene map's rollups. It never touches the canonical pose — that is
// set deliberately, once, by SetSiteCanonicalPose, not implied by a case
// merely being located here.
//
// Takes an execer rather than a *ReplayCaseStore so SetCaseLocation can run it
// inside its own transaction: a case and the site it names come into being
// together or not at all.
func upsertSite(x execer, l16, l13, l10 string) error {
	if l16 == "" {
		return fmt.Errorf("upsert site: empty L16 token")
	}
	if _, err := x.Exec(`
		INSERT INTO lidar_sites (site_id, s2_l13_token, s2_l10_token, created_at_ns)
		VALUES (?, ?, ?, ?)
		ON CONFLICT (site_id) DO NOTHING`,
		l16, l13, l10, time.Now().UnixNano()); err != nil {
		return fmt.Errorf("upsert site: %w", err)
	}
	return nil
}

// SiteStore provides persistence for LiDAR sites.
type SiteStore struct {
	db DBClient
}

// NewSiteStore creates a new SiteStore.
func NewSiteStore(db DBClient) *SiteStore {
	return &SiteStore{db: db}
}

func scanSite(scanner interface{ Scan(dest ...any) error }) (Site, error) {
	var site Site
	var label, canonicalSource sql.NullString
	var canonicalLat, canonicalLon sql.NullFloat64
	var updatedAtNs sql.NullInt64

	if err := scanner.Scan(
		&site.L16Token, &site.L13Token, &site.L10Token,
		&label, &canonicalLat, &canonicalLon, &canonicalSource,
		&site.CreatedAtNs, &updatedAtNs,
	); err != nil {
		return Site{}, err
	}
	if label.Valid {
		site.Label = label.String
	}
	if canonicalLat.Valid && canonicalLon.Valid {
		lat, lon := canonicalLat.Float64, canonicalLon.Float64
		site.CanonicalLat = &lat
		site.CanonicalLon = &lon
	}
	if canonicalSource.Valid {
		site.CanonicalSource = canonicalSource.String
	}
	if updatedAtNs.Valid {
		v := updatedAtNs.Int64
		site.UpdatedAtNs = &v
	}
	return site, nil
}

const siteSelectColumns = `site_id, s2_l13_token, s2_l10_token,
	label, canonical_lat, canonical_lon, canonical_source, created_at_ns, updated_at_ns`

// ByToken returns one site, or ErrNotFound.
func (s *SiteStore) ByToken(l16Token string) (*Site, error) {
	row := s.db.QueryRow(`SELECT `+siteSelectColumns+`
		FROM lidar_sites WHERE site_id = ?`, l16Token)
	site, err := scanSite(row)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("get site: %w", err)
	}
	return &site, nil
}

// All returns every known site. There is no paging: a deployment's distinct
// junctions number in the hundreds at most, the same order of magnitude as
// the capture volume's own sessions.
func (s *SiteStore) All() ([]Site, error) {
	rows, err := s.db.Query(`SELECT ` + siteSelectColumns + ` FROM lidar_sites`)
	if err != nil {
		return nil, fmt.Errorf("list sites: %w", err)
	}
	defer rows.Close()

	sites := []Site{}
	for rows.Next() {
		site, err := scanSite(rows)
		if err != nil {
			return nil, fmt.Errorf("scan site: %w", err)
		}
		sites = append(sites, site)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sites, nil
}

// SetCanonicalPose records a site's fixed position — a surveyed or
// hand-entered point such as the midpoint of the intersection — distinct from
// any one visit's sensor pose. Label is set alongside it when non-empty;
// pass an empty label to leave whatever is already there.
//
// Unlike a case's location, this does not derive S2 tokens from the position:
// the site's identity is already the L16 token it was created under, and a
// canonical pose is not required to fall inside that cell (a surveyed
// intersection midpoint from real survey data takes precedence over the
// geometric cell it happens to be filed under).
func (s *SiteStore) SetCanonicalPose(l16Token string, lat, lon float64, source, label string) (*Site, error) {
	if source != SiteSourceSurveyed && source != SiteSourceOperator {
		return nil, fmt.Errorf("canonical_source must be surveyed or operator")
	}

	var res sql.Result
	var err error
	if label != "" {
		res, err = s.db.Exec(`
			UPDATE lidar_sites
			   SET canonical_lat = ?, canonical_lon = ?, canonical_source = ?,
			       label = ?, updated_at_ns = ?
			 WHERE site_id = ?`,
			lat, lon, source, label, time.Now().UnixNano(), l16Token)
	} else {
		res, err = s.db.Exec(`
			UPDATE lidar_sites
			   SET canonical_lat = ?, canonical_lon = ?, canonical_source = ?,
			       updated_at_ns = ?
			 WHERE site_id = ?`,
			lat, lon, source, time.Now().UnixNano(), l16Token)
	}
	if err != nil {
		return nil, fmt.Errorf("set site canonical pose: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return nil, ErrNotFound
	}
	return s.ByToken(l16Token)
}
