# Vehicle encyclopedia (v1.x)

- **Status:** Draft, open for investigation. No source verified, no code, no commitment
- **Layers:** Separate open project; velocity.report consumes an embedded subset
- **Target:** investigation spikes in v0.6.3; delivery from v1.0
- **Companion plans:** [scene vehicle identification](scene-vehicle-identification-plan.md), [web scene export](lidar-web-scene-export-plan.md), [scene catalogue publishing](lidar-scene-catalogue-publishing-plan.md)
- **Canonical:** [Vehicle encyclopedia](../platform/architecture/vehicle-encyclopedia.md) (single source of truth)
- **Related:** [shape descriptors](lidar-shape-descriptors-plan.md), [TENETS](../../TENETS.md)

## Motivation

velocity.report measures how fast things move past a kerb. It cannot say what
those things weigh, how far they take to stop, how much of the road ahead their
bonnet hides, or how much energy they carry into a person. Speed alone
understates the change of the last thirty years: the fleet did not only get
faster in places, it got heavier, taller and blunter, and the harm curve moved
with it.

An encyclopedia of vehicle makes and models supplies the missing half. Paired
with a measured speed it yields kinetic energy, stopping distance, and front
blind zone for the traffic that actually passed a given kerb, on a given
afternoon. That is an argument a neighbourhood meeting can use, and it is not
available anywhere as one artefact today.

The consequence of not building it: the project keeps publishing speeds, and
speeds alone have been published for fifty years without moving very much.

## What this is, and what it is not

| It is                                                         | It is not                                                     |
| ------------------------------------------------------------- | ------------------------------------------------------------- |
| A catalogue of manufactured products, by make, model and year | A record of who owns what                                     |
| Public reference data, versioned and citable                  | Observation data                                              |
| Editable by the public, approved by review                    | Open write access to the measurement database                 |
| A source of mass, geometry and published test results         | A source of colour, registration, or any per-vehicle identity |

The encyclopedia and the scene are separate artefacts that meet only at render
time. That separation is the whole privacy argument and it is not negotiable.

## Settled

These were answered on 2026-09-22 and are no longer open.

| Question                          | Answer                                                                                           |
| --------------------------------- | ------------------------------------------------------------------------------------------------ |
| Which market?                     | Canada. National is the baseline; provinces and territories are the regional grain               |
| What fills domain 6?              | Direct vision and blind-zone geometry, as a sourced domain                                       |
| Where does the encyclopedia live? | A separate open project, consumed here as an embedded subset                                     |
| How is it versioned?              | Year-marked editions on an annual release cadence                                                |
| Does the web annotate?            | No. The web stays read-only: see the [identification plan](scene-vehicle-identification-plan.md) |

### Market

Canada. That decision reaches further than a flag on a page: units are metric
throughout, vehicle classes follow the federal safety standards rather than a
foreign regulator's, the regional grain is the province or territory, the
insurance picture is provincial rather than a single national market, and
federal source data arrives in both official languages. A record's `market`
field stays, because a model sold in Canada and the same nameplate sold
elsewhere are frequently different vehicles, and the catalogue should be able to
say which one it describes.

### Housing

The encyclopedia is its own repository and its own service, on the pattern the
spatial-priors work already sets. velocity.report consumes a small versioned
subset, embedded in the binary and usable with no network, and the full
catalogue with its public editor lives online.

Three tenets push this way. Tenet 4 requires the Pi to work offline, which a
public wiki cannot promise. Tenet 7 requires a release to be one versioned unit,
which is achievable for an embedded data subset and not for a live catalogue.
And tenet 1 is easier to hold when the public write path is not inside the
binary that holds the measurements: a wiki with open contribution has an attack
surface that a sensor should not inherit.

What crosses the boundary is a versioned data artefact, pulled at build time and
pinned by revision, never a runtime dependency. If the online catalogue vanishes,
every deployed sensor keeps working with the subset it shipped with.

### Editions

The catalogue publishes **year-marked editions on an annual cadence**: one
frozen, citable artefact per year, with the model years known at the time of
cutting. An edition is the unit a consumer pins, a scene cites, and a paper
references.

