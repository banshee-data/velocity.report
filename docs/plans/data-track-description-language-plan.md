# Track Description Language and Description Interface

Status: Proposed
Purpose: Defines the Track Description Language (TDL) for querying the fused transit database, and the description interface for browsing and aggregating transit statistics.

Related: [Product Vision](../VISION.md)

---

## 1. Purpose

A **Track Description Language (TDL)** provides a natural-language query interface over the transit database. It allows users and report generators to express questions like:

- *"What percentage of eastbound vehicles exceed 30 mph between 07:00–09:00?"*
- *"Show transits where a vehicle passed within 1.5 m of a cyclist."*
- *"List outlier transits with speed above the p98 threshold."*
- *"Average speed profile for vehicles classified as lorry."*

The TDL is not SQL. It uses human-readable terms grounded in traffic-engineering vocabulary so that neighbourhood advocates, councillors, and community groups can describe what they want to see without learning a query language.

## 2. Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| **Syntax family** | Natural language | Target users are neighbourhood change-makers, not engineers. The system parses structured English clauses into parameterised queries. |
| **Execution target** | Go query builder → parameterised SQLite | Go translates parsed TDL into safe, parameterised SQL executed against SQLite views. No raw SQL is exposed. |
| **Schema exposure** | Abstract | Users see domain concepts (transit, speed, behaviour) not table names. The abstract schema (§3) maps to underlying storage via SQLite views. |
| **Aggregation model** | Precomputed velocity rollups | p50, p85, p98, and max are stored per-transit at write time. Dataset-wide aggregates are computed on demand over these precomputed values. |

### 2.1 Why Natural Language

SQL-like DSLs are powerful but exclude non-technical users. JSON filter objects suit programmatic access but are opaque in report templates. A natural-language syntax bridges both needs:

- **For users**: reads like a sentence — *"vehicles faster than 30 mph during morning peak"*.
- **For reports**: embeds directly in PDF template text — the generator evaluates TDL expressions inline.
- **For the API**: the Go server parses TDL strings into the same parameterised SQL that a JSON filter would produce, so both interfaces share one execution path.

### 2.2 Why Precomputed Velocity Rollups

The canonical velocity percentiles (p50, p85, p98, max) are stored per-transit at ingestion time rather than computed at query time:

- **p50** — median speed; typical behaviour for the transit.
- **p85** — 85th-percentile speed; traffic-engineering standard for design speed.
- **p98** — 98th-percentile speed; flags the top 2% high-speed outliers.
- **max** — absolute peak observed speed.

These align with the project-wide percentile palette defined in `DESIGN.md` §3.3 and the existing `lidar_tracks` columns (`p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps`, `peak_speed_mps`). Radar transits store `transit_max_speed`; p50/p85/p98 columns will be added when the fused transit schema is defined.

Precomputation avoids scanning per-frame observation rows at query time, keeping TDL response latency low on Raspberry Pi hardware.

## 3. Abstract Transit Schema

The TDL operates over an abstract **transit** concept rather than raw tables. Users never reference `radar_data_transits` or `lidar_tracks` directly.

