# Crash-rate datasets and published risk models in reports plan

How velocity.report should use existing crash-rate datasets and the published speed-to-harm
literature, what an external proposal for doing so got right and wrong, and the order to build it
in. The proposal's two useful ideas survive: a versioned, offline reference asset the report can
cite, and a local crash-history record for each site. Its framing does not: no "axioms", no
kinematic priors from in-vehicle studies, no geohash, no agent protocol, and no risk score.

Sources were checked on October 8, 2026 through search results only. Direct fetches of the TIMS,
DataSF and AAA Foundation pages failed from the review environment, so every rights claim and
figure below is marked for confirmation against the primary document.

- **Status:** Proposed; no dataset ingested and no code written
- **Layers:** Cross-cutting (PDF report, site configuration, importer tooling, L8 behaviour benchmarks)
- **Target:** v0.5.10 for the cited harm curves, the benchmark-aligned temporal strata, and, if Item 1 clears the rights, the area-rate table (else v0.6.8); v0.6.8 for site crash context, the before-and-after model, the site-to-area resolution, and the area benchmark context block; v1.0 for road-segment attachment
- **Companion plans:** [behaviour analytics](lidar-behaviour-analytics-plan.md), [posted speed limits](posted-speed-limits-plan.md), [vehicle encyclopedia](vehicle-encyclopedia-plan.md), [spatial priors reference data](spatial-priors-reference-data-plan.md)
- **Related:** [sober driving and human crash baselines analysis](../platform/operations/human-crash-baselines-analysis-2026-10.md), [data science methodology](../platform/operations/data-science-methodology.md), [identifiability analysis](../platform/architecture/identifiability-analysis.md), [S2 conventions](../lidar/architecture/geographic-indexing.md), [TENETS](../../TENETS.md)
- **Canonical:** [PDF reporting](../platform/operations/pdf-reporting.md)

## Motivation

The report already makes a safety argument, and it makes it without a source. The Citizen Radar
section says a vehicle at 40 mph "has four times the crash energy" of one at 20 mph and that small
increases in speed "dramatically raise the likelihood of severe injury or death", with no citation.
The README calls Clarendon Avenue "a high injury road", which is a municipal designation with a
published methodology and a version, cited as neither. [Q30](../../data/QUESTIONS.md) asks whether
kinetic energy is even the right composite for pedestrian harm and records no decision. Tenet 3
says statistical claims require sources and intervals. The report's safety paragraph is the one
place the product currently fails it.

Crash datasets and the speed-to-harm literature can fix that, and they can do something the
sensor cannot: say what has actually happened on the street, and what a measured change in speed
is expected to do to it. That is the argument the Clarendon survey needed when the January 2026
numbers came back higher than the June 2025 baseline.

The consequence of inaction is that reports keep asserting harm from first principles. The
consequence of adopting the external proposal as written is worse: a second benchmark system
beside the one the behaviour plan already defines, kinematic "priors" the sensor cannot measure
against, and a geographic index that contradicts the repository's S2 convention.

## Current state

- **Report copy.** `citizen-radar()` in
  [sections.typ](../../internal/report/typst/templates/sections.typ) prints the kinetic-energy
  argument with no citation. The percentile section calls p98 a view into "high-risk driving
  patterns". `site-information()` prints only a free-text speed-limit note.
- **Speed limit.** A unitless per-request integer, default 25, with no jurisdiction. Migration
  000014 removed it from `site`. The [posted speed limits plan](posted-speed-limits-plan.md) owns
  its return, at v1.0, on the vector scene.
- **Site location.** `site` carries optional `latitude` and `longitude`; `lidar_sites` carries
  canonical S2 L13 and L10 tokens with a provenance of surveyed or operator. There is no geocoder
  and a test asserts there is none. Tracks live in the sensor-local frame; a track has no
  geographic position until a capture is registered.
- **Kinematics.** Radar transits carry maximum and minimum speed and a point count, with
  direction dropped by the worker. `radar_objects` has a `speed_change` over `delta_time_ms`,
  which is the only deceleration proxy and comes from the sensor's own classifier. LiDAR
  acceleration and braking are specified in behaviour plan Section 8.2 and gated on G-SMO-1, with
  the explicit finding that "no universal harsh-acceleration threshold is defensible here".
- **Benchmarks.** `l8behaviour.Benchmark` (`result.go`) carries `Kind`, `Threshold`, `Citation`,
  `Jurisdiction`, `EffectiveFromUnixNanos` and `Stratification`. The five kinds are `legal`,
  `research_threshold`, `external_distribution`, `local_distribution` and
  `no_established_threshold`. Validation refuses a `research_threshold` or
  `external_distribution` without a citation. This is the only place an external fact currently
  enters a measurement.
- **Wording guard.** The headway report's `VerdictPattern` in `internal/report/headway/wording.go`
  fails any served payload containing `tailgat`, `aggress`, `driver`, `risk`, `score`, `verdict`,
  `unsafe`, `danger`, `violat`, `offend`, `propensity` or `profil`.
- **Crash data.** No crash or collision dataset, designation layer, or speed-to-harm model is
  referenced anywhere in code, configuration, `data/maths/references.bib` or the docs. SHRP2
  appears once, for its free-flow opportunity method (DOT HS 812 858), already cited in the
  behaviour plan.