| Property        | Rule                                                                        |
| --------------- | --------------------------------------------------------------------------- |
| Identity        | The edition year, plus a patch number for corrections within that year      |
| Immutability    | A published edition never changes; a correction is a new patch, not an edit |
| Additive growth | A new model year is added in the next edition; prior editions stay valid    |
| Field history   | Each field records the edition in which its current value was set           |
| Consumption     | An embedded subset pins exactly one edition                                 |
| Citation        | An identification records the edition it resolved against                   |

That last row is the one that earns the machinery. Tenet 3 requires a claim to
be reproducible, and an identification is only reproducible if you know which
catalogue the reader used. A scene published against the 2026 edition and
re-examined in 2031 must resolve to the same answer, because the 2026 edition is
still there and still says the same thing. Without editions, every catalogue
correction silently rewrites the history of every published scene.

A patch exists for corrections: a mass that was wrong, a source that was
misread. New knowledge that is not a correction waits for the next edition.

## Privacy gate

Tenet 1 is absolute, and what it forbids is a published artefact that singles
somebody out. Whether a given artefact does that is a number, not a judgement:
the size of the anonymity set it leaves behind. The
[identifiability analysis](../platform/architecture/identifiability-analysis.md)
derives it. Three results from that analysis shape everything here.

**Publishable depth scales with traffic volume, logarithmically.** The anonymity
set for one sighting is the expected count of that class in the published
window. A hundredfold difference in volume buys roughly three and a half extra
levels of the class tree, so a quiet residential street supports a body form and
a busy arterial can reach a catalogue entry. A single blanket depth would be
wrong on both.

**Repetition collapses anonymity geometrically; coarser classes buy it back only
logarithmically.** A continuously published timestamped per-pass stream singles
out regular users within days, whatever label it carries. That makes the
deployment mode, not the label, the first-order control.

**On a quiet street the timestamp is the identifier.** Fewer than two vehicles
pass in the ten minutes around any given time, so a published pass at 08:12
identifies whoever the reader already knows drives at 08:12, with no class
attached at all.

The gate that follows.

1. **Publishable depth is computed per artefact**, from the realised counts in
   that artefact, the deployment mode, and what the sensor resolved. Never
   configured, never a constant.
2. **Published precision matches the published class.** An export never carries
   numbers that out-resolve its own label: a reader with the catalogue and
   arithmetic could otherwise recover the entry. Geometry is quantised to the
   precision of the class it accompanies.
3. **A counted period is not a labelled pass.** A per-model breakdown in a
   report is a fleet census with nobody in it and may go deeper than a scene,
   subject to small-cell suppression and to watching the time series of a cell
   rather than only its value.
4. **Continuous deployments publish aggregates, surveys may publish passes.**
5. **No colour, ever.** LiDAR returns intensity, not colour. Colour is the
   strongest re-identifier after the plate, the sensor cannot see it, and the
   system will not infer it.
6. **One-way by construction.** The encyclopedia never gains an observation
   column. No page answers "where has this model been seen".

Internally the default is resolve, increment an aggregate counter, discard. A
fleet mass distribution needs a count per entry, not a row per vehicle, and an
operator who wants more retains it locally and explicitly.

None of this defends against somebody who already knows the target's vehicle and
their routine. No publication rule does, and the analysis says so rather than
implying a protection it cannot provide.

## Data domains

The numbered domains below follow the original framing, with domain 6 settled
as direct vision and blind-zone geometry.

| #   | Domain                        | Grain                                | Derived or sourced |
| --- | ----------------------------- | ------------------------------------ | ------------------ |
| 1   | Mass                          | make, model, year, trim              | Sourced            |
| 2   | Fleet make-up over time       | model year, sales volume, since 1940 | Sourced            |
| 3   | Mass and speed profile trend  | model year, road-fleet estimate      | Derived from 1, 2  |
| 4   | Registration counts           | province, city, make, model          | Sourced            |
| 5   | Power and acceleration        | make, model, year, trim              | Sourced, modelled  |
| 6   | Direct vision and blind zones | make, model, year, trim              | Sourced, modelled  |

### 1. Mass

Kerb mass and gross vehicle mass rating, per trim where trims differ materially.
Trim matters more than it looks: the same model name can span 400 kg across a
generation, and an electric variant can add 300 kg over its combustion sibling.
Record a range with a distribution, not a single number, and record which trim a
figure belongs to.