```
transit {
  id              -- unique fused transit identifier
  timestamp       -- start time (UTC)
  duration_s      -- transit duration in seconds
  direction       -- inbound / outbound / unknown

  speed {
    p50_mph       -- median speed (precomputed)
    p85_mph       -- 85th percentile speed (precomputed)
    p98_mph       -- 98th percentile speed (precomputed)
    max_mph       -- absolute peak speed (precomputed)
    mean_mph      -- arithmetic mean speed
    profile[]     -- ordered per-frame speed samples (from LiDAR obs)
  }

  classification {
    category      -- pedestrian / vehicle / cyclist / other
    vehicle_class -- car / van / lorry / bus / motorcycle / unknown
    size {
      length_m, width_m, height_m
    }
    confidence    -- classifier confidence [0, 1]
  }

  behaviour {
    style         -- computed label (see §5): steady / accelerating / braking / erratic
    stopped       -- true if vehicle came to complete stop during transit
    yielded       -- true if vehicle slowed below yield threshold near conflict point
  }

  geometry {
    trail[]       -- polyline [(x, y, ts), ...]
    bbox[]        -- per-frame bounding box dimensions
  }

  context {
    nearest_object_distance_m  -- closest concurrent tracked object
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

### 3.1 Schema-to-Storage Mapping

| Abstract field | Source | Precomputed? |
|---------------|--------|-------------|
| `speed.p50_mph` | `lidar_tracks.p50_speed_mps` × 2.237 | ✅ per-track |
| `speed.p85_mph` | `lidar_tracks.p85_speed_mps` × 2.237 | ✅ per-track |
| `speed.p98_mph` | Fused transit table (new column) | ✅ per-transit |
| `speed.max_mph` | `MAX(lidar_tracks.peak_speed_mps, radar_data_transits.transit_max_speed)` × 2.237 | ✅ per-transit |
| `speed.profile[]` | `lidar_track_obs.speed_mps` ordered by `ts_unix_nanos` | ❌ read-time |
| `behaviour.style` | Derived from `speed.profile[]` shape (§5) | ✅ per-transit |
| `classification.*` | `lidar_tracks.object_class`, `lidar_tracks.object_confidence` | ✅ per-track |
| `geometry.trail[]` | `lidar_track_obs.(x, y, ts_unix_nanos)` | ❌ read-time |
| `context.*` | Computed from concurrent tracks at ingestion | ✅ per-transit |

## 4. Natural Language Syntax

### 4.1 Clause Structure

A TDL expression is an English sentence composed of optional clauses. Order is flexible; the parser recognises clause types by keyword anchors.

```
[subject] [filter] [during <time-range>] [heading <direction>] [show <fields>]
```

**Subject** — what to query. Defaults to `transits` if omitted.

| Subject | Meaning |
|---------|---------|
| `transits` | Individual fused transit records |
| `vehicles` | Transits classified as vehicle (any class) |
| `cars` / `lorries` / `buses` / `motorcycles` | Transits matching a specific vehicle class |
| `cyclists` | Transits classified as cyclist |
| `pedestrians` | Transits classified as pedestrian |

**Filter** — conditions on speed, behaviour, or context.

| Pattern | Meaning |
|---------|---------|
| `faster than <N> mph` | `speed.max_mph > N` |
| `slower than <N> mph` | `speed.max_mph < N` |
| `above p85` | `speed.max_mph > speed.p85_mph` (dataset-level p85) |
| `outliers` | `speed.max_mph > speed.p98_mph` (dataset-level p98) |
| `speeding` | `speed.max_mph > posted_limit` (site speed limit) |
| `braking` | `behaviour.style = 'braking'` |
| `erratic` | `behaviour.style = 'erratic'` |
| `stopped` | `behaviour.stopped = true` |
| `close to cyclist` | `context.nearest_object_class = 'cyclist' AND context.nearest_object_distance_m < 1.5` |
| `within <N> m of <class>` | `context.nearest_object_class = <class> AND context.nearest_object_distance_m < N` |

**Time range** — scopes the query temporally.

| Pattern | Meaning |
|---------|---------|
| `during morning peak` | `timestamp` between 07:00–09:00 |
| `during evening peak` | `timestamp` between 16:00–18:00 |
| `during school run` | `timestamp` between 08:15–08:45 or 15:00–15:30 |
| `between <HH:MM> and <HH:MM>` | Explicit time window |
| `on weekdays` | Monday–Friday |
| `on weekends` | Saturday–Sunday |
| `last 7 days` / `last 30 days` | Rolling date window |

**Direction** — filters by travel direction.

| Pattern | Meaning |
|---------|---------|
| `heading inbound` / `heading outbound` | `direction = 'inbound'` / `'outbound'` |
| `eastbound` / `westbound` / `northbound` / `southbound` | Cardinal direction (requires site calibration) |

### 4.2 Examples

```
vehicles faster than 30 mph during morning peak
```
→ All vehicle transits with `speed.max_mph > 30` between 07:00–09:00.

```
outliers heading inbound last 7 days
```
→ Inbound transits where `speed.max_mph` exceeds the dataset p98, from the last 7 days.

```
lorries braking during school run
```
→ Lorry transits with `behaviour.style = 'braking'` during 08:15–08:45 or 15:00–15:30.

```
cyclists close to vehicle between 16:00 and 18:00
```
→ Cyclist transits where a concurrent vehicle passed within 1.5 m, during evening peak.

```
percentage of vehicles speeding on weekdays
```
→ Aggregate: count of `speed.max_mph > posted_limit` / total vehicle count, weekdays only.

```
speed profile for erratic vehicles last 30 days show trail
```
→ Per-frame speed samples and trail polylines for transits labelled `erratic` in the last 30 days.

### 4.3 Aggregation Syntax

Prefixing a query with an aggregation keyword returns grouped statistics instead of individual transits.

| Prefix | Output |
|--------|--------|
| `count of` | Total matching transits |
| `percentage of` | Matching / total × 100 |
| `speed summary of` | p50, p85, p98, max across matching transits |
| `breakdown of` | Group by vehicle class, return counts per class |
| `hourly distribution of` | Histogram of matching transits by hour of day |

These aggregations operate over the precomputed per-transit p50/p85/p98/max values, so a `speed summary of vehicles during morning peak` computes dataset-level percentiles from the stored per-transit percentiles — no per-frame scan required.

## 5. Behaviour Vocabulary

The TDL supports natural-language terms for vehicle behaviours. These terms require a defined mapping from raw sensor data to labelled behaviour — a layer of abstraction between observation-level measurements and human-readable descriptions.

### 5.1 Labelling Levels

Translating sensor data into natural-language behaviour terms requires three levels of abstraction:

| Level | Name | Input | Output | Example |
|-------|------|-------|--------|---------|
| **L0 — Observation** | Per-frame measurement | Raw sensor readings | `(x, y, speed_mps, heading_rad, ts)` | A single LiDAR observation at frame 1042 |
| **L1 — Transit metric** | Per-transit aggregate | Ordered L0 observations | `p50_mph`, `p85_mph`, `p98_mph`, `max_mph`, `duration_s`, `speed_delta` | Precomputed velocity rollup for one vehicle pass |
| **L2 — Behaviour label** | Semantic classification | L1 metrics + speed profile shape | `steady`, `braking`, `erratic`, `stopped`, `yielded` | Human-readable driving-style tag |
| **L3 — Scene descriptor** | Cross-transit narrative | Multiple L2 labels + context | *"73% of vehicles exceed 30 mph during school run"* | Aggregate statement for reports |

**L0** already exists — `lidar_track_obs` and `radar_data` store per-frame data.

**L1** is partially implemented — `lidar_tracks` stores p50/p85/p95 and peak speed; `radar_data_transits` stores max/min speed. The fused transit schema will unify these with p50/p85/p98/max columns.

**L2** is the critical new layer. It requires:

1. A **speed-profile analyser** that classifies the shape of the `speed.profile[]` curve into behaviour labels.
2. A **conflict detector** that identifies proximity events between concurrent tracks.
3. A **stop detector** that flags transits where speed drops below a threshold (e.g. 2 mph) for a minimum duration.

**L3** is the TDL aggregation output — it composes L2 labels with time/direction/class filters to produce the natural-language statistics that appear in reports.

### 5.2 Behaviour Label Definitions

Each behaviour label maps to a concrete rule over L1 metrics and the speed profile:

| Label | Rule | Inputs |
|-------|------|--------|
| **steady** | Speed variance below threshold across the transit; no significant acceleration or deceleration events. | `std(profile[]) < σ_steady` |
| **accelerating** | Sustained speed increase: end speed exceeds start speed by more than a threshold. | `profile[last] - profile[first] > Δ_accel` |
| **braking** | Sustained speed decrease: start speed exceeds end speed by more than a threshold. | `profile[first] - profile[last] > Δ_brake` |
| **erratic** | High speed variance or multiple direction changes in acceleration; inconsistent driving pattern. | `std(profile[]) > σ_erratic` OR acceleration sign changes > N |
| **stopped** | Speed drops below stop threshold (e.g. 2 mph / 0.9 m/s) for at least `t_stop` seconds. | `min(profile[]) < v_stop AND dwell_time > t_stop` |
| **yielded** | Speed drops below yield threshold near a detected conflict point (e.g. junction, crossing). | `min(profile[window]) < v_yield` within proximity of conflict geometry |
| **close_pass** | Transit passes within a threshold distance of a concurrent cyclist or pedestrian. | `context.nearest_object_distance_m < d_close AND context.nearest_object_class IN ('cyclist', 'pedestrian')` |

Thresholds (`σ_steady`, `Δ_accel`, `v_stop`, `t_stop`, `d_close`, etc.) are site-configurable parameters stored alongside the site speed limit.

### 5.3 Syntax Abstraction Requirements

For the natural-language parser to resolve behaviour terms, it needs:

1. **A vocabulary registry** — a lookup table mapping natural-language tokens to abstract schema fields and filter conditions. E.g. `"speeding"` → `speed.max_mph > site.posted_limit`, `"erratic"` → `behaviour.style = 'erratic'`.

2. **Synonym handling** — common variations must resolve to the same query. E.g. `"braking"` / `"decelerating"` / `"slowing down"` → `behaviour.style = 'braking'`.

3. **Composability** — terms combine naturally. `"erratic vehicles faster than 40 mph"` applies both `behaviour.style = 'erratic'` AND `speed.max_mph > 40`.

4. **Unit awareness** — speeds accept `mph` or `km/h`; distances accept `m` or `ft`. The parser normalises to internal units (m/s, metres) before query execution.

5. **Aggregate qualifiers** — words like `"percentage"`, `"count"`, `"average"`, `"breakdown"` trigger aggregation mode rather than transit listing.

### 5.4 Data Labelling Requirements

Behaviour labels (L2) are derived, not manually applied. The labelling pipeline runs at transit finalisation:

1. **At transit close** (radar or LiDAR track completion), compute L1 velocity rollups (p50, p85, p98, max) and store in the fused transit record.
2. **Run the speed-profile analyser** over `speed.profile[]` to assign a `behaviour.style` label.
3. **Run the stop detector** to set `behaviour.stopped` and `behaviour.yielded` flags.
4. **Run the conflict detector** over concurrent tracks to populate `context.nearest_object_distance_m` and `context.nearest_object_class`.

This is distinct from the human-applied detection/quality labels (`user_label`, `quality_label`) defined in the LiDAR [label taxonomy](../lidar/terminology.md). Those labels evaluate *tracker correctness*; behaviour labels evaluate *driving style*.

## 6. Implementation Path

1. **Define the fused transit schema** as a Go struct and SQLite view joining `radar_data_transits`, `lidar_tracks`, and `lidar_track_obs`. Add p50/p85/p98/max columns to the fused transit table.
2. **Implement the behaviour labelling pipeline** (§5.4) — speed-profile analyser, stop detector, conflict detector. Store L2 labels in the fused transit record.
3. **Build the vocabulary registry** (§5.3) — map natural-language tokens to abstract schema filters. Start with the core vocabulary (§4.1 filter and subject tables) and expand from usage.
4. **Build the TDL parser** — parse natural-language strings into a structured filter tree; translate to parameterised SQL via the Go query builder.
5. **Expose a JSON API** — the web frontend and PDF generator post TDL strings; the server returns filtered transits or aggregated statistics.
6. **Wire the description interface** (§7) to the TDL API.

## 7. Description Interface

A web-based interface over the transit database that:

- **Lets users browse transits** with filtering, sorting, and drill-down.
- **Dynamically generates aggregate statistics** — driving style distributions, outlier counts, stop compliance, gap analysis.
- **Renders a vector-scene replay** of selected transits — bounding boxes moving through a 2D plan view.
- **Exports filtered datasets** as CSV for external analysis.

The description interface is the primary consumer of the TDL — every filter, aggregation, and export operation is expressed as a natural-language TDL string (§4) and executed via the JSON API (§6, step 5).
