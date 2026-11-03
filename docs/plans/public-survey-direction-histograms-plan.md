# Public survey site: speed histograms by direction plan

How a survey page on the public site shows a recording's vehicle speeds for each direction of
travel, each against that direction's own posted limit, from the per-direction summaries the scene
export writes, with one chart form on wide screens (two directions mirrored about a shared speed
axis) and stacked small multiples on narrow ones, and what the page shows when a recording has no
directions or too few vehicles in one of them.

- **Status:** Proposed; waits on the scene export's per-direction summaries (the reporting plan's
  Item 4, or its heading fallback first). Not Active (D-32). Supersedes the survey-site half (§6) of
  the [directional speed reporting plan](platform-directional-speed-reporting-plan.md), reconciled
  November 3, 2026
- **Decisions:** relies on D-11 and D-17 (SVG charts; the public pages' dependency-free SVG), D-18
  (nearest-rank percentiles computed upstream, never from buckets), D-34 (the public site leaves the
  repository in two steps: this work stays in `public_html/` until then and changes no export
  format on its own), D-38, D-39 (street data outside git)
- **Layers:** public site (`public_html/`: Eleventy data, Nunjucks includes, the survey chart and
  model modules), and the scene export's summary contract as its input
- **Target:** unscheduled; proposed as the v0.7.19 slot
- **Companion plans:** [speed by direction and lane](platform-speed-by-direction-reporting-plan.md)
  (the data), [lane snapshot and attribution](lidar-lane-snapshot-and-attribution-plan.md),
  [web scene export](lidar-web-scene-export-plan.md) (the vectors file and §12 context),
  [platform vocabulary, surveys](platform-vocabulary-and-data-model-plan.md)
- **Canonical:** [Frontend and visualisation design language](../ui/DESIGN.md) (§4.3, the speed
  summary contract)

## Motivation

A survey page shows one histogram of vehicle speeds against one posted limit for the site. Where
the two directions of the street are signed differently, or the sensor sees a cross street, that
one limit is wrong for part of the traffic, and the page's share above the limit says something
untrue about one direction. Readers of a survey page are residents and officials deciding about one
street; "eastbound traffic runs 4 mph over its 25 mph sign, westbound runs at its 30 mph sign" is the
finding they need, and the page cannot show it.

The export side is settled by the reporting plan: each recording's speed summary gains a list of
directions, each with its own limit, counts, percentiles and histogram. This plan is the page.

## Current state

- **One speed card per recording** (`public_html/src/_includes/survey/family-speed.njk`): the
  histogram with the limit line and over-limit shading, a legend, an expandable section with the
  percentile strip and the bands above the limit, the posted limit with a "LAW" tag, key numbers
  (`p50`, `p85`, `p98`, `max`), "Chart PNG" and `n` in the footer.
- **Charts are SVG strings** built at build time and in the browser from one module
  (`public_html/src/js/survey-charts.js`): `histogramSvg`, `percentileStripSvg`,
  `percentileRowsSvg` and `sparklineSvg`, with `speedScale` and `shareCeiling` giving a shared axis
  and ceiling across small multiples; the histogram takes its limit, scale, ceiling and compact
  variant as options. Colours are attributes, so a saved chart keeps them.
- **The model** (`public_html/src/js/survey-model.js`): `summariseSpeed(raw)` reads
  `vehicle_speed` (`bucket_size`, `track_count`, `p50`, `p85`, `p98`, `max`, `histogram` of
  `start_mph` and `count`); `validLimit`, `shareAtOrAbove` (exact only when the limit lies on a
  bucket edge), `excessBands`, `speedFinding`, and the compare helpers.
- **The limit is site-level**: `speed_limit_mph` on the site's mark in
  `tools/s2-archive/map-marks.json`, carried through `scenes.json` to `speedLimitMph`
  (`public_html/src/_data/surveys.js`); blank means unknown, and the page then draws no limit line.
- **Each recording may carry Gerardus street geometry** in its vectors file where the site's pose
  is accepted, drawn by the player behind a "Street geometry" switch; travel lanes are not in it yet.
- **The compare view** overlays recordings' histograms and percentile rows with one shared scale.
- **The archive pages** (`/surveys/<id>/archive/`) replay every file for the Wayback Machine and
  must keep working.

## Design

### Data

The page reads `vehicle_speed.directions` from `part-000/timeline.json` (LiDAR) or `summary.json`
(radar), each entry with `id`, `name`, `bearing_deg`, `speed_limit` (`value`, `unit`, `source`),
`track_count`, `p50`, `p85`, `p98`, `max` and `histogram`, as the reporting plan defines. A
recording without the field, or with an empty list, renders as today.

`summariseSpeed` gains a sibling, `summariseDirections(raw)`, returning the headline summary and a
list of per-direction summaries in the same shape `summariseSpeed` returns, each with its own
`limit` (the value in the sign's unit, with unit and source) and `name`. The site-level
`speedLimitMph` remains the fallback for a recording whose directions carry no limit, flagged on the
page as the site's limit rather than the direction's.

### The chart

**Two directions, wide screens: one mirrored histogram.** The speed axis runs through the middle;
one direction's bars rise above it and the other's fall below, both as shares of their own
population, with one share ceiling for both halves (`shareCeiling` over both). Each half has its own
limit line and its own shading above the limit, its direction name at its outer edge, its `n`, and
its `p50`, `p85` and `p98` ticks on its side of the axis. The over-limit colour is the same on both
halves; the under-limit colour differs slightly between halves so the two populations read apart in
a saved PNG without relying on position.

```text
   Kirkham St eastbound   n = 1,204                         LIMIT 25
   share  ▐  ▐▌ ▐██▌▐███▌▐██▌▐▌ ▐▌ ▐   ▐                         ▐
    ──────┼────┼────┼────┼────┼────┼────┼────┼────┼────┼──── mph
          5   10   15   20   25   30   35   40   45   50
          ▐  ▐▌ ▐█▌ ▐██▌▐███▌▐███▌▐█▌ ▐▌ ▐   ▐
   Kirkham St westbound   n = 987                                LIMIT 30
                         P50 ▲   P85 ▲    P98 ▲
```

**Narrow screens:** two stacked small multiples with the shared `speedScale` and `shareCeiling`,
each with its own limit line, as the compact variant draws today, so a phone shows the same facts
one above the other. The breakpoint is the one `sv-hide-narrow` uses.

**More than two directions** (an intersection): a grid of small multiples sharing the axis and
ceiling, two per row on wide screens and one on narrow, each with its name, limit and `n`.

**Chart module.** `survey-charts.js` gains `mirroredHistogramSvg(a, b, opts)` and
`directionGridSvg(list, opts)`, both built from the same bar, limit and tick primitives
`histogramSvg` uses, with `aria-label` text naming both directions, their limits, counts and
percentiles. `speedScale` and `shareCeiling` take the list of direction summaries. The Go chart
package draws the same form for the PDF and the web app (the reporting plan's `RenderMirrored`), so a
reader moving from the public page to a report sees one chart, not two (DESIGN §4.1, §7.2).

### The card

The speed card keeps its shape and gains, when directions exist:

- the mirrored chart (or the stack, or the grid) in place of the single histogram; the single
  histogram remains available under the expandable section as "All vehicles";
- a **LAW** tag per direction in the head ("LAW 25 mph eastbound · 30 mph westbound"), each with
  its source in the tooltip (sign, City record, statutory default); a `statutory` limit shows as
  "default" rather than as a sign;
- key numbers in one row per direction, each with `n`, then the headline row as today;
- the bands above the limit per direction in the expanded section, each computed by `excessBands`
  against that direction's limit;
- the finding sentence per direction (`speedFinding`), then the headline's;
- "Chart PNG" saving the chart shown, named with the direction ids;
- in Data & methods, how directions were assigned: lanes approved in Gerardus (with the snapshot's
  date) or the heading fallback (compass words only, no street names).

**Primary street.** A site with two streets has up to four directions. The survey's primary street,
named on the deployment or the site, decides which two the card leads with; the others sit in the
grid under "All directions" and in the key-number rows.

**Listing.** The surveys listing's speed column shows a two-row sparkline (`sparklineSvg` gains the
variant), and "P85 / limit" shows both primary directions, each against its own limit; sorting by
P85 uses the primary street's faster direction.

A direction with fewer than five tracks is not listed by the export, so the page never has to hide
one; a direction with fewer than 50 shows `p98` and `max` as "--", matching the report's masking.

### Lanes on the 3D view

Where the recording's vectors file carries the snapshot's travel lanes, the player's "Street
geometry" layer draws them as thin directed lines with arrowheads in their direction hue, and the
compass legend names each direction. This is a small addition to `scene-vectors.js` and is drawn
only where the file has the features; the page claims nothing where it has none.

### Compare view, and a limit that changed between recordings

The compare view keeps comparing headline summaries across recordings. A per-direction compare
(eastbound this year against eastbound last year) is a follow-up once two recordings at one site
carry the same direction ids; the ids are stable because they come from the lane snapshot's
directions, or from the radar deployment's two.

Each recording carries the limit that was in force while it was recorded, with its effective date,
because the export attributes by measurement time (the lane snapshot plan). When two recordings at one site
show different limits for the same direction, the site page says so in one line under the
recordings list ("eastbound was signed 25 mph in September 2026 and 30 mph from March 1, 2027"),
and the compare view draws each recording's bars against its own limit line rather than one. Shares
above the limit are therefore comparable across a change: each is against the sign that stood.

### Where the data lives

The page reads only files the recording publishes: `timeline.json` or `summary.json`, and the
vectors file. Those assets sit under `public_html/src/scenes/` today, the one place street data is
committed, and move to the Hugging Face dataset under D-34's first step (the v0.7.4 item), where the
site build fetches them. Nothing this plan adds is committed separately: no per-site JSON, no lane
file, no rendered image. A top-down plot of a site's lanes, limits and traffic, where one exists,
is the render tool's output published in the dataset (`derived/maps/`, the lane snapshot plan) and is
embedded by its dataset URL, never copied into the site tree. The chart tests' fixture is a
hand-written summary with made-up numbers, not a real site's export.

### Accessibility and archiving

- Every chart has an `aria-label` that reads as a sentence with both directions.
- Colour is never the only distinction: names, `n`, and the axis through the middle carry it.
- The archive page lists the same files; `timeline.json` is already among them, so an archived
  page shows directions without a new fetch.
- The `noindex` archive page and the Wayback capture are unaffected: nothing new is fetched.

### DESIGN.md amendment

§4.3, the shared speed summary contract, gains: "where a recording or report carries directions,
each is summarised by the same contract against its own signed limit; two directions draw as one
mirrored histogram about the shared speed axis, more as a grid of small multiples sharing scale and
ceiling; the headline summary is unchanged". Made in the same change as Item 2, since it governs
three surfaces.

## Scope

### Item 1: model and data

**Summary:** Read `vehicle_speed.directions`, summarise each direction with its own limit, fall
back to the site limit, and expose both to the templates.

**Steps:**

1. `summariseDirections` and its tests beside `summariseSpeed`'s in `public_html/src/js/__tests__`.
2. `surveys.js` passes directions and their limits per recording; `speedLimitMph` stays the
   fallback.

**Milestone:** v0.7.19 `S`

### Item 2: the charts

**Summary:** `mirroredHistogramSvg`, `directionGridSvg`, the compact stack, shared scale and ceiling
across directions, and the DESIGN §4.3 amendment.

**Steps:**

1. The two functions from the existing primitives; `speedScale` and `shareCeiling` over lists.
2. Tests: two directions with different limits; three directions; one direction only (draws the
   single histogram); limit off a bucket edge (no exact share, as today).
3. Rendered against the re-exported scene the reporting plan provides, checked at 360 and 1280 px.

**Milestone:** v0.7.19 `M`

### Item 3: the card

**Summary:** Per-direction LAW tags, key numbers, bands, findings and PNG naming in
`family-speed.njk`; the headline kept.

**Milestone:** v0.7.19 `S`

### Item 4: lanes in the 3D view

**Summary:** Draw the snapshot's travel lanes with direction arrows where the vectors file has them.

**Milestone:** v0.7.19 `S`

## Dependencies

- The companion reporting plan's Item 4: `directions` in the export and lanes in the vectors file;
  Items 1 to 3 here can be built against a hand-written fixture before that lands, and are only
  visible on a published page once a scene is re-exported.
- D-34's first step (scene assets out of git) does not block this: the templates and modules read
  whatever the fetched assets carry.

## Risks

| Risk                                                               | Likelihood | Impact | Mitigation                                                                                                               |
| ------------------------------------------------------------------ | ---------- | ------ | ------------------------------------------------------------------------------------------------------------------------ |
| The mirrored chart is read as one distribution with a strange axis | Medium     | Medium | Names at the outer edges, `n` on each half, the axis labelled through the middle, a legend; narrow screens stack instead |
| Two limits make the "share above limit" headline ambiguous         | Medium     | Medium | The headline keeps the site limit and says so; per-direction shares sit under their own LAW tags                         |
| A re-exported scene changes the headline numbers                   | Low        | High   | The headline summary is the same population; the reporting plan pins it; the page's headline is unchanged                |
| The site-level limit and a direction's limit disagree              | Medium     | Low    | The page prints both, each with its source; the direction's governs its chart                                            |
| Build-time chart tests go stale with the fixture                   | Low        | Low    | Fixture committed under `__tests__`; the re-exported scene is a second check, not the fixture                            |

## Design decisions

- **The mirrored form for two directions, a grid for more**, shared with the PDF and the web app
  (the reporting plan's Design decisions); the stacked small multiples on narrow screens. The owner
  endorsed the stacked form on November 3, 2026. The superseded directional plan's side-by-side
  pair on one speed axis is the recorded alternative, and the wide-screen default may be switched
  without reopening this plan.
- **Each recording keeps the limit that stood while it was recorded**, with its effective date, so
  a City change between surveys shows as a change, not as a correction to the earlier page.
- **No street data is committed for this page** beyond the survey assets already under
  `public_html/src/scenes/`, which D-34 moves to the dataset; renders come from the dataset by URL.
- **The headline stays.** Directions add to the card; they never replace the single summary a
  reader already knows.
- **Limits come with the data.** A direction's limit travels in the export with its source; the
  site mark's limit is a fallback and is labelled as the site's.
- **Nothing new is fetched.** Directions ride in files the page already loads, so the archive pages
  need no change.

## Open questions

- Should the "All vehicles" histogram stay visible on wide screens beside the mirrored one, or only
  in the expanded section? The expanded section is proposed.
- Does the compare view need per-direction comparison in the same slot, or can it wait for a
  second recording that carries directions?
- Should the per-direction `p98` and `max` floor follow the report's 50, or the page's own rule?

## Checklist

### Complete

- [x] Plan written against the speed card, chart module and model as built

### Outstanding

- [ ] Item 1: `summariseDirections`, data plumbing, tests (`S`)
- [ ] Item 2: mirrored and grid charts, shared scale, DESIGN §4.3 amendment (`M`)
- [ ] Item 3: the card's per-direction tags, numbers, bands and findings (`S`)
- [ ] Item 4: lanes drawn in the 3D view (`S`)

### Deferred

- [ ] Per-direction compare across recordings: after two recordings at one site carry direction ids
- [ ] The public site's move to its own repository (D-34): the modules move with it unchanged

### Accepted residuals (no action planned)

- [ ] A recording exported before the reporting plan's Item 4 shows no directions until re-exported
- [ ] The site-level limit in `map-marks.json` remains for sites with no snapshot and no radar directions