### 2. Fleet make-up over time

Units sold by model year, from 1940 forward. This is where honesty costs the
most. Post-1975 model-level volumes are well documented. Pre-1970 they are
patchy, secondary, and frequently reconstructed from a manufacturer's annual
report that counted something slightly different. Every cell carries its own
provenance and a confidence grade, and the surface shows the grade. A chart that
looks equally confident in 1948 and 2018 is lying.

### 3. Mass and speed profile over time

The derived layer, and the reason domains 1 and 2 are worth the effort. Sales by
model year convolved with a survival curve gives an estimate of what is on the
road in a given year, not what was sold in it. Mass-weight that estimate and the
result is "the median vehicle on a road in this province weighed X in 1990 and Y
today". Cross it with velocity.report's own measured speeds and the same series
becomes measured kinetic energy over time, for one kerb, against a national
baseline.

Survival curves are themselves a sourced input, not an assumption. Scrappage
differs by class, by province and by decade.

### 4. Registration counts

Per province or territory, and per city where one publishes it. Registration is
administered provincially in Canada, so this domain has thirteen publishers
rather than one, with thirteen different views on what may be released.

Two hard constraints. Individual registration records are out of the question at
any grain: federal and provincial privacy law governs them, only aggregate
counts by make, model and year are in scope, and only from a publisher entitled
to release them. Second, model-level counts below provincial scale are mostly
sold rather than published, so budget gates this domain rather than engineering.

One structural advantage over the comparable work in the United States is worth
naming. Several provinces run public auto insurers, which publish claims and
loss data at a grain a private market treats as commercial. Where that holds, an
insurance-loss figure per model may be obtainable free and citably, and the
registration denominator may come from the same publisher, which makes the two
consistent with each other rather than stitched from different sources.

This domain also supplies the rarity floor. Without it, rule 2 of the privacy
gate has no input and identification must stay at body-class rung everywhere.

### 5. Power and acceleration

Rated power, torque, and published standing-start times where they exist. Where
they do not, a modelled envelope from power-to-mass, drivetrain and tyre class,
clearly marked as modelled. The surface must never present a modelled number in
the same style as a measured one.

### 6. Direct vision and blind zones

Bonnet leading-edge height, bonnet length, A-pillar geometry, glazing line and
mirror position, per model and trim. This is the domain that explains the change
a street has felt without being able to name: a bonnet that clears a child's
head hides the child.

Two figures come straight out of it. The **front blind zone** is the distance
ahead of the bumper at which an object of a stated height first becomes visible
from the driver's seat: a geometric construction from bonnet height, driver eye
height and seat position, so it is comparable across models in a way a
manufacturer's prose is not. The **direct vision score** is the same idea
generalised around the vehicle, and at least one regulator already publishes a
graded standard for it, which is worth reusing rather than reinventing.

Sourced where a regulator or a test house publishes the measurement, modelled
from published geometry where not, and never guessed. The eye-height assumption
is the sensitive input and belongs in the surface next to the number, not in a
footnote: a blind zone quoted for a 1.75 m driver and one quoted for a 1.55 m
driver are different vehicles.

## Derived safety metrics

These are the per-vehicle figures the scene surface shows, and the population
surface aggregates.

| Metric                   | Computed from                               | Honest caveat                                                     |
| ------------------------ | ------------------------------------------- | ----------------------------------------------------------------- |
| Kinetic energy           | encyclopedia mass, measured speed           | Intuitive, and a poor proxy for pedestrian harm: see below        |
| Weight class             | kerb mass, banded                           | Bands need publishing, not inventing                              |
| Bonnet height            | domain 6, sourced                           | Trim and tyre choice move it; quote the variant                   |
| Front blind zone         | domain 6 geometry, stated eye height        | Quote the eye height beside the number, never below it            |
| Braking distance         | published 60-0 test, or tyre and mass model | Test surface and tyre wear dominate; a single number is a fiction |
| Insurance loss           | published per-model loss data               | Measures claims, which mixes vehicle, driver and use              |
| Pedestrian fatality risk | impact speed curve, front-end geometry      | The literature is on speed and front geometry, not on mass        |

