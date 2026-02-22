# Track Description Language and Description Interface

Status: Proposed
Purpose: Defines the Track Description Language (TDL) for querying the fused transit database, and the description interface for browsing and aggregating transit statistics.

Related: [Product Vision](../VISION.md)

---

## 1. Purpose

A **Track Description Language (TDL)** provides a textual query interface over the transit database. It allows users and report generators to express questions like:

- *"What percentage of eastbound vehicles exceed 30 mph between 07:00–09:00?"*
- *"Show transits where a vehicle passed within 1.5 m of a cyclist."*
- *"List outlier transits with speed above the p98 threshold."*
- *"Average speed profile for vehicles classified as lorry."*

## 2. Design Considerations

The language sits between raw SQL and a full programming language. Key decisions:

| Decision | Options | Notes |
|----------|---------|-------|
| **Syntax family** | SQL-like DSL / JSON filter objects / natural-language-to-SQL | SQL-like is familiar to traffic engineers; JSON is easier to generate from UI controls |
| **Execution target** | SQLite views / Go query builder / embedded interpreter | SQLite views maximise portability; Go builder integrates with existing API |
| **Schema exposure** | Abstract (transit, speed, class) / raw (table.column) | Abstract schema insulates users from storage changes |
| **Aggregation model** | Pre-computed percentiles / on-demand window functions | Hybrid — store p50/p85/p98 per-track, compute ad-hoc aggregates on demand |

## 3. Proposed Schema Concepts

The TDL operates over an abstract **transit schema** rather than raw tables:

```
transit {
  id              -- unique transit identifier
  timestamp       -- start time (UTC)
  duration_s      -- transit duration in seconds
  direction       -- inbound / outbound / unknown

  speed {
    peak_mph      -- maximum observed speed
    mean_mph      -- average speed across transit
    p85_mph       -- 85th percentile (if multi-sample)
    profile[]     -- ordered speed samples (from LiDAR obs)
  }

  classification {
    category      -- pedestrian / vehicle / cyclist / other
    vehicle_class -- car / van / lorry / bus / motorcycle / unknown
    size {
      length_m, width_m, height_m
    }
    confidence    -- classifier confidence [0, 1]
  }

  geometry {
    trail[]       -- polyline [(x, y, ts), ...]
    bbox[]        -- per-frame bounding box dimensions
  }

  context {
    nearest_object_distance_m  -- closest concurrent object
    nearest_object_class       -- class of nearest object
    lane_position              -- estimated lateral offset
  }

  sensor {
    radar_transit_id   -- link to radar_data_transits
    lidar_track_id     -- link to lidar_tracks
    fusion_confidence  -- quality of radar/LiDAR association
  }
}
```

## 4. Query Examples

The following use SQL-like pseudocode to illustrate TDL intent. The actual syntax (SQL-like DSL, JSON filters, or natural-language) is a design decision; these examples show the *semantics* the language must support.

```
-- Percentage exceeding 30 mph, morning peak
SELECT
  COUNT(CASE WHEN speed.peak_mph > 30 THEN 1 END) * 100.0 / COUNT(*)
FROM transit
WHERE timestamp BETWEEN '07:00' AND '09:00'

-- Outlier transits (above p98 for the dataset)
SELECT * FROM transit
WHERE speed.peak_mph > p98(speed.peak_mph)

-- Close passes to cyclists
SELECT t1.id, t1.speed.peak_mph, t1.context.nearest_object_distance_m
FROM transit t1
WHERE t1.context.nearest_object_class = 'cyclist'
  AND t1.context.nearest_object_distance_m < 1.5

-- Speed profile for lorries
SELECT speed.profile FROM transit
WHERE classification.vehicle_class = 'lorry'
```

## 5. Implementation Path

1. **Define the abstract transit schema** as a Go struct and SQLite view joining `radar_data_transits`, `lidar_tracks`, and `lidar_track_obs`.
2. **Build a JSON filter API** — the web frontend posts filter objects; the Go server translates them to parameterised SQL. This avoids exposing raw SQL to users.
3. **Add a TDL text parser** (optional, later) — a lightweight DSL that compiles to the same parameterised SQL, for use in report templates and CLI queries.
4. **Expose an aggregation endpoint** — given a filter, return grouped statistics (percentiles, counts, histograms) suitable for rendering in the description interface or injecting into PDF reports.

## 6. Description Interface

A web-based interface over the transit database that:

- **Lets users browse transits** with filtering, sorting, and drill-down.
- **Dynamically generates aggregate statistics** — driving style distributions, outlier counts, stop compliance, gap analysis.
- **Renders a vector-scene replay** of selected transits — bounding boxes moving through a 2D plan view.
- **Exports filtered datasets** as CSV for external analysis.

The description interface is the primary consumer of the TDL — every filter, aggregation, and export operation is expressed through the abstract transit schema (§3) and executed via the JSON filter API (§5, step 2).
