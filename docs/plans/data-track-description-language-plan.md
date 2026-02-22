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
| **Aggregation model** | Per-transit max speed + on-demand dataset percentiles | Each transit stores its own `max_speed`. Dataset-level percentiles (p50, p85, p98) are computed at query time over the filtered set of transit max speeds. |

### 2.1 Why Natural Language

SQL-like DSLs are powerful but exclude non-technical users. JSON filter objects suit programmatic access but are opaque in report templates. A natural-language syntax bridges both needs:

- **For users**: reads like a sentence — *"vehicles faster than 30 mph during morning peak"*.
- **For reports**: embeds directly in PDF template text — the generator evaluates TDL expressions inline.
- **For the API**: the Go server parses TDL strings into the same parameterised SQL that a JSON filter would produce, so both interfaces share one execution path.

### 2.2 Speed Measurement Model

There are two distinct kinds of speed percentile in the system. Confusing them leads to incorrect schema design:

**Per-track profile percentiles** — computed from the ordered speed observations *within a single track*. If a LiDAR track has 100 frames of `speed_mps`, the p50/p85/p95 are percentiles of those 100 readings. These describe the speed *profile shape* of one vehicle pass (e.g. "this car was mostly doing 28 mph but briefly hit 35 mph"). The existing `lidar_tracks` table stores these as `p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps`. They are useful for behaviour classification (§5) but are *not* the percentiles that traffic engineers reference.

**Dataset-level percentiles** — computed across the *max speeds of many transits*. If a street has 1,000 vehicle transits in a week, the p85 is the 85th-percentile of those 1,000 max-speed values. This is the traffic-engineering standard for design speed. These percentiles **cannot be precomputed per-transit** because they depend on which transits are included — a TDL filter that restricts to "weekday mornings" or "lorries only" changes the population and therefore the percentiles.

The TDL stores **one scalar speed per transit**: `max_speed_mph` (the absolute peak observed speed for that vehicle pass). Dataset-level aggregates (p50, p85, p98, max) are computed at query time over the `max_speed_mph` values of the filtered transit set. On a Raspberry Pi with SQLite, computing percentiles over a sorted column of a few thousand rows takes single-digit milliseconds — precomputation is unnecessary.

## 3. Abstract Transit Schema

The TDL operates over an abstract **transit** concept rather than raw tables. Users never reference `radar_data_transits` or `lidar_tracks` directly.