One correction worth making in the surface itself rather than in a footnote.
Half m v squared is the right instinct for vehicle-to-vehicle severity and for
what a bollard has to absorb. It is the wrong instinct for a person: pedestrian
fatality risk rises with impact speed far more sharply than with vehicle mass,
and front-end geometry, specifically bonnet leading-edge height, changes the
injury pattern from "thrown onto the bonnet" to "pushed under". Showing kinetic
energy alone would repeat a common mistake with better graphics. Show both, name
both, and let the reader see that the heavy tall thing at 30 km/h and the light
low thing at 50 km/h are dangerous in different ways.

## Candidate sources

**None of the following is verified.** This table is the input to the first
investigation spike, not a statement of availability, licence or cost.

| Source                                           | Would supply                      | To verify                                    |
| ------------------------------------------------ | --------------------------------- | -------------------------------------------- |
| Federal statistics agency, vehicle registrations | Counts by province and class      | Model-level grain, likely absent             |
| Federal statistics agency, new vehicle sales     | Sales by year and segment         | Model grain, back-range, revision policy     |
| Federal fuel consumption guide                   | Mass, power, class, by model-trim | Whether the published mass is kerb mass      |
| Federal transport department, safety standards   | Class definitions, compliance     | Which class vocabulary the catalogue adopts  |
| Federal collision database                       | Collision outcomes                | Whether vehicle model is recorded at all     |
| Provincial public insurers                       | Loss and claims by model          | Which provinces, what grain, what licence    |
| Provincial registries and open data portals      | Registrations by model            | Which publish model grain, and on what terms |
| Foreign pedestrian and front-geometry ratings    | Direct vision, front-end scoring  | Applicability to Canadian-market variants    |
| Commercial registration data vendors             | City and provincial model counts  | Cost, redistribution rights                  |
| Encyclopedic community sources                   | Sales history before the 1970s    | Licence compatibility, accuracy              |
| Manufacturer press and specification kits        | Specs, geometry, shell dimensions | Licence, archival availability, market fit   |

Three conclusions to test rather than assume: that national fleet aggregates are
free and reach back several decades; that model-level counts below provincial
scale are the expensive part; and that at least one public insurer publishes
loss data by model under terms that allow redistribution. If the third holds, it
is the cheapest good data in the whole plan, and the insurance metric stops
being blocked on a commercial licence.

Canadian-market caveat, which applies to every foreign source in that table: a
nameplate sold here is often not the vehicle tested elsewhere. Trim mix, engine
availability and safety equipment differ, and a front-geometry rating measured
on a foreign variant is evidence about that variant. Record which market a
figure describes, and let the surface say so.

## The record

One encyclopedia entry is a model-year-trim. The entry is the unit of editing,
review, citation and versioning.

| Group      | Fields                                                               |
| ---------- | -------------------------------------------------------------------- |
| Identity   | make, model, generation, model year range, trim, body style, market  |
| Mass       | kerb mass, gross rating, distribution across trims                   |
| Geometry   | length, width, height, wheelbase, track, bonnet leading-edge height  |
| Shell      | station profile, landmarks, axle stations, wheel diameter: see below |
| Stance     | suspension lift front and rear, fitted tyre diameter front and rear  |
| Power      | rated power, torque, drivetrain, standing-start time                 |
| Braking    | published stopping distance, test conditions                         |
| Population | units sold by model year, registrations by province or territory     |
| Safety     | published test results, insurance loss indices, collision outcomes   |
| Assets     | canonical mesh, descriptor prototypes, synthetic scan set            |
| Provenance | per-field source, retrieval date, confidence grade, editor, edition  |

Provenance is per field, not per record. A record whose mass came from a
manufacturer kit and whose 1958 sales figure came from a secondary compilation
must be able to say so in both places, and each field carries the edition in
which its current value was set.

## The outer shell

The shell is the geometric core of a record and the thing most of the derived
work reads. It is deliberately not the 3D model.

A shell is a set of **transverse stations** along the vehicle's longitudinal
axis, from front bumper to rear, each carrying four numbers: the top height, the
shoulder height where the glazing meets the body, the body width, and the
underbody clearance. Alongside the stations sit **landmarks** that a station
grid would blur: the bonnet leading edge, the windscreen base, the A-pillar
foot, the roof front and rear, the bed or boot step, and the bumper faces.
Finally the **axle stations**, track width and wheel diameter, because
everything about stance is defined relative to the axles.

