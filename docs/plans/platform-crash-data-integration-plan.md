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
- **Target:** v0.5.10 for the cited harm curves; v0.6.8 for site crash context and the before-and-after model; v1.0 for road-segment attachment
- **Companion plans:** [behaviour analytics](lidar-behaviour-analytics-plan.md), [posted speed limits](posted-speed-limits-plan.md), [vehicle encyclopedia](vehicle-encyclopedia-plan.md), [spatial priors reference data](spatial-priors-reference-data-plan.md)
- **Related:** [data science methodology](../platform/operations/data-science-methodology.md), [identifiability analysis](../platform/architecture/identifiability-analysis.md), [S2 conventions](../lidar/architecture/geographic-indexing.md), [TENETS](../../TENETS.md)
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
  `unsafe`, `danger` or `violat`.
- **Crash data.** No crash or collision dataset, designation layer, or speed-to-harm model is
  referenced anywhere in code, configuration, `data/maths/references.bib` or the docs. SHRP2
  appears once, for its free-flow opportunity method (DOT HS 812 858), already cited in the
  behaviour plan.
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

Three questions a report reader asks, three source families, and the boundary of each.

| Question                                                                         | Source family                                                                                                                                                              | Establishes                                                                                                                                                 | Does not establish                                                                                                                                                                                                                              |
| -------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| How much more does a pedestrian stand to lose at 35 mph than at 25?              | Published impact-speed injury and fatality curves: Tefft 2013 (AAA Foundation, _Accident Analysis & Prevention_), Rosén and Sander 2009, Hussain et al. 2019 meta-analysis | The probability of severe injury or death as a function of **impact** speed, for the population and vehicle fleet the study standardised to, with its curve | Harm from a **travel** speed: a driver brakes before impact. Applied to measured speeds the curve is an upper bound, and the report says so. Mass and front-end geometry, which the encyclopedia supplies later, shift the curve and are absent |
| If mean speed rose after repaving, what does the literature predict for crashes? | Speed-change crash models: Nilsson's power model, Elvik's exponential model, with exponents and intervals from Elvik et al. 2019                                           | The expected relative change in crashes by severity for a change in **mean speed** of the same traffic on the same road, with a confidence interval         | A crash count for this street; anything from a change in p85 or p98 alone; anything across a change in road users or volume                                                                                                                     |
| What has actually happened on this street?                                       | Jurisdiction crash records (DataSF injury crashes; FARS; STATS19) and designation layers (Vision Zero High Injury Network)                                                 | Counts by severity and mode over a stated window near a stated geometry, from a named source with its reporting rules; designation membership and version   | A rate, without exposure; a trend, without a long window; causation from the measured speeds; anything about an individual passage                                                                                                              |

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
- **Edition.** A year-marked identifier, the date each source was read, and a digest. The report
  prints the edition it rendered with. A correction is a new edition; a published report stays
  reproducible.

No network, no runtime fetch, no crash records in the edition. This follows Tenet 7 and the
encyclopedia's edition model exactly, at a size of a few kilobytes.

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
4. **Wording guard.** The headway report's verdict test extends to these sections. "Risk score",
   "unsafe", "dangerous" and "likelihood" do not reach a page.

### Machine access

The report's `data.json`, the edition JSON and the context row are the machine-readable surface.
An agent that wants to reason over them reads the same bytes the template does, with the same
provenance. No prompt protocol is added; one line in the `.github/knowledge/` index points at the
benchmark taxonomy and this plan.

### Boundaries

No composite score. No per-track geography. No runtime network access. No SHRP2 kinematic
thresholds. No "likelihood" column. No crash records in the binary. No claim that a surrogate
measure predicts a crash at this site.

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
5. Add every source to `data/maths/references.bib` and a dated verified-sources table to this
   plan.

**Milestone:** v0.5.10

### Item 2: safety-reference edition and the cited harm paragraph

**Summary:** Ship the edition package and replace the uncited report copy.

**Steps:**

1. `internal/report/safetyref`: edition JSON, loader, validation that every entry carries a
   citation, population and unit, and a test that the embedded edition validates.
2. The `published_model` benchmark kind, or the recorded decision not to add one.
3. The cited harm paragraph in `sections.typ`, driven from `data.json`, in the sign's unit, with
   the impact-versus-travel caveat and the edition identifier.
4. The wording-guard test over the new section.
5. Answer Q30 to the extent the curves allow and record the decision in `data/QUESTIONS.md`.

**Milestone:** v0.5.10

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

**Summary:** A Go importer for DataSF, the `site_crash_context` migration, and the report block.

**Steps:**

1. Migration for `site_crash_context` with the field groups above.
2. `velocity tools import-crash-context --source datasf --site <id> --input <dir>`: parse the
   extract, join by radius, print the candidate segments, write the row only with `--confirm`.
3. Small-count handling per the identifiability analysis, tested.
4. The site context block in the report, with source and licence line.
5. A second adapter, FARS or STATS19, to prove the source interface is not SF-shaped.

**Milestone:** v0.6.8

### Item 5: attach context to road segments

**Summary:** Move the join from a radius to the vector-scene road segments once they exist.

**Milestone:** v1.0, after the posted speed limits plan

## Dependencies

- [Posted speed limits](posted-speed-limits-plan.md) for any compliance framing; until then the
  harm paragraph uses the per-request limit and says it was operator-supplied.
