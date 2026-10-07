# Posted speed limits on the vector scene plan

How velocity.report will record posted speed limits and apply them to measured speeds, once the
vector scene map gives a site its road geometry. Nothing is stored until then, because a single
limit per site, or per site configuration period, describes the wrong thing.

- **Status:** Proposed; implementation waits for the vector scene map (v1.0)
- **Canonical:** [3D vector scene map](../lidar/architecture/vector-scene-map.md)
- **Layers:** Vector scene roadway context, radar and LiDAR speed reports, site configuration
- **Related:** [Behaviour analytics plan, Section 10.1](lidar-behaviour-analytics-plan.md#101-roadway-context-does-not-exist-yet-including-the-speed-limit), [Speed limit schedules](../radar/architecture/speed-limit-schedules.md), [Cosine correction spec](../radar/architecture/site-config-cosine-correction-spec.md), [Traffic description language plan](data-traffic-description-language-plan.md)

## Where things stand

- No posted limit exists in the schema. Migration 000014 removed `speed_limit` and
  `speed_limit_note` from `site`, to be reintroduced later.
- The PDF report takes `ReportConfig.SpeedLimit` per request: an integer with no unit, printed
  beside a free-text note. The report's own sample note shows why one number is not enough: its
  location is signed 35 mph, reduced to 25 mph while school children are present.
- The web `Site` type and the reports page still read `site.speed_limit`, which the API no longer
  returns, so the page shows an empty value.
- A first attempt (#685) added a limit, its unit and a jurisdiction to `site_config_periods`. It
  was withdrawn before merging, for the two reasons below.

## Why one limit per site is the wrong model

### A limit is signed in a unit

Signs in the US and the UK show mph; most of the rest of the world uses km/h. The unit is part of
the limit, not a display preference: a 25 mph limit is exactly 25 mph, and 40.2336 km/h is a
conversion of it.

- Store a limit as signed: the number on the sign and its unit, `mph` or `km/h`. A converted value
  is never the record.
- Compare in exact arithmetic. Convert the measured speed and the limit to m/s with the exact
  factors (1 mph is 0.44704 m/s, 1 km/h is 1/3.6 m/s), so 25 mph is 11.176 m/s.
- Report in the sign's unit, whatever the site's display units. A report on a 25 mph street says
  "over 25 mph", never "over 40.2 km/h".
- Percentile benchmarks against a limit (p85, p98) stay aggregates over a population of vehicle
  maximum speeds, taken per limit segment.

### A street carries more than one limit

Within one sensor's view, the limit can change:

- **At a sign.** One limit applies before the sign and another after it, so a sign inside the
  field of view splits the road there.
- **By direction.** Each direction of travel can carry its own limit, with its signs at different
  places.
- **By lane.** Some roads post a different limit per lane.
- **By time.** School, work and park zones change the limit during set hours, per the
  [schedules design](../radar/architecture/speed-limit-schedules.md).
- **By date.** Limits are changed, and a report must use the one in force when each speed was
  measured.

A scalar per site, or per configuration period, attributes one limit to every speed in the view.
That is wrong wherever a sign falls inside it.

## The model: limits on the road geometry

The vector scene map describes the road surface as Ground polygons, one per lane at LOD 1, and
records signs as scene features (`TYPE_SIGN`, a Volume `sign_cluster`). Limits attach there.

- **Limit segment.** A stretch of road for one direction of travel, optionally one lane, between
  two limit changes. It carries the signed value and unit, the jurisdiction (an ISO 3166-2 code
  where one fits), effective from and to dates, an optional schedule, and its source.
- **Limit change.** A point on the road's centreline for its direction, held as an arc length
  along the road. It comes from the sign's scene feature where the sensor sees the sign, or from
  the operator where it does not. A segment's limit holds from one change to the next, and is
  open-ended past the scene's edge.
- **Source.** Each limit records where it came from: entered by an operator, read from a sign the
  scene shows, or an OpenStreetMap `maxspeed` prior. A prior is a suggestion until someone
  confirms it, and a conflict between sources is kept rather than silently resolved.
- **Unsigned stretches.** Where no sign governs a stretch, the limit is unknown, not inferred. A
  statutory default, such as a built-up area's general limit, can be recorded explicitly, marked
  statutory with its jurisdiction, so a report can tell a posted limit from a default.

## Applying a limit to a measured speed

### LiDAR

A track has a position at every sample. Each speed sample takes the limit of the segment that
contains its position, for its direction of travel, at its capture time. A transit that crosses a
sign is measured against each segment separately: its speed within each segment against that
segment's limit. Comparing one maximum speed with one limit across a sign would be wrong.

### Radar

The OPS243 measures speed along its beam, with no position along the road. A radar speed takes
the limit of the segment that the beam's footprint on the road lies in, for the direction it
measures, and site setup records that footprint against the scene. If a sign falls inside the
footprint for a direction, the radar cannot tell which limit a speed was measured under, so
compliance for that direction is suppressed with that reason, never guessed.

### Reports and the API

- Every compliance figure and benchmark carries the segment, its limit as signed, the
  jurisdiction and the effective date it used.
- A report covering a period in which a limit changed splits at the change.
- The report's per-request `SpeedLimit` stays until the scene can supply limits. A report that uses
  it says the limit was operator-supplied and applies one value to the whole view.

## Implementation, once the vector scene exists

1. **Storage.** A limit-segment table keyed to the scene's road feature and direction, holding
   the signed value and unit, the jurisdiction, effective dates, the arc-length range along the
   road, the source and an optional schedule. The unit is `mph` or `km/h`, and the value and
   unit are set together or not at all.
2. **Attribution.** One function from a position, heading and capture time to the governing limit,
   or to the reason there is none: no segment, an unknown limit, or an ambiguous radar
   footprint. The LiDAR and radar paths share it.
3. **Site setup.** Record the radar beam footprint against the scene, and enter or confirm limits
   and their change points, with OpenStreetMap priors offered as suggestions.
4. **Reports.** Compliance per segment, figures in the sign's unit, splits at limit changes, and
   the provenance of every limit used.
5. **Tests.**
   - A street with a sign mid-view, giving two segments.
   - Opposite directions with different limits.
   - A school-zone schedule.
   - A limit changed during a report's period.
   - A radar footprint spanning a sign, which is suppressed.
   - An mph site and a km/h site.
   - Exact conversion at the boundary: 25 mph is 11.176 m/s.

The behaviour analytics `legal` benchmarks, speed relative to the posted limit among them, depend
on this attribution and stay unavailable until it exists.

## Open questions

- Who confirms a limit read from a sign or an OpenStreetMap prior, and in which tool?
- How are variable message signs and temporary limits, such as road works, entered?
- Should limits be recorded per lane from the start, or per direction until a site needs lanes?
- Do radar-only sites, with no LiDAR scene, get a minimal hand-drawn road line to hang limits on?
