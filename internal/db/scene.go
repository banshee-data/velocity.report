package db

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Scene is one publishable LiDAR scene: the index entry that gives a recording
// a place, a time and a name.
//
// Latitude and Longitude are optional overrides. When they are nil the scene
// inherits the position of its linked site, which is why the API also returns
// EffectiveLatitude and EffectiveLongitude — a caller placing a marker on a map
// should not have to know where the number came from.
//
// The capture window is stored in Unix nanoseconds because that is what the
// recording carries. It is exposed as RFC 3339 alongside, so an editor can show
// a date and time without every client reimplementing the conversion.
type Scene struct {
	SceneID     string  `json:"scene_id"`
	SiteID      *int    `json:"site_id"`
	Title       string  `json:"title"`
	Description *string `json:"description"`

	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`

	// Resolved position: the scene's own, or the linked site's. Read-only.
	EffectiveLatitude  *float64 `json:"effective_latitude,omitempty"`
	EffectiveLongitude *float64 `json:"effective_longitude,omitempty"`
	PositionSource     string   `json:"position_source,omitempty"`

	CapturedStartNs *int64   `json:"captured_start_ns"`
	CapturedEndNs   *int64   `json:"captured_end_ns"`
	DurationSecs    *float64 `json:"duration_secs"`

	// RFC 3339 renderings of the capture window, for display and editing.
	CapturedStart *string `json:"captured_start,omitempty"`
	CapturedEnd   *string `json:"captured_end,omitempty"`

	SourceCapture     *string `json:"source_capture"`
	SourceVRLOGSHA256 *string `json:"source_vrlog_sha256"`
	FrameCount        *int    `json:"frame_count"`
	FrameStride       *int    `json:"frame_stride"`

	AssetPath *string `json:"asset_path"`
	Published bool    `json:"published"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const sceneColumns = `
	s.scene_id, s.site_id, s.title, s.description,
	s.latitude, s.longitude,
	s.captured_start_ns, s.captured_end_ns, s.duration_secs,
	s.source_capture, s.source_vrlog_sha256, s.frame_count, s.frame_stride,
	s.asset_path, s.published, s.created_at, s.updated_at,
	site.latitude, site.longitude`

// scanScene reads one row and resolves the effective position and timestamps.
func scanScene(scan func(dest ...any) error) (*Scene, error) {
	var (
		sc                   Scene
		published            int
		createdAt, updatedAt int64
		siteLat, siteLng     sql.NullFloat64
	)
	if err := scan(
		&sc.SceneID, &sc.SiteID, &sc.Title, &sc.Description,
		&sc.Latitude, &sc.Longitude,
		&sc.CapturedStartNs, &sc.CapturedEndNs, &sc.DurationSecs,
		&sc.SourceCapture, &sc.SourceVRLOGSHA256, &sc.FrameCount, &sc.FrameStride,
		&sc.AssetPath, &published, &createdAt, &updatedAt,
		&siteLat, &siteLng,
	); err != nil {
		return nil, err
	}
	sc.Published = published == 1
	sc.CreatedAt = time.Unix(createdAt, 0).UTC()
	sc.UpdatedAt = time.Unix(updatedAt, 0).UTC()

	// A scene's own position wins; otherwise it inherits the site's.
	switch {
	case sc.Latitude != nil && sc.Longitude != nil:
		sc.EffectiveLatitude, sc.EffectiveLongitude = sc.Latitude, sc.Longitude
		sc.PositionSource = "scene"
	case siteLat.Valid && siteLng.Valid:
		lat, lng := siteLat.Float64, siteLng.Float64
		sc.EffectiveLatitude, sc.EffectiveLongitude = &lat, &lng
		sc.PositionSource = "site"
	default:
		sc.PositionSource = "none"
	}

	if sc.CapturedStartNs != nil {
		t := time.Unix(0, *sc.CapturedStartNs).UTC().Format(time.RFC3339)
		sc.CapturedStart = &t
	}
	if sc.CapturedEndNs != nil {
		t := time.Unix(0, *sc.CapturedEndNs).UTC().Format(time.RFC3339)
		sc.CapturedEnd = &t
	}
	return &sc, nil
}

// GetAllScenes returns every scene, newest capture first. Scenes with no
// capture window sort last rather than being hidden.
func (db *DB) GetAllScenes(ctx context.Context) ([]Scene, error) {
	query := `SELECT ` + sceneColumns + `
		FROM lidar_scenes s
		LEFT JOIN site ON site.id = s.site_id
		ORDER BY s.captured_start_ns IS NULL, s.captured_start_ns DESC, s.scene_id`

	rows, err := db.DB.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("query scenes: %w", err)
	}
	defer func() { _ = rows.Close() }()

	scenes := []Scene{}
	for rows.Next() {
		sc, err := scanScene(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan scene: %w", err)
		}
		scenes = append(scenes, *sc)
	}
	return scenes, rows.Err()
}