Twenty stations plus landmarks is on the order of a hundred numbers. Quantised
to millimetres that is a few hundred bytes a vehicle, which is what makes an
embedded subset of several thousand vehicles a file rather than a download.

Four properties earn the shape:

- **It is contributable.** A person with a tape measure and a level can record a
  shell. Nobody can produce a mesh that way. For a public catalogue, the
  artefact people can actually supply is the one that matters.
- **It is inspectable.** A shell is a table of measurements, each row of which
  is a statement somebody can check against a real vehicle in a car park.
- **It is transformable.** Stance changes are a rigid transform of the body
  stations relative to the axle stations, so a lifted variant needs no new
  measurements and no new mesh.
- **It is enough.** Footprint, bonnet height, blind zone, roofline and the
  descriptor prototypes all come out of the shell. The detailed mesh is for
  rendering a record page and for close-range synthetic scans where fine
  structure matters, and is derived or donated rather than required.

### Footprint estimator

The shell gives three footprints, in increasing usefulness and difficulty.

| Footprint | From                                      | Says                                                 |
| --------- | ----------------------------------------- | ---------------------------------------------------- |
| Static    | Width profile integrated over length      | The plan area a parked vehicle takes from the street |
| Dynamic   | Static, plus stopping distance at a speed | The road a moving vehicle has effectively claimed    |
| Swept     | Static, plus turning geometry             | What a turn demands of a junction                    |

Static is available as soon as a shell exists, and it is the one that carries
the argument: two vehicles rated for the same number of people can differ by
half a parking space. Dynamic needs braking from domain 5 and a measured speed,
which the sensor supplies. Swept needs a turning circle, which is not currently
a recorded field and should become one if the junction-geometry argument is
worth making.

## Stance: lift, squat and fitted tyres

A modified vehicle is not the vehicle in the manufacturer's brochure, and it is
the modified one that drives past the sensor. Stance is recorded as four
numbers against a base record rather than as a separate record, because
everything except geometry is unchanged.

| Field                       | Meaning                                                                      |
| --------------------------- | ---------------------------------------------------------------------------- |
| Suspension lift, front      | Body and frame raised relative to the front axle centre; negative is lowered |
| Suspension lift, rear       | The same at the rear; a negative rear against a positive front is a squat    |
| Fitted tyre diameter, front | Replaces the stock diameter; raises the axle centre by half the difference   |
| Fitted tyre diameter, rear  | The same at the rear                                                         |

Total ride-height change at each axle is the suspension lift plus half the tyre
diameter difference, and the difference between the two ends is the rake. One
field pair covers lifted, lowered and squatted vehicles, which is cleaner than
three concepts that all mean "the body moved relative to the axles".

Three reasons this is not a detail.

**It moves the number the safety argument rests on.** Bonnet leading-edge height
is domain 6's headline, and a lift raises it directly. A catalogue holding only
stock geometry understates the blind zone of exactly the vehicles whose blind
zone is worst.

**The sensor sees the modified vehicle, not the record.** A lifted pickup will
not match a stock prototype, so stance variants have to generate prototypes of
their own. Because a lift is a rigid transform of the body stations relative to
the axle stations, those prototypes are generated from the base shell rather
than authored, which keeps the cost near zero.

**It is measurable and currently unmeasured.** Some jurisdictions regulate lift
and some have moved against squatted vehicles specifically. Nobody can say how
much of a local fleet is modified, because nothing counts it. A sensor that
records ride height against a matched base record can.

Out of scope here: what a lift does to mass distribution, centre of gravity and
rollover propensity. Real, and a different plan.

## Wiki editing and the approval queue

Public editing with a review gate. The states are `proposed`, `approved` and
`rejected`, and they are deliberately the same three the annotation sidecar
already uses: one review vocabulary across the project, not two.

| State      | Who sets it               | Visible where                                |
| ---------- | ------------------------- | -------------------------------------------- |
| `proposed` | Any contributor           | Proposal queue, and on the record as pending |
| `approved` | A reviewer                | Public record, scene identification, exports |
| `rejected` | A reviewer, with a reason | Proposal history, never on the public record |

