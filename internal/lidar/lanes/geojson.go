package lanes

import (
	"encoding/json"
	"fmt"
)

// reservedProperties are Gerardus's GeoJSON properties that are not tags.
var reservedProperties = map[string]bool{"layer": true, "id": true, "kind": true, "author": true, "edited_at": true, "row_id": true, "model_id": true}

// FromGeoJSON reads the street-use features of Gerardus's
// GET /api/v1/sites/{l16}/geojson?layers=street_use&frame=site: each
// feature's id, row, kind, author, edit time and tags as properties, and
// site-frame [x, y, z] coordinates. Features of other layers are skipped.
func FromGeoJSON(data []byte) ([]Feature, error) { return FromGeoJSONLayer(data, "street_use") }

// FromGeoJSONLayer reads one layer's features from Gerardus's GeoJSON in
// the same shape: `street_use`, or `lane_proposals` for the proposals.
func FromGeoJSONLayer(data []byte, layerName string) ([]Feature, error) {
	var fc struct {
		Features []struct {
			ID       string `json:"id"`
			Geometry struct {
				Type        string          `json:"type"`
				Coordinates json.RawMessage `json:"coordinates"`
			} `json:"geometry"`
			Properties map[string]any `json:"properties"`
		} `json:"features"`
	}
	if err := json.Unmarshal(data, &fc); err != nil {
		return nil, fmt.Errorf("street-use GeoJSON: %w", err)
	}
	out := []Feature{}
	for i, f := range fc.Features {
		if layer, _ := f.Properties["layer"].(string); layer != layerName {
			continue
		}
		feature := Feature{FeatureID: f.ID, Tags: map[string]string{}}
		if id, ok := f.Properties["id"].(string); ok && id != "" {
			feature.FeatureID = id
		}
		feature.Kind, _ = f.Properties["kind"].(string)
		feature.RowID, _ = f.Properties["row_id"].(string)
		feature.Author, _ = f.Properties["author"].(string)
		feature.EditedAt, _ = f.Properties["edited_at"].(string)
		for k, v := range f.Properties {
			if reservedProperties[k] {
				continue
			}
			if s, ok := v.(string); ok {
				feature.Tags[k] = s
			} else {
				feature.Tags[k] = fmt.Sprint(v)
			}
		}
		var ring [][]float64
		switch f.Geometry.Type {
		case "LineString":
			if err := json.Unmarshal(f.Geometry.Coordinates, &ring); err != nil {
				return nil, fmt.Errorf("feature %d: %w", i, err)
			}
		case "Polygon":
			var rings [][][]float64
			if err := json.Unmarshal(f.Geometry.Coordinates, &rings); err != nil {
				return nil, fmt.Errorf("feature %d: %w", i, err)
			}
			if len(rings) > 0 {
				ring = rings[0]
				// GeoJSON closes a ring by repeating its first vertex.
				if n := len(ring); n > 1 && ring[0][0] == ring[n-1][0] && ring[0][1] == ring[n-1][1] {
					ring = ring[:n-1]
				}
			}
			feature.Closed = true
		case "Point":
			var p []float64
			if err := json.Unmarshal(f.Geometry.Coordinates, &p); err != nil {
				return nil, fmt.Errorf("feature %d: %w", i, err)
			}
			ring = [][]float64{p}
		default:
			continue
		}
		for _, p := range ring {
			z := 0.0
			if len(p) > 2 {
				z = p[2]
			}
			if len(p) < 2 {
				return nil, fmt.Errorf("feature %d: a coordinate needs x and y", i)
			}
			feature.XYZ = append(feature.XYZ, p[0], p[1], z)
		}
		out = append(out, feature)
	}
	return out, nil
}