- **Human crash baselines.** The [October 2026 analysis][hcb] read the Waymo sober-baseline and
  fatal-rate papers and Valgo's client README against this plan. It verified the sober paper's
  exposure-reconstruction algebra ([Finding 2][hcb-f2]) and located the uncertainty: the fatal
  reduction is pinned near the observed alcohol share, so the relative risk's interval moves it
  only from 21.7% to 25.5%, while the police-reported sober rate of 4.51 (4.01, 4.82) against a
  status quo of 4.88 incidents per million miles spans a reduction of 1.2% to 17.8%
  ([Finding 3][hcb-f3]). Its arithmetic shows that one site cannot test an area rate: a
  residential site of 1,500 transits a day over 0.1 mile expects 0.27 police-reported
  involvements a year ([Finding 6][hcb-f6]). No code, configuration, or `references.bib` entry
  cites these sources yet.
- **Shipped reference data.** The precedent for embedding third-party data is the
  [vehicle encyclopedia](../platform/architecture/vehicle-encyclopedia.md): a versioned subset,
  pinned at build time, never a runtime dependency, with per-field provenance and year-marked
  editions. Embedding uses `go:embed` beside the consuming package (`assets.go`,
  `internal/api/sensor_models.go`).
- **Names already taken.** `spatial-priors` is the geometry-prior project. `internal/prior/` is
  reserved for its velocity.report adapter. `data/` holds maths, structures and exploration, not
  runtime data. `*.db` is already ignored.
- **Tooling policy.** Python is developer-only and absent from Pi images
  ([python-venv](../platform/operations/python-venv.md)). Operator importers are Go commands under
  `cmd/tools/`, such as `import-s2-site-index`. Exploration lives in `data/explore/`.

## Findings

### Review of the external proposal

| Proposal                                                                                                      | Review                                                                                                                                                                                                                                                                                                                                                                                                                                        | Consequence                                                                                                                                                                                                |
| ------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| An "axiom" JSON of SHRP2 macro-priors: mean near-crash deceleration, lateral acceleration, a 1.5 s TTC cutoff | SHRP2 is an in-vehicle naturalistic study; this is a roadside sensor with no acceleration channel on radar and gated acceleration on LiDAR. None of the values is cited. The ones that resemble real numbers are event-detection triggers from an instrumented vehicle's inertial unit, not population means. The 1.5 s TTC figure is FHWA SSAM's screening default, already in the behaviour plan with its defined use and its caveat        | Not adopted. SHRP2 enters only as the opportunity method already cited, and as an `external_distribution` where a published aggregate exists for a quantity this sensor measures                           |
| "Axioms", "immutable parameters" the agent binds telemetry against                                            | Tenet 3 and behaviour plan Section 1: observables, not verdicts. A benchmark is a cited value with the use it was defined for, never an axiom. Immutability is the right instinct for a shipped edition, and the encyclopedia already sets that pattern                                                                                                                                                                                       | Adopt a pinned, versioned reference edition. Reject the word and the posture                                                                                                                               |
| "A deceleration of −0.52 g against a baseline of −0.45 g qualifies as an anomalous hard-braking event"        | One event compared with the mean of events selected because they were near-crashes is not a test of anything. Radar cannot produce the number. LiDAR braking metrics wait on G-SMO-1 and carry no universal threshold by the plan's own finding                                                                                                                                                                                               | Not adopted. Per-passage kinematic verdicts are out of scope of this plan and of the behaviour plan                                                                                                        |
| A local SQLite `geographic_crash_likelihood` keyed by six-to-seven character geohash                          | The repository's geographic key is S2 L13 with L10 parents ([convention](../lidar/architecture/geographic-indexing.md)). A geohash prefix is a near-square cell; a street is a line, and an intersection's crashes split across cell edges. "Likelihood" names a probability the table does not contain                                                                                                                                       | Adopt the local, offline, versioned record. Key it to the site, and later to a vector-scene road segment, carrying S2 L13 for interoperability. No geohash, no "likelihood"                                |
| `data/spatial_priors.db` built by `cmd/import-spatial-priors/`                                                | The name belongs to the geometry-prior project; a crash table in a file called spatial priors will be opened by the wrong tool. `data/` is not a runtime directory here                                                                                                                                                                                                                                                                       | A `site_crash_context` record in the one SQLite database, produced by a Go importer under `cmd/tools/`                                                                                                     |
| The agent extracts a trajectory's start and end coordinates, converts them to a geohash and queries           | Tracks have no geographic coordinates until registration, and a site has one position. Per-track geographic lookups are also the wrong shape for privacy: the join is per site, offline, at report time                                                                                                                                                                                                                                       | Per-site context, resolved when a report is generated                                                                                                                                                      |
| SWITRS through UC Berkeley's TIMS as the primary source                                                       | TIMS is account-gated and governed by a licence agreement whose redistribution terms this review could not read. For San Francisco the open sources are DataSF's injury-crash dataset, derived from SFPD reports and SWITRS and released under PDDL unless a dataset page says otherwise, and the Vision Zero High Injury Network layer. FARS is public domain but fatal-only; STATS19 covers Great Britain under the Open Government Licence | Per-jurisdiction source adapters with a rights manifest, as the [reference data plan](spatial-priors-reference-data-plan.md) does for geometry. San Francisco first. TIMS only after its agreement is read |
| `high_injury_network_bool` and "14 historical collisions" become "an elevated physical risk vector"           | The HIN is a planning designation: a stated methodology (at least 10 severe or fatal injuries per mile over a stated window), a version, and no per-segment counts in the published layer. A count with no window, severity, mode or exposure is not a rate. "Risk vector" is a verdict the wording guard already refuses                                                                                                                     | Report counts by severity and mode over a stated window, with the source's definition, years and version; designation membership as a designation. Never a rate without a denominator, never a score       |
| A Phase 3 "Agent Execution Protocol" in developer documentation                                               | The report's reader is a person, usually a city engineer; a prompt is not an engineering control. Coding agents read `.github/knowledge/` modules. The machine-readable surface already exists: the report's `data.json` and the benchmark provenance on every measurement                                                                                                                                                                    | Not adopted. The reference edition and the site context are structured data the report, the API and any agent read alike; one line in the knowledge index points at the taxonomy                           |
| Phase 4 layout: `assets/priors/`, `data/raw_imports/`, `internal/priors/`, `test/testdata/`                   | None of these follows the repository: embedded assets sit beside their package, `internal/prior/` is reserved, testdata sits beside the code under test, `*.db` is already ignored and `results` directories are already local-only                                                                                                                                                                                                           | Repository conventions, listed in the design section                                                                                                                                                       |
| "Go or Python for the ingestion scripts?"                                                                     | Decided by [policy](../platform/operations/python-venv.md): Python never runs on the device and never ships; anything that produces a record the server stores is Go                                                                                                                                                                                                                                                                          | Go importer under `cmd/tools/`. Python in `data/explore/` for looking at a dataset before writing the adapter                                                                                              |
| "Draft the JSON from validated federal briefs"                                                                | The numbers worth pinning are not SHRP2 kinematics. They are pedestrian injury-risk curves and speed-change crash models, and each needs its primary paper read, not a brief                                                                                                                                                                                                                                                                  | Item 1 below is that reading                                                                                                                                                                               |