Only an `approved` record gets a generated 3D model and enters the
identification set. A proposal can carry an uploaded or generated mesh, which
sits in the queue with it.

Everything a public wiki needs is already a solved problem in this repository in
a different shape: full revision history, optimistic concurrency against a
digest of the loaded bytes, restore-as-new-revision rather than rewrite, and a
tri-state review model where an algorithm's proposal never silently becomes
truth. The annotation sidecar store implements all of it. Reuse the model, and
where practical the code, rather than importing a wiki engine.

Vandalism, licence hygiene and attribution are the parts that are genuinely new.
A contributor pasting manufacturer copy or uploading scraped CAD is the likely
failure, not someone typing rude words into a field.

## The point-cloud bridge

This is the part that makes the encyclopedia belong to this project rather than
being a car website with better typography.

```text
 approved record
       |
       +--> shell (+ stance transform) --(ray-cast: sensor pose, beams, range)--> synthetic scan
       |                                                                                |
       +--> canonical mesh -------------(close range, fine structure)-------------------+
                                                                                        |
                                                                                        v
                                                                        descriptor prototype
                                                                                        ^
                                                                                        |
 observed cluster --(L4 shape descriptors)----------------------------------------------+
                                                                                        |
                                                                                        v
                                                                      nearest neighbour, calibrated
```

The bridge works because both sides reduce to the same descriptor. The
shape-descriptor plan already specifies what L4 will compute for an observed
cluster: eigenvalue features, vertical mass profile, ground footprint, return
character. Ray-cast a shell from a real sensor pose, with the real beam geometry
and the real range, and the synthetic returns go through the identical code
path. Matching is then a nearest-neighbour search in a shared space, with a
confidence that falls off honestly with range because the synthetic scan gets
just as sparse as the real one.

The shell carries the everyday case and the mesh is the refinement. At the
ranges that matter for a roadside sensor, a shell and a mesh produce
indistinguishable returns, because the sensor cannot resolve the difference
between them. The mesh earns its keep close in, and on the record page.

Two consequences worth stating up front. Identification degrades with distance
by construction, which is correct and should be visible in the interface. And
the reviewed masks in existing annotation packs are real point sets for real
vehicles: where a person also names the model, that is a labelled exemplar and
the validation corpus writes itself out of work already being done.

Prototypes are generated, not authored. One base shell plus a stance transform
plus a range bin yields a prototype row, so a lifted variant costs a row rather
than a survey, and a new edition regenerates the whole prototype table from the
shells it ships. How that table is searched without becoming an opaque weight
file is the [identification plan](scene-vehicle-identification-plan.md)'s
problem, and it has an answer.

## Population exploration

The encyclopedia's own surface, independent of any scene.

Facets: make, model, year range, body class, weight band, power band, bonnet
height band, drivetrain, market. Measures: count, share of fleet, mass
percentiles, energy at a chosen speed, stopping distance, blind-zone length.
Groupings: by year, by make, by class, by region.

The chart policy already set for this repository applies. Report-shaped charts
are served as SVG by the Go chart package; live interactive charts are
LayerChart. No ad-hoc SVG in routes.

The one view that has to exist, because it is the argument: a distribution of
kinetic energy for vehicles measured at a specific kerb, drawn over the same
distribution for the Canadian fleet at the same speeds, with the difference
called out in words.

### The fleet census in a report

The PDF report gains a per-entry breakdown over the reporting period: how many
passes of each make, model and year range, with mass, bonnet height and energy
alongside. This is the surface where model-level detail belongs, and it is
allowed to go deeper than a scene because a count over a period is a census with
nobody in it: no timestamp to link on, no pass to point at. The
[identifiability analysis](../platform/architecture/identifiability-analysis.md)
sets out why the two artefacts are governed differently.

Standard statistical disclosure control applies, and it is not optional:

| Control       | Rule                                                                                               |
| ------------- | -------------------------------------------------------------------------------------------------- |
| Small cells   | Suppress counts below the anonymity floor                                                          |
| Complements   | Suppress enough other cells that a suppressed one cannot be recovered by subtraction               |
| Time series   | Watch a cell's history, not only its value: a monthly count reading 1, 1, 1, 1 is one regular user |
| Period length | Lengthen the period rather than suppressing the model, where the data allow                        |

