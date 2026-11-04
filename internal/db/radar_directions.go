package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

// A radar deployment's two directions
// (docs/plans/platform-speed-by-direction-reporting-plan.md, Item 1): the
// OPS243's readings are signed, so a transit is inbound or outbound at the
// sensor; the deployment names each, gives its bearing, and the limit
// signed for it, so a report can print two histograms against two signs.

// DirectionLimit is one limit as signed for a direction, in force from its
// effective date (YYYY-MM-DD) until its end, if any.
type DirectionLimit struct {
	Value         int    `json:"value"`
	Unit          string `json:"unit"`
	Source        string `json:"source"`
	Jurisdiction  string `json:"jurisdiction,omitempty"`
	Note          string `json:"note,omitempty"`
	EffectiveFrom string `json:"effective_from,omitempty"`
	EffectiveTo   string `json:"effective_to,omitempty"`
}

// DirectionConfig is one of the radar's two directions.
type DirectionConfig struct {
	// Key is inbound or outbound, as the transit's direction reads.
	Key        string           `json:"key"`
	Name       string           `json:"name"`
	BearingDeg *float64         `json:"bearing_deg,omitempty"`
	Limits     []DirectionLimit `json:"limits,omitempty"`
}

// RadarDirections is the deployment's directions.
type RadarDirections struct {
	// InboundSign is the sign the sensor reports approaching traffic with:
	// 1 (the OPS243's default) or −1 for a sensor configured the other way.
	InboundSign int               `json:"inbound_sign"`
	Directions  []DirectionConfig `json:"directions"`
	// FootprintSpansLimitChange suppresses compliance for the deployment:
	// the beam's footprint covers a change of limit (posted-limits plan).
	FootprintSpansLimitChange bool `json:"footprint_spans_limit_change,omitempty"`
}

var directionJurisdictionRE = regexp.MustCompile(`^[A-Z]{2}-[A-Z0-9]{1,3}$`)

// Validate refuses a malformed set: the sign, unique inbound/outbound
// keys with names, and limits as Gerardus checks them.
func (r *RadarDirections) Validate() error {
	if r.InboundSign != 1 && r.InboundSign != -1 {
		return fmt.Errorf("directions.inbound_sign must be 1 or -1")
	}
	seen := map[string]bool{}
	for _, d := range r.Directions {
		if d.Key != TransitInbound && d.Key != TransitOutbound {
			return fmt.Errorf("directions: key must be inbound or outbound, not %q", d.Key)
		}
		if seen[d.Key] {
			return fmt.Errorf("directions: %s given twice", d.Key)
		}
		seen[d.Key] = true
		if d.Name == "" {
			return fmt.Errorf("directions: %s needs a name", d.Key)
		}
		for _, l := range d.Limits {
			ceiling := 90
			if l.Unit == "km/h" {
				ceiling = 150
			} else if l.Unit != "mph" {
				return fmt.Errorf("directions: %s limit unit must be mph or km/h", d.Key)
			}
			if l.Value <= 0 || l.Value > ceiling {
				return fmt.Errorf("directions: %s limit must be 1 to %d %s", d.Key, ceiling, l.Unit)
			}
			switch l.Source {
			case "sign", "city", "statutory", "operator":
			default:
				return fmt.Errorf("directions: %s limit source must be sign, city, statutory or operator", d.Key)
			}
			if l.Jurisdiction != "" && !directionJurisdictionRE.MatchString(l.Jurisdiction) {
				return fmt.Errorf("directions: %s jurisdiction must be ISO 3166-2", d.Key)
			}
			for _, day := range []string{l.EffectiveFrom, l.EffectiveTo} {
				if day == "" {
					continue
				}
				if _, err := time.Parse("2006-01-02", day); err != nil {
					return fmt.Errorf("directions: %s effective date %q must be YYYY-MM-DD", d.Key, day)
				}
			}
		}
	}
	return nil
}

// Direction maps a transit's sensor direction to the deployment's key,
// turning it round when the sensor reports approaching traffic negative.
func (r *RadarDirections) Direction(transit string) string {
	if r == nil || r.InboundSign != -1 {
		return transit
	}
	switch transit {
	case TransitInbound:
		return TransitOutbound
	case TransitOutbound:
		return TransitInbound
	}
	return transit
}

// Config returns the direction with a key, or nil.
func (r *RadarDirections) Config(key string) *DirectionConfig {
	if r == nil {
		return nil
	}
	for i := range r.Directions {
		if r.Directions[i].Key == key {
			return &r.Directions[i]
		}
	}
	return nil
}

func parseDirectionsColumn(v sql.NullString) (*RadarDirections, error) {
	if !v.Valid || v.String == "" {
		return nil, nil
	}
	var r RadarDirections
	if err := json.Unmarshal([]byte(v.String), &r); err != nil {
		return nil, fmt.Errorf("site config period directions: %w", err)
	}
	return &r, nil
}

func directionsColumn(r *RadarDirections) any {
	if r == nil {
		return nil
	}
	b, err := json.Marshal(r)
	if err != nil {
		return nil
	}
	return string(b)
}

// DirectionalTransit is one transit's start, speed (cosine-corrected where
// a site is given) and direction, with the deployment's directions in force
// then.
type DirectionalTransit struct {
	StartUnix  float64
	SpeedMPS   float64
	Direction  string // the deployment's key: inbound, outbound, mixed or unknown
	Directions *RadarDirections
}

// TransitDirections reads a period's transits with their directions, the
// same population as RadarObjectRollupRange's radar_data_transits source:
// the model version, the minimum speed and, given a site, the cosine
// correction of the config period covering each transit.
func (db *DB) TransitDirections(startUnix, endUnix int64, siteID int, modelVersion string, minSpeed float64) ([]DirectionalTransit, error) {
	if modelVersion == "" {
		modelVersion = "hourly-cron"
	}
	if minSpeed <= 0 {
		minSpeed = 2.2352
	}
	speedExpr := buildCosineSpeedExpr("rt.transit_max_speed")
	query := fmt.Sprintf(`
		SELECT rt.transit_start_unix, %s, rt.direction, scp.directions_json
		  FROM radar_data_transits rt
		  LEFT JOIN site_config_periods scp
		    ON scp.site_id = ?
		   AND rt.transit_start_unix >= scp.effective_start_unix
		   AND (scp.effective_end_unix IS NULL OR rt.transit_start_unix < scp.effective_end_unix)
		 WHERE rt.model_version = ?
		   AND %s > ?
		   AND rt.transit_start_unix BETWEEN ? AND ?
		 ORDER BY rt.transit_start_unix`, speedExpr, speedExpr)
	rows, err := db.Query(query, siteID, modelVersion, minSpeed, startUnix, endUnix)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cache := map[string]*RadarDirections{}
	var out []DirectionalTransit
	for rows.Next() {
		var t DirectionalTransit
		var raw sql.NullString
		if err := rows.Scan(&t.StartUnix, &t.SpeedMPS, &t.Direction, &raw); err != nil {
			return nil, err
		}
		if raw.Valid {
			d, ok := cache[raw.String]
			if !ok {
				if d, err = parseDirectionsColumn(raw); err != nil {
					return nil, err
				}
				cache[raw.String] = d
			}
			t.Directions = d
			t.Direction = d.Direction(t.Direction)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}