### What the evidence can and cannot establish

Four questions a report reader asks, four source families, and the boundary of each. The fourth,
from the [human crash baselines analysis][hcb-align], is the first family with a denominator.

| Question                                                                                               | Source family                                                                                                                                                                                                                                            | Establishes                                                                                                                                                                                                               | Does not establish                                                                                                                                                                                                                              |
| ------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| How much more does a pedestrian stand to lose at 35 mph than at 25?                                    | Published impact-speed injury and fatality curves: Tefft 2013 (AAA Foundation, _Accident Analysis & Prevention_), Rosén and Sander 2009, Hussain et al. 2019 meta-analysis                                                                               | The probability of severe injury or death as a function of **impact** speed, for the population and vehicle fleet the study standardised to, with its curve                                                               | Harm from a **travel** speed: a driver brakes before impact. Applied to measured speeds the curve is an upper bound, and the report says so. Mass and front-end geometry, which the encyclopedia supplies later, shift the curve and are absent |
| If mean speed rose after repaving, what does the literature predict for crashes?                       | Speed-change crash models: Nilsson's power model, Elvik's exponential model, with exponents and intervals from Elvik et al. 2019                                                                                                                         | The expected relative change in crashes by severity for a change in **mean speed** of the same traffic on the same road, with a confidence interval                                                                       | A crash count for this street; anything from a change in p85 or p98 alone; anything across a change in road users or volume                                                                                                                     |
| What has actually happened on this street?                                                             | Jurisdiction crash records (DataSF injury crashes; FARS; STATS19) and designation layers (Vision Zero High Injury Network)                                                                                                                               | Counts by severity and mode over a stated window near a stated geometry, from a named source with its reporting rules; designation membership and version                                                                 | A rate, without exposure; a trend, without a long window; causation from the measured speeds; anything about an individual passage                                                                                                              |
| What involvement rate does this area's traffic have, by stratum, and what would it be without alcohol? | Exposure-normalised area crash involvement rates with Poisson intervals: the fatal-rate paper (FCIR, 50 urban areas), the sober paper (sober and impaired cohorts), the SAE paper (police-reported, counties), and Valgo (hosted, definition-selectable) | The rate per vehicle-mile for the area, road type, stratum, severity, and cohort, in a stated year under a stated definition, with the count's interval; the expected involvements implied for a stated volume and length | Anything at one street; a measured rate at the site; the site's crash history, which the context record carries; any causal link from the site's speeds; impairment at the site                                                                 |

What no source here establishes: that any observed passage was a near-crash, that this site's
speeds caused its crash history, or a calibration from surrogate safety measures to crashes at
one site. Those remain out of scope, as the behaviour plan and the state-estimation plan's
Section 12 already say.

## Design / approach

### One taxonomy, not two

Every external fact enters a report as a benchmark of a declared kind with a citation, through the
same contract `l8behaviour.Benchmark` already enforces. A published curve or transfer function is
neither a threshold nor a distribution, so this plan proposes a sixth kind, `published_model`:
a cited function of a measured quantity, carrying the population and years it was fitted to, the
unit it was published in, and its interval. The alternative is to carry curves under
`research_threshold` with the citation doing the work; the behaviour plan's owner decides, and
the choice is recorded there, not here.

### A pinned safety-reference edition

A small Go package, proposed as `internal/report/safetyref`, embeds one JSON edition with
`go:embed`:

- **Curves.** Each as its published parameters or tabulated points, with author, year, DOI,
  population, standardisation, the speed it is a function of (impact, not travel), and the unit.
- **Models.** Exponent or coefficient per severity class with its interval, the road types it was
  estimated on, and the paper.