The census is also where the argument that needs names actually lives.
Practically every claim worth making about a specific model is a population
claim, and a population claim belongs in a report rather than against a moving
box in a scene.

## National and provincial comparison

Three baselines, in increasing difficulty.

| Baseline                          | Grain                  | Expected difficulty             |
| --------------------------------- | ---------------------- | ------------------------------- |
| National fleet by model year      | mass, power, footprint | Low, if federal aggregates hold |
| Provincial registrations by class | class, not model       | Low to medium                   |
| City registrations by model       | make, model, year      | High, probably paid             |

A comparison must state its own grain. "This junction against the national
fleet" is a sound claim when both sides are class-level. "This junction against
this city" is a stronger claim and needs the expensive data. The surface should
refuse to draw a comparison whose two sides are at different grains rather than
quietly rescaling one.

Provincial variation is not noise to be averaged away, and it is why the
provincial baseline earns its place beside the national one. Fleet composition,
climate, terrain, fuel price and insurance regime all differ across the country,
and a junction compared only against a national mean will look anomalous for
reasons that have nothing to do with the junction.

## Visual layout, first pass

The design work is a workstream of its own. These wireframes are the starting
point for it, not the outcome.

**Vehicle record**

```text
+---------------------------------------------------------------+
| Make Model, 2019-2024                     [proposed edits: 2]  |
+------------------------------+--------------------------------+
|                              |  Mass        1,940-2,310 kg    |
|      3D model, orbit         |  Class       Light truck       |
|                              |  Bonnet      1.14 m            |
|                              |  Blind zone  4.2 m            |
|                              |  0-100       6.1 s (measured) |
|                              |  60-0        38 m  (modelled) |
+------------------------------+--------------------------------+
| Energy at speed    [====slider 30 km/h====]                    |
| 231 kJ    vs median vehicle here 118 kJ   vs Canada 141 kJ    |
+---------------------------------------------------------------+
| Sales by model year   [chart, confidence shaded]               |
+---------------------------------------------------------------+
| Sources: mass (kit, 2024-03-01, high) - sales (secondary, low) |
+---------------------------------------------------------------+
```

**Population explorer**

```text
+------------+--------------------------------------------------+
| Facets     |  Group by: [year v]  Measure: [energy at 30 v]   |
| make       |                                                  |
| class      |  +--------------------------------------------+  |
| weight     |  |                                            |  |
| power      |  |   distribution, this kerb vs Canada        |  |
| bonnet     |  |                                            |  |
| year       |  +--------------------------------------------+  |
|            |                                                  |
| [k floor]  |  1,204 models - 62% of local fleet identified    |
+------------+--------------------------------------------------+
```

## Workstreams

Sized for parallel work, ordered by what unblocks what.

| #   | Workstream                                                               | Depends on           | Effort |
| --- | ------------------------------------------------------------------------ | -------------------- | ------ |
| A   | Source verification and licensing pass                                   | nothing              | `M`    |
| B   | Privacy gate specification and rarity floor measurement                  | A (domain 4)         | `M`    |
| C   | Record schema, provenance model, storage, embedded-subset contract       | A                    | `M`    |
| C2  | Shell format, stance transform, and the footprint estimator              | C                    | `M`    |
| C3  | Edition machinery: cutting, patching, pinning, and citation              | C                    | `M`    |
| D   | Wiki editor, review queue, revision reuse                                | C                    | `L`    |
| E   | Shell and mesh pipeline, stance transform, synthetic scan generation     | C, C2                | `L`    |
| F   | Descriptor bridge and match calibration                                  | E, shape descriptors | `L`    |
| G   | Population explorer and chart surface                                    | C                    | `M`    |
| G2  | Fleet census report section, with small-cell and time-series suppression | G, B                 | `M`    |
| H   | National and provincial baselines                                        | A                    | `M`    |
| I   | Design passes for every surface above                                    | wireframes           | `M`    |

Workstream A gates almost everything and is cheap. Do it first and let its
findings resequence the rest.

## Open questions

1. **What is `k`?** The rarity floor needs a number and a rung ladder. It cannot
   be chosen from taste: it should come from measuring, on real provincial
   registration data, how many models fall below each candidate threshold and
   what share of observations would degrade to a coarser rung as a result.