```
transit {
  id              -- unique fused transit identifier
  timestamp       -- start time (UTC)
  duration_s      -- transit duration in seconds
  direction       -- inbound / outbound / unknown

  speed {
    max_mph       -- absolute peak speed for this transit (stored per-transit)
    mean_mph      -- arithmetic mean speed across the transit (stored per-transit)
    profile[]     -- ordered per-frame speed samples (from LiDAR obs; read-time)
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

**Dataset-level aggregates** (p50, p85, p98 of `speed.max_mph`) are not fields on the transit record. They are computed at query time over the filtered result set. A TDL query like `speed summary of vehicles during morning peak` computes these from the matching transits' `max_mph` values.

### 3.1 Schema-to-Storage Mapping

| Abstract field | Source | Stored per-transit? |
|---------------|--------|---------------------|
| `speed.max_mph` | `MAX(lidar_tracks.peak_speed_mps, radar_data_transits.transit_max_speed)` × 2.237 | ✅ |
| `speed.mean_mph` | `lidar_tracks.avg_speed_mps` × 2.237 | ✅ |
| `speed.profile[]` | `lidar_track_obs.speed_mps` ordered by `ts_unix_nanos` | ❌ read-time join |
| `behaviour.style` | Derived from `speed.profile[]` shape (§5) | ✅ |
| `classification.*` | `lidar_tracks.object_class`, `lidar_tracks.object_confidence` | ✅ |
| `geometry.trail[]` | `lidar_track_obs.(x, y, ts_unix_nanos)` | ❌ read-time join |
| `context.*` | Computed from concurrent tracks at ingestion | ✅ |

Note: the existing `lidar_tracks.p50_speed_mps`, `p85_speed_mps`, `p95_speed_mps` columns are *per-track profile percentiles* — percentiles of the speed observations within a single track. They describe the speed profile shape and feed the behaviour classifier (§5), but are not exposed in the TDL abstract schema because users expect p85/p98 to mean dataset-level aggregates.

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
| `above p85` | `speed.max_mph > dataset_p85` (dataset-level p85, computed on demand) |
| `outliers` | `speed.max_mph > dataset_p98` (dataset-level p98, computed on demand) |
| `speeding` | `speed.max_mph > posted_limit` (site speed limit) |
| `braking` | `behaviour.style = 'braking'` |
| `erratic` | `behaviour.style = 'erratic'` |
| `stopped` | `behaviour.stopped = true` |
| `close to cyclist` | `context.nearest_object_class = 'cyclist' AND context.nearest_object_distance_m < d_close` (default 1.5 m; site-configurable) |
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

These aggregations run at query time over the filtered transit set. For `speed summary`, the server collects `max_speed_mph` from all matching transits, sorts the values, and computes p50/p85/p98/max using floor-based indexing (consistent with the existing `ComputeSpeedPercentiles` function in `l6objects`). This is a single ordered scan — not a per-frame observation scan — so latency stays low even on constrained hardware. See §6 for index strategy and performance constraints.

## 5. Behaviour Vocabulary

The TDL supports natural-language terms for vehicle behaviours. These terms require a defined mapping from raw sensor data to labelled behaviour — a layer of abstraction between observation-level measurements and human-readable descriptions.

### 5.1 Labelling Levels

Translating sensor data into natural-language behaviour terms requires three levels of abstraction:

| Level | Name | Input | Output | Example |
|-------|------|-------|--------|---------|
| **L0 — Observation** | Per-frame measurement | Raw sensor readings | `(x, y, speed_mps, heading_rad, ts)` | A single LiDAR observation at frame 1042 |
| **L1 — Transit metric** | Per-transit scalar | Ordered L0 observations | `max_mph`, `mean_mph`, `duration_s`, `speed_delta`, per-track profile percentiles | Stored per-transit; profile percentiles feed behaviour classifier |
| **L2 — Behaviour label** | Semantic classification | L1 metrics + speed profile shape | `steady`, `braking`, `erratic`, `stopped`, `yielded` | Human-readable driving-style tag |
| **L3 — Scene descriptor** | Cross-transit narrative | Multiple L2 labels + context | *"73% of vehicles exceed 30 mph during school run"* | Aggregate statement for reports; percentiles (p50/p85/p98) computed at query time |

**L0** already exists — `lidar_track_obs` and `radar_data` store per-frame data.

**L1** is partially implemented — `lidar_tracks` stores per-track profile percentiles (p50/p85/p95) and peak speed; `radar_data_transits` stores max/min speed. The fused transit record will carry `max_speed_mph` and `mean_speed_mph` as per-transit scalars. Per-track profile percentiles remain internal to the behaviour classifier and are not exposed via the TDL.

**L2** is the critical new layer. It requires:

1. A **speed-profile analyser** that classifies the shape of the `speed.profile[]` curve into behaviour labels.
2. A **conflict detector** that identifies proximity events between concurrent tracks.
3. A **stop detector** that flags transits where speed drops below a threshold (e.g. 2 mph) for a minimum duration.

**L3** is the TDL aggregation output — it composes L2 labels with time/direction/class filters to produce the natural-language statistics that appear in reports.

### 5.2 Behaviour Label Definitions

Each behaviour label maps to a concrete rule over L1 metrics and the speed profile:

| Label | Rule | Inputs |
|-------|------|--------|
| **steady** | Speed variance below threshold across the transit; no significant acceleration or deceleration events. The profile should be smoothed (e.g. moving average) before variance calculation to avoid false classification from sensor noise. | `std(smooth(profile[])) < σ_steady` |
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

1. **At transit close** (radar or LiDAR track completion), store `max_speed_mph` and `mean_speed_mph` in the fused transit record. Per-track profile percentiles (p50/p85/p95 of observations within the track) are stored internally for the behaviour classifier but not in the abstract transit schema.
2. **Run the speed-profile analyser** over `speed.profile[]` to assign a `behaviour.style` label.
3. **Run the stop detector** to set `behaviour.stopped` and `behaviour.yielded` flags.
4. **Run the conflict detector** over concurrent tracks to populate `context.nearest_object_distance_m` and `context.nearest_object_class`.

This is distinct from the human-applied detection/quality labels (`user_label`, `quality_label`) defined in the LiDAR [label taxonomy](../lidar/terminology.md). Those labels evaluate *tracker correctness*; behaviour labels evaluate *driving style*.

## 6. Storage, Indexing, and Retrieval

### 6.1 SQLite as the Query Engine

SQLite is the project's single database (see `ARCHITECTURE.md`). The existing deployment runs on Raspberry Pi 4 with WAL mode, `PRAGMA synchronous = NORMAL`, and `busy_timeout = 30000`. The TDL query engine builds on this — no additional database is needed.

SQLite's strengths for TDL:

- **Single-file portability** — the transit database ships with the deployment; no external service.
- **Sorted-index scans** — `ORDER BY max_speed_mph` over a B-tree index gives O(n) percentile computation without materialising a temp table.
- **Window functions** — `PERCENT_RANK()`, `NTILE()`, and `ROW_NUMBER()` are available (SQLite ≥ 3.25; the project bundles 3.51.2 via `modernc.org/sqlite v1.44.3`).
- **Parameterised queries** — the Go query builder emits `?`-parameterised SQL, avoiding injection and enabling prepared-statement caching.

SQLite's constraints to design around:

- **No concurrent writers** — WAL mode allows one writer + many readers, but long-running TDL aggregations must not block transit ingestion. Mitigation: TDL queries run on a read-only connection; ingestion uses a separate write connection.
- **No server-side cursor streaming** — large result sets are materialised in memory. Mitigation: TDL list queries are paginated (default 100 rows); aggregation queries return scalar/histogram results, not row sets.
- **No built-in percentile function** — `PERCENTILE_CONT` is not in base SQLite. Mitigation: use `NTILE(100)` or sorted-index offset to compute percentiles in SQL, or compute in Go from a sorted `max_speed_mph` slice (matching the existing `ComputeSpeedPercentiles` pattern).

### 6.2 Fused Transit Table

The TDL operates over a **materialised fused transit table**, not a view. A view joining `radar_data_transits`, `lidar_tracks`, and `lidar_track_obs` would require the join on every query. Instead, the fused transit record is written at transit-finalisation time:

```sql
CREATE TABLE IF NOT EXISTS fused_transits (
    transit_id      INTEGER PRIMARY KEY AUTOINCREMENT,
    timestamp_unix  DOUBLE NOT NULL,          -- transit start (unix epoch)
    duration_s      REAL,                     -- transit duration
    direction       TEXT,                     -- inbound / outbound / unknown
    max_speed_mph   REAL NOT NULL,            -- peak speed (primary query column)
    mean_speed_mph  REAL,                     -- arithmetic mean speed
    category        TEXT,                     -- pedestrian / vehicle / cyclist / other
    vehicle_class   TEXT,                     -- car / van / lorry / bus / motorcycle / unknown
    behaviour_style TEXT,                     -- steady / accelerating / braking / erratic
    stopped         INTEGER DEFAULT 0,        -- 1 if vehicle came to complete stop
    yielded         INTEGER DEFAULT 0,        -- 1 if vehicle slowed below yield threshold
    nearest_obj_distance_m REAL,              -- closest concurrent object distance
    nearest_obj_class      TEXT,              -- class of nearest concurrent object
    radar_transit_id       INTEGER,           -- FK → radar_data_transits
    lidar_track_id         TEXT,              -- FK → lidar_tracks
    fusion_confidence      REAL,              -- quality of radar/LiDAR association
    created_at      DOUBLE DEFAULT (UNIXEPOCH('subsec'))
);
```

This table is the single scan target for all TDL filter and aggregation queries. Per-frame data (`speed.profile[]`, `geometry.trail[]`) is fetched via a secondary join to `lidar_track_obs` only when requested by a `show profile` or `show trail` clause.

### 6.3 Index Strategy

Indexes are designed for the TDL query patterns described in §4:

```sql
-- Primary query: filter by time + speed
CREATE INDEX idx_fused_time_speed
    ON fused_transits (timestamp_unix, max_speed_mph);