- **Area rates.** Crash involvement rates per vehicle-mile, one row per area, road type, temporal
  stratum, severity, cohort (status quo or sober), crash year, and source edition. Each row
  carries its rate, lower and upper bound, count, vehicle-miles, unit (IP100MM, incidents per 100
  million miles, for fatal; IPMM, per million miles, for police-reported), and definition
  ([alignment][hcb-align]). Rows are per area, never the 50-area averages with averaged bounds
  that the sober paper's Table 4 prints ([sober paper][hcb-sober]). The fatal-rate paper's
  supplemental data is the first content once Item 1 clears its licence; the sober paper's rows
  wait until it publishes with data, since the preprint has per-area values only in figures. Each
  row enters a report as an `external_distribution` benchmark with its citation and
  stratification, a kind `Validate` already refuses without a citation, so a rate needs no new
  kind. `l8behaviour.Benchmark` carries one `Threshold` and no interval, so the bounds live in the
  edition entry the `Citation` names and the report prints them from there: a question for the
  behaviour plan's owner, beside `published_model`.
- **Edition.** A year-marked identifier, the date each source was read, and a digest. The report
  prints the edition it rendered with. A correction is a new edition; a published report stays
  reproducible.

No network, no runtime fetch, no crash records in the edition. This follows Tenet 7 and the
encyclopedia's edition model exactly, at a size of a few kilobytes before the area-rate table,
whose size Item 1's reading of the supplement fixes.

### A site crash-context record

One `site_crash_context` row per site and source, written by an operator tool and read by the
report:

| Field group  | Contents                                                                                                                    |
| ------------ | --------------------------------------------------------------------------------------------------------------------------- |
| Source       | Source identifier and version or retrieval date, licence, attribution text, importer version                                |
| Geometry     | Basis of the join: a radius around the site point, or named street segments the operator confirmed; the site's S2 L13 token |
| Window       | First and last date covered, in the source's own reporting rules                                                            |
| Counts       | By severity class and road-user mode, as the source defines them; never victim-level rows                                   |
| Designations | Membership in named layers, each with its version and the methodology it cites                                              |
| Provenance   | Digest of the input extract, so the row can be rebuilt and checked                                                          |

Rules:

- The importer runs on a workstation, in Go, under `cmd/tools/`, against a downloaded open
  dataset in its own directory. Downloads are untrusted input and never enter the checkout. The
  device stores the resulting row and nothing else.
- The join is a suggestion until an operator confirms the segments, mirroring how the posted
  limits plan treats an OpenStreetMap `maxspeed` prior.
- A site without coordinates has no context, and the report says nothing rather than something.
- Counts are aggregates over a window of years. Where a count would single out a person, which
  the source's own publication may already do, the report lengthens the window or suppresses the
  cell, following the
  [identifiability analysis](../platform/architecture/identifiability-analysis.md).
- The record attaches to the site now and to the vector-scene road segment at v1.0, when the
  posted limits plan gives a segment to attach to. Nothing in the row depends on the scene.

### Benchmark-aligned temporal strata

The papers cut the week the way the NHTSA convention does, and a report can cut its own transits
the same way with no external data ([Finding 5][hcb-f5]):

| Stratum | Local clock time             |
| ------- | ---------------------------- |
| Day     | 06:00 to 17:59               |
| Night   | 18:00 to 05:59               |
| Weekday | Monday 06:00 to Friday 17:59 |
| Weekend | Friday 18:00 to Monday 05:59 |

Day and night cross weekday and weekend into the four cells the papers report. The weekend begins
on Friday at 18:00, not Saturday at midnight. Each transit takes its stratum from its start time,
`transit_start_unix`, in the site's IANA zone, which `site_reports.timezone` already carries, with
daylight saving as the clock showed it.

- Per stratum, the report prints the count and p50, p85, and p98 over per-transit maximum speeds,
  as [percentile aggregation semantics](../radar/architecture/percentile-aggregation-semantics.md)
  requires, and never averages them across strata.
- A stratum under 50 transits prints its count and masks its percentiles
  ([Finding 5][hcb-f5]). The charts use 50 as `LowSampleThreshold` and shade below it rather
  than blank, so the stratum table is the stricter of the two.
- The boundary-hour filter (`BoundaryThreshold`) applies to the stratum counts exactly as to the
  headline numbers, or the report prints the difference: the first and last hour of each day,
  which it drops when sparse, fall inside the night stratum at a site that records all day.
- The headline numbers do not change.

The same per-site distribution of passes by time of day and day of week is the evidence
[Q35](../../data/QUESTIONS.md) asks for, for a privacy reason.

### Site-to-area resolution

The area benchmark context needs the area whose rates apply. The site's latitude and longitude
resolve to a 2020 Census urban area or a county, the geographies the papers publish
([alignment][hcb-align]). The resolution is a suggestion until the operator confirms it, exactly
as the crash-context join is: a lookup in a coarse covering pinned in the edition, or an
operator-entered area id from the edition's list. The confirmed area id sits on the site with the
site's S2 L13 token. It is never a geocoder call, and the web test that asserts there is no
geocoder (`MapEditorInteractive.test.ts`) stays green. A site without coordinates or a confirmed
area has no area benchmark context.

### Report surfaces

1. **Cited harm paragraph.** Replace the kinetic-energy paragraph with a short statement of the
   impact-speed curves, the figure for the site's posted or operator-supplied limit against its
   measured p85 and p98, in the sign's unit, and the citation with the edition. The
   impact-versus-travel caveat is printed, not footnoted.
2. **Site context block.** Counts by severity and mode over the window, the designation and its
   version, the source and licence line. Absent when there is no record.
3. **Before and after.** When a report compares two periods at one site, the mean-speed change and
   the model's predicted relative change in crashes by severity, with the interval, labelled as a
   model prediction from a named paper. Never computed from p85 alone.