// GetScene returns one scene by its identifier.
func (db *DB) GetScene(ctx context.Context, sceneID string) (*Scene, error) {
	query := `SELECT ` + sceneColumns + `
		FROM lidar_scenes s
		LEFT JOIN site ON site.id = s.site_id
		WHERE s.scene_id = ?`

	sc, err := scanScene(db.DB.QueryRowContext(ctx, query, sceneID).Scan)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get scene %q: %w", sceneID, err)
	}
	return sc, nil
}

// CreateScene inserts a scene.
func (db *DB) CreateScene(ctx context.Context, sc *Scene) error {
	published := 0
	if sc.Published {
		published = 1
	}
	_, err := db.DB.ExecContext(ctx, `
		INSERT INTO lidar_scenes (
			scene_id, site_id, title, description,
			latitude, longitude,
			captured_start_ns, captured_end_ns, duration_secs,
			source_capture, source_vrlog_sha256, frame_count, frame_stride,
			asset_path, published
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sc.SceneID, sc.SiteID, sc.Title, sc.Description,
		sc.Latitude, sc.Longitude,
		sc.CapturedStartNs, sc.CapturedEndNs, sc.DurationSecs,
		sc.SourceCapture, sc.SourceVRLOGSHA256, sc.FrameCount, sc.FrameStride,
		sc.AssetPath, published)
	if err != nil {
		return fmt.Errorf("create scene %q: %w", sc.SceneID, err)
	}
	return nil
}

// UpdateScene replaces the editable fields of an existing scene. It reports
// whether a row matched, so a caller can distinguish "not found" from success.
func (db *DB) UpdateScene(ctx context.Context, sc *Scene) (bool, error) {
	published := 0
	if sc.Published {
		published = 1
	}
	res, err := db.DB.ExecContext(ctx, `
		UPDATE lidar_scenes SET
			site_id = ?, title = ?, description = ?,
			latitude = ?, longitude = ?,
			captured_start_ns = ?, captured_end_ns = ?, duration_secs = ?,
			source_capture = ?, source_vrlog_sha256 = ?,
			frame_count = ?, frame_stride = ?,
			asset_path = ?, published = ?
		WHERE scene_id = ?`,
		sc.SiteID, sc.Title, sc.Description,
		sc.Latitude, sc.Longitude,
		sc.CapturedStartNs, sc.CapturedEndNs, sc.DurationSecs,
		sc.SourceCapture, sc.SourceVRLOGSHA256, sc.FrameCount, sc.FrameStride,
		sc.AssetPath, published, sc.SceneID)
	if err != nil {
		return false, fmt.Errorf("update scene %q: %w", sc.SceneID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("update scene %q: %w", sc.SceneID, err)
	}
	return n > 0, nil
}

// DeleteScene removes a scene, reporting whether one existed.
func (db *DB) DeleteScene(ctx context.Context, sceneID string) (bool, error) {
	res, err := db.DB.ExecContext(ctx, `DELETE FROM lidar_scenes WHERE scene_id = ?`, sceneID)
	if err != nil {
		return false, fmt.Errorf("delete scene %q: %w", sceneID, err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("delete scene %q: %w", sceneID, err)
	}
	return n > 0, nil
}