- The merged `sites` table with coordinate provenance, in the
  [vocabulary plan](platform-vocabulary-and-data-model-plan.md), for where the context row hangs.
- G-SMO-1 before any LiDAR kinematic quantity is compared with anything.
- The [vehicle encyclopedia](vehicle-encyclopedia-plan.md) for mass and front-end geometry, which
  this plan does not wait for.

## Risks

| Risk                                                                               | Likelihood | Impact | Mitigation                                                                                                                        |
| ---------------------------------------------------------------------------------- | ---------- | ------ | --------------------------------------------------------------------------------------------------------------------------------- |
| A curve for impact speed is read as a statement about travel speed                 | High       | High   | The caveat is printed in the section; the edition entry names the independent variable; the guard test checks the phrase is there |
| A source's licence forbids redistribution of the aggregate                         | Medium     | High   | Rights manifest per source; the row stores counts, not records; TIMS not used until its agreement is read                         |
| Small crash counts near a quiet site identify a person                             | Low        | High   | Window lengthening and cell suppression from the identifiability analysis; never victim-level fields                              |
| The before/after model is applied to a p85 change or across a configuration change | Medium     | Medium | The section refuses unless both periods share site configuration and sensor; mean speed only                                      |
| Reviewers want a single "safety score" anyway                                      | Medium     | Medium | The behaviour plan's Section 1 rule, restated here; the wording guard makes the score impossible to print                         |
| Figures in this plan drawn from search summaries are wrong                         | Medium     | Medium | Item 1 is the first milestone and replaces every marked figure from the primary paper                                             |
| The join radius misattributes an adjacent arterial's crashes to a side street      | Medium     | Medium | Operator confirmation of segments; segment attachment at v1.0                                                                     |

## Open questions

- Is `published_model` a sixth benchmark kind, or is a cited curve a `research_threshold` with a
  function attached? Decided in the behaviour plan.
- What join geometry does the first importer use: a fixed radius, or the operator's named
  segments from the start?
- What window length does a quiet residential site need before counts are printed at all?
- Sites outside the United States: Transport Canada's National Collision Database publishes no
  coordinates, so a Canadian site may get a model prediction and no local context. Is that
  acceptable, or does the context block need a provincial or municipal adapter first?
- Does the context row belong to the site or to a site configuration period, given that a
  quick-build changes the street the counts describe?

## Sources checked

Checked on October 8, 2026 through search results. **Confirm** marks a claim whose primary
document was not read.

| Source                     | What the search results support                                                                                                                                                     | Status                   |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------ |
| [SHRP2 NDS access][shrp2]  | InSight is view-only for registered users and its terms bar downloading sensor data; data use licences are executed with VTTI for research. Published aggregate reports are citable | Checked                  |
| Dingus et al. 2016, PNAS   | 905 crash events, volume 113 issue 10; the proposal's 68% distraction figure was not found in the sources retrieved                                                                 | **Confirm**              |
| [TIMS][tims]               | Terms are set out in a licence agreement the review could not read; attribution to SafeTREC requested; data supplied as-is                                                          | **Confirm**              |
| [DataSF terms][datasf]     | PDDL v1.0 unless a dataset page says otherwise, per the [licensing standard][datasf-pddl]                                                                                           | **Confirm per dataset**  |
| [High Injury Network][hin] | SFDPH, 2022 update, at least 10 severe or fatal injuries per mile, streets only; window is 2017 to 2021 in the methodology and 2017 to 2022 in the [catalogue][hin-catalogue]       | **Confirm window**       |
| [FARS][fars]               | Census of fatal crashes since 1975, CSV downloads, catalogued as US public domain                                                                                                   | Checked                  |
| [STATS19][stats19]         | Great Britain road casualty data under the Open Government Licence v3.0                                                                                                             | Checked                  |
| [Tefft 2013][tefft]        | 50% severe-injury risk at about 31 mph and 50% fatality risk at about 42 mph, standardised to 2007 to 2009 US pedestrians and vehicles, per the [NHTSA summary][nhtsa-ped]          | **Confirm from paper**   |
| [Elvik et al. 2019][elvik] | Power and exponential models both fit post-2000 data well, at individual and aggregate level; see also the [ITF note][itf]                                                          | **Confirm coefficients** |

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

## Checklist

### Complete

- [x] Review of the external proposal against the repository's benchmark, privacy, naming and tooling conventions

### Outstanding

- [ ] Item 1: primary-source reading and rights record (`S`)
- [ ] Item 2: safety-reference edition, `published_model` decision, cited harm paragraph (`M`)
- [ ] Item 3: before-and-after model section (`M`)
- [ ] Item 4: `site_crash_context` migration, DataSF importer, report block, second adapter (`L`)

### Deferred

- [ ] Item 5: road-segment attachment, tracked by [posted-speed-limits-plan](posted-speed-limits-plan.md)
- [ ] Mass and front-end geometry in the harm curve, tracked by [vehicle-encyclopedia-plan](vehicle-encyclopedia-plan.md)

### Accepted residuals (no action planned)

- [ ] SHRP2 kinematic thresholds: not measurable by this sensor, not adopted
- [ ] Per-passage crash or near-crash classification: out of scope here and in the state-estimation plan's Section 12
- [ ] Surrogate-measure-to-crash calibration at a single site: not establishable from one site