4. **Area benchmark context.** The confirmed area's status-quo and sober rates by stratum, each
   with its interval, crash year, and definition; the expected involvements the area's crash
   involvement rate implies for the surveyed segment, `E = CIR x N x L`, from the site's stratum
   counts N and an operator-stated segment length L ([Finding 6][hcb-f6]); and the printed
   sentence that this is what the area rate implies for this segment's volume, not a measurement
   at this site. Absent when the site has no confirmed area, no stated length, or no unmasked
   stratum. The block says "status-quo cohort" and "sober cohort", never "drivers": the guard's
   `VerdictPattern` matches `driver` and `risk` anywhere in a served payload, so citation strings
   render from a field the guard skips or use titles that pass. The fatal-rate and sober papers'
   titles pass as written; Table 3's "Relative-Risk" does not ([alignment][hcb-align]).
5. **Wording guard.** The headway report's verdict test extends to these sections. "Risk score",
   "unsafe", "dangerous" and "likelihood" do not reach a page.

### Hosted baselines: what an exploration could add

Valgo serves the status-quo rate for a region under a definition the caller chooses, outcome,
vehicle class, road type, weather, crash year, and, in some regions, posted-speed band, and
returns the rate with its bounds, count, and vehicle-miles ([Valgo][hcb-valgo]). The report's
rates come from the pinned edition, and Tenets 1, 4, and 7 keep a runtime call out of the product
([incorporation][hcb-incorporate]). What a workstation exploration could learn beside the edition:

- How far an area rate moves under matched definitions, the sensitivity [Finding 7][hcb-f7]
  illustrates from one published number, and so which definition a report names when it quotes
  an area rate.
- Rates by posted-speed band where the service offers them, which the
  [posted speed limits plan](posted-speed-limits-plan.md) would let a site cite for roads posted
  like it.
- Regions, corridors, and crash years the open papers do not cover.

The exploration lives in `data/explore/` with the Python client, under the
[Python policy](../platform/operations/python-venv.md). Each result is kept with its full
definition and retrieval date, and nothing from it reaches a report. What it shows to be useful
returns to this plan as a proposal, with the service's terms of use read first.

### Machine access

The report's `data.json`, the edition JSON and the context row are the machine-readable surface.
An agent that wants to reason over them reads the same bytes the template does, with the same
provenance. No prompt protocol is added; one line in the `.github/knowledge/` index points at the
benchmark taxonomy and this plan.

### Boundaries

No composite score. No per-track geography. No runtime network access. No SHRP2 kinematic
thresholds. No "likelihood" column. No crash records in the binary. No claim that a surrogate
measure predicts a crash at this site. No site crash rate: a rate is printed for an area, never
computed for a site.

## Scope

### Item 1: read the primary sources and record what they say

**Summary:** Replace every figure in this plan marked for confirmation with the number from the
paper, and record each source's rights.

**Steps:**

1. Tefft 2013: the curve form, standardisation (2007 to 2009 US pedestrians and fleet), sample,
   and the 50% severe-injury and fatality speeds. Rosén and Sander 2009 and Hussain et al. 2019 for
   the residual disagreement between curves, stated rather than averaged.
2. Elvik et al. 2019: exponential and power coefficients by severity with intervals and road
   types. Confirm whether the Scottish 2024 appendix's 4.1 (2.9 to 5.3) for fatal crashes is the
   same estimate; do not cite it until it is.
3. DataSF injury-crash dataset: fields, severity and mode coding, geocoding basis, update
   cadence, and the dataset page's licence line. The 2022 High Injury Network layer: methodology,
   window (sources disagree between 2017 to 2021 and 2017 to 2022), and licence.
4. TIMS licence agreement, read in full, before SWITRS is used directly. FARS and STATS19 terms
   for the non-SF adapters.
5. The fourth family's rights, per the [analysis's rights rows][hcb-align]: the fatal-rate paper
   and its supplement under CC BY-NC-ND 4.0, settling whether an embedded subset table is a
   derivative and whether this use is non-commercial; the sober paper, whose preprint states no
   terms, confirmed on publication; the SAE police-reported paper; FHWA HM-71 and VM-4, confirming
   no third-party component; INRIX, proprietary, so only the aggregates the papers publish are
   citable.
6. Open the fatal-rate paper's supplement and record whether it carries the temporal vehicle-mile
   fractions as well as the rates. The research note in the Deferred checklist depends on it.
7. Add every source to `data/maths/references.bib` and a dated verified-sources table to this
   plan.

**Milestone:** v0.5.10

### Item 2: safety-reference edition and the cited harm paragraph

**Summary:** Ship the edition package with its area-rate table and replace the uncited report
copy.

**Steps:**

1. `internal/report/safetyref`: edition JSON, loader, validation that every entry carries a
   citation, population and unit, and a test that the embedded edition validates.
2. The `published_model` benchmark kind, or the recorded decision not to add one.
3. The cited harm paragraph in `sections.typ`, driven from `data.json`, in the sign's unit, with
   the impact-versus-travel caveat and the edition identifier.
4. The wording-guard test over the new section.
5. Answer Q30 to the extent the curves allow and record the decision in `data/QUESTIONS.md`.
6. The area-rate table from the fatal-rate paper's supplement: per-area rows with the fields in
   the design, a validation test that every row carries its bounds, count, vehicle-miles, unit,
   definition, and citation, and the guard test over every citation string. If Item 1 refuses an
   embedded subset, the edition cites the paper instead and the table waits.

**Milestone:** v0.5.10; the area-rate table (step 6) at v0.5.10 if Item 1 clears the rights, else
v0.6.8

### Item 2a: benchmark-aligned temporal strata

**Summary:** Print each report's counts and percentiles in the papers' four temporal cells, with
no external data.