2. **How many stations does a shell need?** Twenty is a guess. The right number
   is the one at which adding stations stops changing the descriptor, measured
   against real point clouds rather than chosen for tidiness. Too few blurs the
   bonnet step that distinguishes a pickup; too many is measurement nobody will
   contribute.
3. **Who reviews?** A public wiki with one reviewer is a bottleneck; a public
   wiki with open review is a vandalism target. Trust levels, or invitation, or
   a quorum.
4. **What is in the embedded subset?** The whole catalogue will not fit on a Pi
   alongside everything else, and a subset chosen by popularity cannot identify
   the unusual vehicle. Probably the provincial fleet by registration count,
   which makes the subset regional and the rarity floor a build input.
5. **How common are modified vehicles?** Stance fields are worth the schema only
   if a meaningful share of the fleet is modified. Nothing counts it today, so
   the first honest answer will come from the sensor: matched vehicles whose
   measured ride height sits well above their base record.
6. **Licence for contributed content, shells and meshes.** Needed before the
   first public edit, not after. A shell measured by a contributor and a mesh
   donated by a third party are not the same licensing question.

## Risks

| Risk                                                      | Likelihood | Impact                                       | Mitigation                                                                                                                  |
| --------------------------------------------------------- | ---------- | -------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| Identification enables re-identification of an individual | Medium     | Severe, violates tenet 1                     | Computed publishable depth, mode-aware publishing, matched precision, no colour, aggregate-first framing                    |
| A quiet-street deployment publishes timestamped passes    | Medium     | Regular users identified regardless of class | Mode declared and enforced at the export boundary; the timestamp is the identifier there, not the label                     |
| City registration data is unaffordable                    | High       | Rarity floor loses its input                 | Degrade to provincial grain and hold identification at class rung                                                           |
| Scraped or copyrighted specs and CAD enter the corpus     | Medium     | Legal, and reputational                      | Per-field provenance, licence field, review gate before publication                                                         |
| Pre-1975 sales data is too weak to chart                  | High       | Domain 2 back-range shrinks                  | Confidence grading, visible on every chart                                                                                  |
| Encyclopedia becomes the project                          | Medium     | Measurement work stalls                      | Workstream A first, delivery gated behind v1.0                                                                              |
| A second repository is a second thing to maintain         | Medium     | Catalogue stalls, embedded subset goes stale | The subset is pinned by revision and keeps working; a stale catalogue degrades identification rather than breaking a sensor |
| Modelled figures read as measured                         | Medium     | Evidence claims weaken, tenet 3              | Distinct visual treatment, never mixed in one column                                                                        |

## Checklist

### Outstanding

- [ ] Workstream A: verify every candidate source for coverage, licence and cost (`M`)
- [ ] Workstream B: measure the rarity floor and specify the rung ladder (`M`)
- [ ] Workstream C: record schema with per-field provenance and confidence (`M`)
- [ ] Workstream C: embedded-subset contract, build-time revision pin, and offline behaviour when the catalogue is unreachable (`M`)
- [ ] Workstream C2: shell station format, stance transform, footprint estimator (`M`)
- [ ] Workstream C3: edition cut, patch and pin machinery, and edition citation on an identification (`M`)
- [ ] Workstream D: wiki editor and review queue over the existing revision model (`L`)
- [ ] Workstream E: shell and mesh pipeline, stance transform, sensor-accurate synthetic scan generation (`L`)
- [ ] Workstream F: descriptor bridge, match confidence calibration against reviewed masks (`L`)
- [ ] Workstream G: population explorer, facets, groupings, chart policy compliance (`M`)
- [ ] Workstream G2: per-entry fleet census in the PDF report, with disclosure controls (`M`)
- [ ] Workstream H: national and provincial baselines, grain-matching guard (`M`)
- [ ] Workstream I: design passes for record, explorer, and scene identification surfaces (`M`)
- [ ] Answer the six open questions above, at least 1, 2 and 4 before any code (`S`)

### Deferred

- [ ] Per-city model-level registration data, until a source exists that is both affordable and redistributable
- [ ] Any global or multi-market catalogue, until one market works