-- Filter by direction within a time range
CREATE INDEX idx_fused_time_direction
    ON fused_transits (timestamp_unix, direction);

-- Filter by classification
CREATE INDEX idx_fused_category
    ON fused_transits (category, vehicle_class);

-- Filter by behaviour
CREATE INDEX idx_fused_behaviour
    ON fused_transits (behaviour_style);

-- Aggregation: sorted speed for percentile computation
CREATE INDEX idx_fused_speed
    ON fused_transits (max_speed_mph);
```

The composite `(timestamp_unix, max_speed_mph)` index covers the most common TDL pattern: time-range filter + speed threshold or aggregation. SQLite can scan the index in `max_speed_mph` order within a time range to compute percentiles without a separate sort.

### 6.4 Query Patterns

**List query** — returns matching transits with pagination:

```sql
SELECT transit_id, timestamp_unix, max_speed_mph, category, vehicle_class,
       behaviour_style, direction
FROM fused_transits
WHERE timestamp_unix BETWEEN ?1 AND ?2
  AND category = 'vehicle'
  AND max_speed_mph > ?3
ORDER BY timestamp_unix DESC
LIMIT ?4 OFFSET ?5;
```

**Speed summary** — computes dataset-level percentiles over filtered max speeds:

```sql
-- Approach A: NTILE window function (pure SQL)
SELECT
    MAX(CASE WHEN tile = 50  THEN max_speed_mph END) AS p50,
    MAX(CASE WHEN tile = 85  THEN max_speed_mph END) AS p85,
    MAX(CASE WHEN tile = 98  THEN max_speed_mph END) AS p98,
    MAX(max_speed_mph) AS max