**Steps:**

1. Stratum assignment from each transit's start time in the site's IANA zone, in the rollup
   (`RadarObjectRollupRange`) or in Go after it.
2. Per-stratum rows in `data.json`: count, p50, p85, p98, and whether the stratum is masked.
3. One PDF table of the four cells, with the boundary-hour filter applied identically or its
   difference printed.
4. Tests on the Friday 18:00 boundary ([Finding 5][hcb-f5]) and on both daylight-saving
   transitions, and a test that the headline numbers do not change.

**Milestone:** v0.5.10; depends on nothing

### Item 3: before-and-after model section

**Summary:** When two periods at one site are compared, print the predicted relative change in
crashes by severity from the pinned model, with its interval.

**Steps:**

1. Mean speed per period from the same transit population, with sample sizes and intervals.
2. The model evaluated on the mean-speed change only; refuse when the periods differ in site
   configuration period, cosine correction or sensor.
3. The report section and its guard test; a synthetic oracle in the style of the headway report.

**Milestone:** v0.6.8

### Item 4: site crash-context importer and record

**Summary:** A Go importer for DataSF, the `site_crash_context` migration, the report block, and
the site-to-area resolution with the area benchmark context block.

**Steps:**

1. Migration for `site_crash_context` with the field groups above.
2. `velocity tools import-crash-context --source datasf --site <id> --input <dir>`: parse the
   extract, join by radius, print the candidate segments, write the row only with `--confirm`.
3. Small-count handling per the identifiability analysis, tested.
4. The site context block in the report, with source and licence line.
5. A second adapter, FARS or STATS19, to prove the source interface is not SF-shaped.
6. Site-to-area resolution: the suggested area from the edition's covering or an operator-entered
   area id, written on the site only when the operator confirms it, carrying the S2 L13 token; a
   test that the resolution makes no network call, beside the web test that asserts no geocoder.
7. The area benchmark context block from the edition and the site's stratum counts, with the
   implied-not-measured sentence, absent without a confirmed area, a stated length, or an
   unmasked stratum; the guard test over the block and its citation strings, and a test that the
   sentence is printed.

**Milestone:** v0.6.8; steps 6 and 7 after the sites merge in the vocabulary plan and after the
area-rate table

### Item 5: attach context to road segments

**Summary:** Move the join from a radius to the vector-scene road segments once they exist.

**Milestone:** v1.0, after the posted speed limits plan

## Dependencies

- [Posted speed limits](posted-speed-limits-plan.md) for any compliance framing; until then the
  harm paragraph uses the per-request limit and says it was operator-supplied.
- The merged `sites` table with coordinate provenance, in the
  [vocabulary plan](platform-vocabulary-and-data-model-plan.md), for where the context row hangs
  and for the site-to-area resolution, which needs coordinates with a provenance.
- Item 2a's per-stratum counts for the area benchmark context block, which multiplies the area
  rate by them.
- G-SMO-1 before any LiDAR kinematic quantity is compared with anything.
- The [vehicle encyclopedia](vehicle-encyclopedia-plan.md) for mass and front-end geometry, which
  this plan does not wait for.

## Risks

| Risk                                                                               | Likelihood | Impact | Mitigation                                                                                                                                                                                                  |
| ---------------------------------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A curve for impact speed is read as a statement about travel speed                 | High       | High   | The caveat is printed in the section; the edition entry names the independent variable; the guard test checks the phrase is there                                                                           |
| A source's licence forbids redistribution of the aggregate                         | Medium     | High   | Rights manifest per source; the row stores counts, not records; TIMS not used until its agreement is read                                                                                                   |
| Small crash counts near a quiet site identify a person                             | Low        | High   | Window lengthening and cell suppression from the identifiability analysis; never victim-level fields                                                                                                        |
| The before/after model is applied to a p85 change or across a configuration change | Medium     | Medium | The section refuses unless both periods share site configuration and sensor; mean speed only                                                                                                                |
| Reviewers want a single "safety score" anyway                                      | Medium     | Medium | The behaviour plan's Section 1 rule, restated here; the wording guard makes the score impossible to print                                                                                                   |
| Figures in this plan drawn from search summaries are wrong                         | Medium     | Medium | Item 1 is the first milestone and replaces every marked figure from the primary paper                                                                                                                       |
| The join radius misattributes an adjacent arterial's crashes to a side street      | Medium     | Medium | Operator confirmation of segments; segment attachment at v1.0                                                                                                                                               |
| An area rate is read as the site's rate                                            | High       | High   | The block prints the sentence that the rate is what the area implies, not a measurement at the site; a test checks the sentence is there and the guard refuses verdict words; no site rate is ever computed |
| The sober paper's Table 4 averaged bounds are quoted as one area's interval        | Medium     | Medium | The edition holds per-area rows only; a report quotes one area's rate with that area's interval                                                                                                             |
| The CC BY-NC-ND derivative question blocks the area-rate table                     | Medium     | Medium | Item 1 reads the licence before Item 2 builds the table; if a subset is refused, the edition cites the paper instead of embedding it and the context block waits                                            |

## Open questions

- Is `published_model` a sixth benchmark kind, or is a cited curve a `research_threshold` with a
  function attached? Decided in the behaviour plan.
- What join geometry does the first importer use: a fixed radius, or the operator's named
  segments from the start?
- What window length does a quiet residential site need before counts are printed at all?
  **Answered:** no window turns a residential count into a rate. At 0.27 expected police-reported
  involvements a year, a count of ten takes 37 years ([Finding 6][hcb-f6]). The record prints the
  count, the window, and the interval, with the area rate as the prior; small-count suppression
  still follows the identifiability analysis.
- Sites outside the United States: Transport Canada's National Collision Database publishes no
  coordinates, so a Canadian site may get a model prediction and no local context. Is that
  acceptable, or does the context block need a provincial or municipal adapter first?
- Does the context row belong to the site or to a site configuration period, given that a
  quick-build changes the street the counts describe? **Answered:** the area rate attaches to the
  site's area, which a quick-build does not move, and the counts stay on the site as this plan has
  them ([alignment][hcb-align]).
- Does `l8behaviour.Benchmark` gain an interval field, or do an area rate's bounds stay in the
  edition entry its citation names? Decided in the behaviour plan, beside `published_model`.
- Is an embedded subset of the fatal-rate paper's supplement a derivative under CC BY-NC-ND 4.0,
  and is this project's use non-commercial ([rights rows][hcb-align])? Item 1 settles both before
  Item 2 builds the table.
- Which road type does the context block print for a surface-street site: the surface-street rows
  where the edition has them, or all roads, the only road type the sober paper publishes?

## Sources checked

Checked on October 8, 2026 through search results, except where a row names the
[human crash baselines analysis][hcb] as the reader. **Confirm** marks a claim whose primary
document was not read.

| Source                                                             | What the checked sources support                                                                                                                                                                                                                                                                                                                                                       | Status                                              |
| ------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------- |
| [SHRP2 NDS access][shrp2]                                          | InSight is view-only for registered users and its terms bar downloading sensor data; data use licences are executed with VTTI for research. Published aggregate reports are citable                                                                                                                                                                                                    | Checked                                             |
| Dingus et al. 2016, PNAS                                           | 905 crash events, volume 113 issue 10; the proposal's 68% distraction figure was not found in the sources retrieved                                                                                                                                                                                                                                                                    | **Confirm**                                         |
| [TIMS][tims]                                                       | Terms are set out in a licence agreement the review could not read; attribution to SafeTREC requested; data supplied as-is                                                                                                                                                                                                                                                             | **Confirm**                                         |
| [DataSF terms][datasf]                                             | PDDL v1.0 unless a dataset page says otherwise, per the [licensing standard][datasf-pddl]                                                                                                                                                                                                                                                                                              | **Confirm per dataset**                             |
| [High Injury Network][hin]                                         | SFDPH, 2022 update, at least 10 severe or fatal injuries per mile, streets only; window is 2017 to 2021 in the methodology and 2017 to 2022 in the [catalogue][hin-catalogue]                                                                                                                                                                                                          | **Confirm window**                                  |
| [FARS][fars]                                                       | Census of fatal crashes since 1975, CSV downloads, catalogued as US public domain                                                                                                                                                                                                                                                                                                      | Checked                                             |
| [STATS19][stats19]                                                 | Great Britain road casualty data under the Open Government Licence v3.0                                                                                                                                                                                                                                                                                                                | Checked                                             |
| [Tefft 2013][tefft]                                                | 50% severe-injury risk at about 31 mph and 50% fatality risk at about 42 mph, standardised to 2007 to 2009 US pedestrians and vehicles, per the [NHTSA summary][nhtsa-ped]                                                                                                                                                                                                             | **Confirm from paper**                              |
| [Elvik et al. 2019][elvik]                                         | Power and exponential models both fit post-2000 data well, at individual and aggregate level; see also the [ITF note][itf]                                                                                                                                                                                                                                                             | **Confirm coefficients**                            |
| [Scanlon et al. 2026, TIP][tip]                                    | Fatal crash involvement rates by urban area, road type, and temporal stratum for 2023, with exact Poisson intervals; DOI 10.1080/15389588.2026.2684002; CC BY-NC-ND 4.0, stated on the article; results downloadable as supplemental data, not opened. Read in full by the [analysis][hcb-tip]                                                                                         | Checked; **Confirm** licence reading and supplement |
| Scanlon, Kusano, McMurry, Campolettano, and Victor, JSR (accepted) | Sober and impaired cohort rates by exposure reconstruction, read in full from the preprint by the [analysis][hcb-sober]; per-area values in figures only; the preprint states no terms                                                                                                                                                                                                 | **Confirm on publication**                          |
| [Scanlon, McMurry, Chen, Kusano, and Victor 2026, SAE][sae]        | Police-reported status-quo involvement rates by county, which the sober paper's police-reported phase scales; SAE International Technical Paper 09-14-02-0003, as the sober paper's reference list cites it; neither paper nor terms read                                                                                                                                              | **Confirm**                                         |
| [FHWA Highway Statistics][fhwa] HM-71 and VM-4                     | Vehicle-miles by urban area and functional class (HM-71) and passenger-vehicle share by state (VM-4), the fatal-rate paper's denominators; a US government publication                                                                                                                                                                                                                 | **Confirm** no third-party component                |
| INRIX                                                              | Proprietary; the fatal-rate paper uses its probe-based volumes to distribute the FHWA totals across the temporal strata, and only the aggregates the papers publish are citable ([analysis][hcb-tip])                                                                                                                                                                                  | Checked (paper states source)                       |
| [Valgo `humanbaselines` client][valgo]                             | Apache-2.0 Python client, version 0.4.0 on PyPI, for a key-authenticated REST API returning rate, bounds, N, D_miles, and cells under a selectable definition ([analysis][hcb-valgo]). The valgo.ai and humanbaselines.com pages were not reachable from the review environment, so every page claim is **Confirm**; the terms of use are read before any exploration result is quoted | Checked (README); **Confirm** pages                 |