FROM (
    SELECT max_speed_mph,
           NTILE(100) OVER (ORDER BY max_speed_mph) AS tile
    FROM fused_transits
    WHERE timestamp_unix BETWEEN ?1 AND ?2
      AND category = 'vehicle'
);

-- Approach B: sorted slice in Go (matches existing ComputeSpeedPercentiles)
-- 1. SELECT max_speed_mph FROM fused_transits WHERE ... ORDER BY max_speed_mph
-- 2. Go code picks p50 = speeds[n/2], p85 = speeds[floor(n*0.85)], etc.
```

Approach B is simpler and matches the existing pattern in `internal/lidar/l6objects/classification.go`. For small-to-medium result sets (< 50,000 transits — roughly a year of data on a residential street at ~150 transits/day), the sorted slice fits comfortably in Raspberry Pi memory.

**Count / percentage** — scalar aggregates:

```sql
SELECT
    COUNT(*) AS total,
    COUNT(CASE WHEN max_speed_mph > ?3 THEN 1 END) AS exceeding
FROM fused_transits
WHERE timestamp_unix BETWEEN ?1 AND ?2
  AND category = 'vehicle';
```

**Hourly distribution** — histogram:

```sql
SELECT
    CAST(strftime('%H', timestamp_unix, 'unixepoch') AS INTEGER) AS hour,
    COUNT(*) AS count
FROM fused_transits
WHERE timestamp_unix BETWEEN ?1 AND ?2
  AND category = 'vehicle'
GROUP BY hour
ORDER BY hour;
```

### 6.5 Volume Estimates and Performance

| Metric | Estimate | Notes |
|--------|----------|-------|
| Transits per day | 100–500 | Residential street; varies by traffic volume |
| Transits per year | ~50,000 | Upper bound for a busy street |
| Row size (fused_transits) | ~200 bytes | Fixed columns, no BLOBs |
| Table size per year | ~10 MB | Well within Raspberry Pi SD card capacity |
| Percentile query time | < 50 ms | Sorted index scan over ≤ 50K rows |
| Filtered list query time | < 10 ms | Composite index covers time + speed + category |

These estimates assume the `fused_transits` table contains only scalar per-transit data. Per-frame observations remain in `lidar_track_obs` (which may be significantly larger) and are joined only for `show profile` / `show trail` requests.

### 6.6 Read/Write Separation

The Raspberry Pi runs a single Go process handling both sensor ingestion and HTTP API. To avoid write contention:

- **Write path**: transit finalisation inserts into `fused_transits` using the main write connection (WAL mode allows concurrent readers).
- **Read path**: TDL queries run on a separate read-only connection (`?mode=ro`). This ensures long-running aggregation queries never block ingestion.
- **Cache**: prepared statements are cached per-connection. The Go query builder re-uses a small set of parameterised query templates, so the SQLite query planner runs once per template.

## 7. Implementation Path

1. **Define the fused transit schema** as a Go struct and SQLite view joining `radar_data_transits`, `lidar_tracks`, and `lidar_track_obs`. Per-transit columns: `max_speed_mph`, `mean_speed_mph`, `direction`, `behaviour_style`, `classification`, `context`. No per-transit percentile columns — p50/p85/p98 are query-time aggregates.
2. **Implement the behaviour labelling pipeline** (§5.4) — speed-profile analyser, stop detector, conflict detector. Store L2 labels in the fused transit record.
3. **Build the vocabulary registry** (§5.3) — map natural-language tokens to abstract schema filters. Start with the core vocabulary (§4.1 filter and subject tables) and expand from usage.
4. **Build the TDL parser** — parse natural-language strings into a structured filter tree; translate to parameterised SQL via the Go query builder.
5. **Expose a JSON API** — the web frontend and PDF generator post TDL strings; the server returns filtered transits or aggregated statistics.
6. **Wire the description interface** (§8) to the TDL API.

## 8. Description Interface

A web-based interface over the transit database that:

- **Lets users browse transits** with filtering, sorting, and drill-down.
- **Dynamically generates aggregate statistics** — driving style distributions, outlier counts, stop compliance, gap analysis.
- **Renders a vector-scene replay** of selected transits — bounding boxes moving through a 2D plan view.
- **Exports filtered datasets** as CSV for external analysis.

The description interface is the primary consumer of the TDL — every filter, aggregation, and export operation is expressed as a natural-language TDL string (§4) and executed via the JSON API (§7, step 5). The API also accepts structured JSON filter objects for programmatic access; both input formats compile to the same parameterised SQL.