[shrp2]: https://trb.org/StrategicHighwayResearchProgram2SHRP2/SHRP2DataSafetyAccess.aspx
[tims]: https://tims.berkeley.edu/about.php
[datasf]: https://sf.gov/reports/april-2017/datasf-terms-use
[datasf-pddl]: https://datasf.org/resources/open-data-licensing-standard/
[hin]: https://www.visionzerosf.org/wp-content/uploads/2023/03/2022_Vision_Zero_Network_Update_Methodology.pdf
[hin-catalogue]: https://catalog.data.gov/dataset/vision-zero-high-injury-network
[fars]: https://catalog.data.gov/dataset/fatality-analysis-reporting-system-fars-ftp-raw-data
[stats19]: https://packages.ropensci.org/stats19/doc/stats19.html
[tefft]: https://aaafoundation.org/impact-speed-pedestrians-risk-severe-injury-death/
[nhtsa-ped]: https://nhtsa.gov/book/countermeasures-that-work/pedestrian-safety/understanding-problem
[elvik]: https://swov.nl/en/publicatie/updated-estimates-relationship-between-speed-and-road-safety-aggregate-and-individual
[itf]: https://www.itf-oecd.org/sites/default/files/docs/speed-changes-crash-risk.pdf
[tip]: https://doi.org/10.1080/15389588.2026.2684002
[sae]: https://doi.org/10.4271/09-14-02-0003
[fhwa]: https://www.fhwa.dot.gov/policyinformation/statistics.cfm
[valgo]: https://pypi.org/project/humanbaselines/
[hcb]: ../platform/operations/human-crash-baselines-analysis-2026-10.md
[hcb-f2]: ../platform/operations/human-crash-baselines-analysis-2026-10.md#2-the-exposure-reconstruction-algebra-is-correct
[hcb-f3]: ../platform/operations/human-crash-baselines-analysis-2026-10.md#3-where-the-uncertainty-lives
[hcb-f5]: ../platform/operations/human-crash-baselines-analysis-2026-10.md#5-temporal-strata-the-site-can-align-with
[hcb-f6]: ../platform/operations/human-crash-baselines-analysis-2026-10.md#6-exposure-arithmetic-what-a-site-count-can-and-cannot-say
[hcb-f7]: ../platform/operations/human-crash-baselines-analysis-2026-10.md#7-valgos-definition-sensitivity-from-the-one-number-it-publishes
[hcb-align]: ../platform/operations/human-crash-baselines-analysis-2026-10.md#alignment-with-the-crash-data-integration-plan
[hcb-survey]: ../platform/operations/human-crash-baselines-analysis-2026-10.md#what-a-speed-survey-adds-to-these-datasets
[hcb-incorporate]: ../platform/operations/human-crash-baselines-analysis-2026-10.md#how-to-incorporate-the-datasets
[hcb-sober]: ../platform/operations/human-crash-baselines-analysis-2026-10.md#the-sober-baseline-paper-scanlon-et-al-journal-of-safety-research-accepted
[hcb-tip]: ../platform/operations/human-crash-baselines-analysis-2026-10.md#the-status-quo-base-paper-scanlon-et-al-traffic-injury-prevention-2026
[hcb-valgo]: ../platform/operations/human-crash-baselines-analysis-2026-10.md#valgo-human-crash-baselines

## Checklist

### Complete

- [x] Review of the external proposal against the repository's benchmark, privacy, naming and tooling conventions
- [x] Human crash baselines analysis folded in: the fourth source family, its rights rows, the temporal strata, the area-rate table, the area benchmark context block, and the hosted-baselines exploration

### Outstanding

- [ ] Item 1: primary-source reading and rights record, with the fourth family's rights rows and the fatal-rate supplement's contents (`S`)
- [ ] Item 2: safety-reference edition, `published_model` decision, cited harm paragraph (`M`); area-rate table from the fatal-rate paper's supplement (`M`, v0.5.10 if Item 1 clears the rights, else v0.6.8)
- [ ] Item 2a: benchmark-aligned temporal strata in `data.json` and the PDF (`S`)
- [ ] Item 3: before-and-after model section (`M`)
- [ ] Item 4: `site_crash_context` migration, DataSF importer, report block, second adapter (`L`); site-to-area resolution and area benchmark context block (`M`)

### Deferred

- [ ] Item 5: road-segment attachment, tracked by [posted-speed-limits-plan](posted-speed-limits-plan.md)
- [ ] Mass and front-end geometry in the harm curve, tracked by [vehicle-encyclopedia-plan](vehicle-encyclopedia-plan.md)
- [ ] Research note: multi-site temporal exposure and speed by stratum against the published strata, in `data/explore/` (`S`), after Item 2a runs at several sites; it tests the fatal-rate paper's stated assumption for INRIX-excluded segments only if the supplement carries the temporal vehicle-mile fractions ([analysis][hcb-survey])
- [ ] Hosted-baselines exploration in `data/explore/`: definition sensitivity, posted-speed bands, and coverage beside the pinned edition, each result kept with its definition and retrieval date, with the service's terms of use read first (`S`)

### Accepted residuals (no action planned)

- [ ] SHRP2 kinematic thresholds: not measurable by this sensor, not adopted
- [ ] Per-passage crash or near-crash classification: out of scope here and in the state-estimation plan's Section 12
- [ ] Surrogate-measure-to-crash calibration at a single site: not establishable from one site
- [ ] A site crash rate: not computable at one site, by the arithmetic in the [report][hcb-f6]
