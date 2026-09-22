# Scene vehicle identification and the web observation surface (v1.x)

- **Status:** Draft, open for investigation. No source verified, no code, no commitment
- **Layers:** L4 perception, L9 endpoints, Svelte frontend, public scene site
- **Target:** investigation spikes in v0.6.3; delivery from v1.0, matching work from v2.0
- **Companion plans:** [vehicle encyclopedia](vehicle-encyclopedia-plan.md), [web scene export](lidar-web-scene-export-plan.md)
- **Canonical:** [Vehicle encyclopedia](../platform/architecture/vehicle-encyclopedia.md) (single source of truth)
- **Related:** [shape descriptors](lidar-shape-descriptors-plan.md), [track labelling](lidar-track-labelling-auto-aware-tuning-plan.md), [point annotation and object dataset](lidar-point-annotation-and-object-dataset-plan.md) on branch `dd/lidar/annotation` <!-- link-ignore -->

## Motivation

A published scene currently shows boxes moving through a junction. A viewer
learns how fast they went. They do not learn what went past, and "what went
past" is where the safety argument lives: two tonnes at 30 km/h is a different
street from one tonne at 30 km/h, and a viewer can see that difference
immediately if the scene tells them which is which.

This plan covers the observation half of the encyclopedia work: turning a
tracked cluster into an identification with an honest confidence, saying
"unknown" convincingly when it is unknown, and giving the Go server a Svelte
surface where tracked vehicles, their point-cloud observations, and the
annotations drawn over them can be browsed in one place.

## Current state

| Fact                            | Value                                                                     | Source                                                     |
| ------------------------------- | ------------------------------------------------------------------------- | ---------------------------------------------------------- |
| Published scene carries         | id, position, velocity, speed, heading, L/W/H, box yaw, class, confidence | `internal/scene/types.go`                                  |
| Scene export format             | gzipped NDJSON chunks, 2 dp, timestamp-driven playback                    | [web scene export plan](lidar-web-scene-export-plan.md)    |
| Published scenes                | 27 site directories under `public_html/src/scenes/`                       | repository                                                 |
| Browser stack                   | plain ES modules, three.js, no bundler, no protobuf runtime               | `public_html/src/js/`                                      |
| Annotation truth                | immutable pack of point indices plus a revisioned sidecar                 | `internal/lidar/annotation` (branch `dd/lidar/annotation`) |
| Annotation states               | `proposed`, `reviewed`, `rejected`, with the proposing algorithm recorded | same                                                       |
| Annotation client               | macOS visualiser annotation pane                                          | `tools/visualiser-macos/.../Annotation`                    |
| Shape descriptors               | planned, not built: no eigenvalue or profile features exist today         | [shape descriptors plan](lidar-shape-descriptors-plan.md)  |
| Road-user classifier            | rule-based in L6, with a transparent improvement ladder planned           | [classifier plan](lidar-ml-classifier-training-plan.md)    |
| Catalogue shells and prototypes | specified, not built; Canadian market, year-marked editions               | [encyclopedia plan](vehicle-encyclopedia-plan.md)          |

Two things follow from that table. Identification has no feature vector to work
with until the shape-descriptor work lands, so this plan is sequenced behind it.
And the scene export needs no new fields, which is what keeps the privacy
argument in the companion plan intact.

## The boundary: the web reads, macOS grades

Settled on 2026-09-22, consistent with the recorded direction: the macOS
visualiser stays the one instrument for annotating, editing and grading, and the
web keeps its existing label CRUD without gaining functionality. The Svelte work
in this plan is a read-only dataset-identification surface, which is exactly what
that direction already permits.

So there is no browser annotation editor, no public proposal protocol, and no
second path by which a mask can be created. A browser displays masks that macOS
made; it never makes one.

The analysis of where annotation storage should live is kept below anyway, in
[Annotation file format](#annotation-file-format), because the read path needs a
transport form regardless and because the argument should be on the record if
the question is ever reopened.

## What an identification is

Not a name. A node in a tree, with a depth, a confidence, and a reason.

The tree is the [vehicle taxonomy](../lidar/architecture/vehicle-taxonomy.md),
and its levels are derived rather than authored: at each range, two catalogue
shells that the sensor cannot tell apart belong to the same class, so the
classes coarsen with distance and nest into a hierarchy. Level 0 is the shipped
seven-class label vocabulary and does not change.

| Depth | What it names                                               | Becomes possible with                 |
| ----- | ----------------------------------------------------------- | ------------------------------------- |
| 0     | Road user                                                   | Tracking alone                        |
| 1     | Gross form: low box, tall box, long box                     | Height, footprint, boxiness           |
| 2     | Silhouette: bonneted or cab-forward, sloped or upright rear | Dominant visible plane slopes         |
| 3     | Profile: hatch, saloon, estate, pickup, van, and size band  | Vertical mass profile, roofline break |
| 4     | Model family                                                | Landmark positions                    |
| 5     | Catalogue entry                                             | Fine geometry, ground clearance       |

### Resolved inside, published at a computed depth

The pipeline resolves as deep as the evidence allows. **How deep that answer is
published is computed per artefact, not fixed.**

$$
k^*_{\text{published}} = \min\big(k_{\text{resolved}},\; k_{\text{volume}},\; k_{\text{mode}}\big)
$$

$k_{\text{resolved}}$ is what the sensor supported. $k_{\text{volume}}$ is the
deepest node whose realised count inside that artefact reaches the anonymity
floor. $k_{\text{mode}}$ is what the deployment allows: a twenty-minute survey
on irregular days may publish per-pass classes, a continuously running sensor
may not publish a timestamped per-pass stream at any depth. The
[identifiability analysis](../platform/architecture/identifiability-analysis.md)
derives all three.

The practical consequences are less restrictive than a blanket rule and more
restrictive in the places that matter. A busy arterial with ten thousand passes
in the window can publish deeply, because eight other passes that hour looked
identical. A quiet residential street cannot publish much at all, and not
because of the class: fewer than two vehicles pass in any ten-minute slice, so
the timestamp identifies before the label does.

Geometry is quantised to the precision of the published depth, so a reader
cannot recover the entry from the numbers printed beside the class.

A viewer seeing "tall box, short bonnet, 2.1 to 2.4 tonnes" has learnt something
true and useful. A viewer seeing a specific model at 60 m, where the sensor
returned eleven points, has been lied to about the instrument. A viewer seeing
any per-pass label at 08:12 on a street with four vehicles an hour has been told
something about a neighbour.

Every identification records the catalogue edition it resolved against, because
one that cannot be reproduced later is not evidence.

Confidence falls with range by construction, because the synthetic scans the
match runs against are generated at the same range and get just as sparse. A
scene where distant vehicles visibly resolve to coarser classes is telling the
truth about its own sensor, and that is worth showing rather than hiding.

## Unknowns are a first-class result

Most of what passes a kerb will not resolve to rung 4, and the surface should be
built for that rather than treating it as failure. An unknown vehicle still has
measured geometry, measured speed, and therefore a mass estimate from the
class-and-dimension distribution and a kinetic energy with an error bar. It
contributes to every population statistic. It is drawn differently, not faintly:
a distinct treatment that reads as "measured, not identified", never as noise.

Three unknown kinds are worth separating, because they mean different things:
too few returns to try, enough returns but no match above threshold, and a
confident match to something not in the encyclopedia. The third is the
interesting one: it is a proposal queue entry for the wiki, generated by the
sensor. A recurring unmatched silhouette at one junction is a model somebody
should add.

## Naming a vehicle without a weight file

The road-user classifier is a small set of inspectable ranges over measured
features: no weight file, no opaque model, and a decision anyone can read. The
question is whether the same discipline survives a vehicle-type classifier with
thousands of possible answers.

**Not as a rule table, and it does not need to be.** A rule table is a
discrimination structure. It stops scaling somewhere around a couple of dozen
classes: the rules multiply, the boundaries stop being separable, and the
readability that justified the approach is the first thing lost. Writing one
over thousands of vehicles would produce exactly the unreadable artefact the
constraint exists to avoid.

The way out is that the premise carries a wrong assumption. Naming a model is
not a classification problem. It is **retrieval**, and retrieval over thousands
of items wants a metric and a table of prototypes, not a decision region per
class.

### Why the frame matters

A classifier over N classes learns N decision regions, and each one needs
training data. There will never be enough labelled point clouds per model: it
would take thousands of observations of each of thousands of vehicles, from
every angle and range, before a per-class boundary meant anything.

Retrieval needs no training data per class at all. It needs one _generated_
prototype per class, which the catalogue's shell supplies for free, and labelled
data only to calibrate a single shared distance function. That asymmetry is what
makes the problem tractable: the catalogue does the work that would otherwise
need a corpus.

### The structure

| Part                | What it is                                                                           | Size                                    |
| ------------------- | ------------------------------------------------------------------------------------ | --------------------------------------- |
| Feature extractor   | The existing L4 shape descriptors, ordinary inspectable code                         | Code                                    |
| Prototype table     | One row per catalogue entry, stance variant and range bin                            | A table of measurements                 |
| Distance function   | Per-dimension total uncertainty: sensor spread, within-model spread, prototype error | Around twenty variances                 |
| Rejection threshold | The distance beyond which the answer is "unknown"                                    | One number, from a chi-squared quantile |

A prototype table is not a weight file. Every row is a readable statement about
a real vehicle: at this range, this model presents this length, this height,
this roofline break, this footprint. Every decision is explainable in the same
terms: this matched because length and roofline agreed within tolerance, and the
runner-up lost on height by 180 mm. That is precisely the property the rule
table was chosen for, kept at a scale the rule table could not reach.

Order of magnitude, to be measured rather than trusted: a few thousand entries
across a handful of range bins, at a couple of dozen numbers each, is single-
digit megabytes as float, and well under that quantised to millimetres. The
embedded subset is provincial, so smaller again.

There are also **no free weights**, which is worth being precise about because
it is what keeps the twenty numbers from being a small weight file. A
per-dimension weight and a per-dimension tolerance are the same parameter
written twice. Written as an uncertainty it is a measurement with a unit and a
procedure: the sensor's spread on that dimension, plus how much that dimension
varies within one catalogue entry, plus how wrong the prototype is, combined in
quadrature. Those are three things somebody measures, not three things somebody
tunes. The [maths note](../../data/maths/proposals/20260922-vehicle-taxonomy-resolution-maths.md)
sets them out.

One consequence falls straight out and is worth checking early, because it is
counter-intuitive. Only the sensor term depends on range, so identification does
not keep improving as a vehicle approaches: it approaches a floor set by how
much a model varies within itself and how good its shell is. **There is a
maximum useful depth even at zero range, and it is a property of the catalogue
rather than the sensor.**

### The thousands dissolve, twice

**The rarity floor already collapsed the answer space.** The surface never has
to distinguish thousands of models, only those common enough to be published at
model level. Everything below `k` degrades to family or body class as a
condition of the privacy gate, not as a concession to the matcher. The number of
models clearing `k` in one province is in the hundreds at most.

That the privacy constraint and the tractability constraint land in the same
place is not a coincidence. Both are refusals to make a distinction the evidence
does not support: one because the population is too small to be anonymous, the
other because the sample is too small to be sure.

**The sensor sets the resolvable count, not the catalogue.** Two shells that
differ by less than the sensor's measurement error at a given range are not
distinguishable, and presenting them as separate answers would be a lie about
the instrument. So cluster the catalogue's own prototypes by descriptor distance
at each range bin: the resulting clusters _are_ the answer classes at that
range. There might be a handful of buckets at 40 m, a few dozen at 25 m, some
hundreds at 12 m. Those buckets are derived, measurable and publishable, and
they are the rung ladder computed rather than asserted.

The practical consequence is that a single decision is a nearest-neighbour
search over a few hundred rows, filtered by range and by the road-user class
already assigned. Nothing in the pipeline ever faces a thousand-way choice.

### Where machine learning belongs, if anywhere

The two-stage shape is right: establish the road-user class, then the vehicle.
Stage one has few classes and well-separated features, which is exactly where a
learned model buys least, so the existing rule-and-threshold approach and the
transparent ladder in the
[classifier plan](lidar-ml-classifier-training-plan.md) should stay. Stage one
also narrows stage two for free, because only vehicle prototypes are searched
for something already established to be a vehicle.

If a learned component is wanted, the honest place for it is **estimating the
per-dimension uncertainties** from the reviewed-mask exemplar corpus. That is
around twenty variances, each meaning "how far apart can two readings of this
dimension be before the difference means something". Calling that fitting rather
than measuring is mostly a matter of how the estimator is written: the output is
a table of physical spreads that fits in a document and can be checked against a
tape measure, not a set of coefficients that only make sense inside the model.

What should not happen is a learned embedding or a per-model head. Both are
opaque, both fail the repository's standing requirement that an algorithm stay
inspectable and explainable, and neither is necessary.

### Why this is explainable, precisely

Four properties, each one a consequence of the distance being a sum of
independent normalised squares rather than a design goal bolted on afterwards.
The [maths note](../../data/maths/proposals/20260922-vehicle-taxonomy-resolution-maths.md)
carries the derivations.

1. **Attribution is exact, not estimated.** Each dimension's share of the
   mismatch is its own term divided by the total, and the shares sum to one.
   Methods such as SHAP and LIME exist because a model's output cannot be
   decomposed this way and an approximation has to be built after the fact. Here
   the decomposition is the computation, so an explanation cannot drift from the
   decision it describes.
2. **The margin names the deciding dimension.** The gap between the best match
   and the runner-up decomposes the same way, which yields a checkable sentence:
   "it is this rather than that because the roofline break sits 0.4 m further
   forward, which is 3.1 standard deviations at this range and accounts for 78%
   of the margin."
3. **The threshold has units.** Under the uncertainty model the distance follows
   a chi-squared distribution, so the rejection point is stated as a
   false-rejection rate rather than a tuned constant. Better still, the gap
   between that theoretical value and the empirically calibrated one measures
   how wrong the uncertainty model is, and points at which term is off.
4. **The published depth is a minimum of two measured limits**: what the
   instrument resolved, and what the population will hide. Neither is a
   preference and neither can be relaxed by one.

### Rejection is the load-bearing parameter

With thousands of prototypes, something is always nearest. The rejection
threshold is the only thing standing between that fact and a confident wrong
answer, and it is the parameter that most needs measuring. Set it from the
descriptor-distance distribution of known-correct matches at each range, not
from a percentile of all matches, and expect it to be the single most-tuned
number in the system.

### What could break this

| Failure                                                       | Consequence                                                                                       | Response                                                                                                                        |
| ------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| Synthetic prototypes do not land where real observations land | The catalogue stops supplying prototypes for free, and corpus size becomes the binding constraint | The biggest single risk: measure it early, tracked as [Q29](../../data/QUESTIONS.md)                                            |
| Descriptors are not discriminative between similar vehicles   | The ladder tops out at a coarser rung than hoped                                                  | An honest result, not a failure: publish the resolvable-bucket count and stop there                                             |
| Occlusion truncates the observed shell                        | A partly hidden vehicle matches the wrong shorter prototype                                       | Compare only stations the sensor could see: needs a per-observation visibility mask, and a rejection when too little is visible |
| Stance variants multiply the table                            | Prototype count grows faster than the catalogue                                                   | Generated from one base shell by transform, so rows are cheap; prune variants that fall inside the sensor's resolution          |

Occlusion is the one that is easy to underestimate. At a junction, vehicles hide
each other constantly, and a truncated silhouette is a confident match to a
shorter vehicle unless the distance function knows which parts were observable.

## Svelte observation surface

New routes in the existing Svelte app, served by the Go binary.

| Route                   | Purpose                                                                          | Read or write |
| ----------------------- | -------------------------------------------------------------------------------- | ------------- |
| `/vehicles`             | Catalogue browser over the embedded subset: facets, groupings, population charts | Read          |
| `/vehicles/[model]`     | One record: specs, 3D model, derived metrics, provenance, link out to the editor | Read          |
| `/observations`         | Tracked objects across runs, with rung, confidence and energy                    | Read          |
| `/observations/[track]` | One track: point cloud through time, descriptors, candidates, masks              | Read          |
| `/scene/[id]`           | Existing route, gains an identification layer                                    | Read          |

Every route is read-only. Catalogue editing happens on the encyclopedia
project's own site, and mask editing happens in the macOS visualiser. A record
page links out to the former; it does not embed it.

`/observations/[track]` is the surface that does not exist anywhere today and is
the one most obviously missing: a tracked object's point cloud frame by frame,
its descriptor vector, and the ranked encyclopedia candidates with the distance
to each. It is a debugging tool, a review tool, and the page that explains an
identification to a sceptical reader, all from the same data.

## Annotation frame extracts

A pack holds every return of every frame in an excerpt. That is the right
artefact for truth and the wrong one to send to a browser. A **frame extract**
is the browser's view: one frame, or a short keyframe window, with the points, a
class flag per point, the masks that cover them, and the settled background in
force at that frame for context. It is a read artefact. Nothing sends one back.

The extract is a derived, regenerable artefact and should reuse the scene
export's shape rather than inventing a third encoding: gzipped NDJSON, two
decimal places, which the scene export work already measured as the best option
for exactly this payload and which needs no browser toolchain.

**Two index spaces, and the whole design turns on keeping them apart.**

| Space                | Owner             | Lifetime                   | What cites it             |
| -------------------- | ----------------- | -------------------------- | ------------------------- |
| Canonical pack index | Server, immutable | Permanent, digest-verified | Every stored mask         |
| Extract-local index  | The extract       | One extract                | The browser's working set |

The extract carries the mapping from its local indices back to canonical ones,
so a point a reader hovers can be named in canonical terms without the browser
ever holding authority over an index. An extract regenerated with a different
stride produces different local indices and touches no stored mask. The rule is
one line long and worth keeping even with no write path: a stored mask cites a
canonical index and nothing else. Store a local index once and every mask is
invalidated by the next export change.

## Annotation file format

### What exists is already close to ideal

The existing split is right, and the reasons are worth writing down so they are
not re-litigated:

- **Masks cite point indices, not boxes and not track ids.** This is the single
  most important property in the whole design. A mask stored as a bounding box
  or a track reference is invalidated by every retune, every re-cluster, every
  tracker change. A mask stored as point indices into an immutable pack is truth
  independent of the pipeline that produced the pack, which is exactly what
  truth has to be if it is going to score the pipeline.
- **The point domain is immutable and digested.** An index citation is
  verifiable rather than merely plausible.
- **Review state is tri-valued and carries the proposer.** An algorithm's
  proposal never silently becomes a human's judgement.
- **Concurrency is optimistic against both a revision and a digest of the loaded
  bytes**, and a conflict never discards the caller's dirty data.

Nothing below proposes changing any of that.

### What a format needs to gain

| Gap                                | Proposal                                                               | Why                                                                                                                           |
| ---------------------------------- | ---------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| Index sets are stored plainly      | Sort, delta-encode, run-length encode                                  | Object point indices are spatially clustered, so deltas are small and repetitive. Measure before adopting                     |
| No transport form for a browser    | Frame extract, NDJSON, local index space plus mapping                  | The pack is far too large to ship; the scene export already solved this shape                                                 |
| No read-only view of a pack        | Serve extracts from the existing annotation API                        | A reviewer on any machine should be able to see what macOS graded                                                             |
| Mask geometry can be recomputed    | Never store a derived box as truth, derive on read                     | Two representations of one fact drift                                                                                         |
| Model identity has nowhere to live | An optional identification on an object, distinct from its class label | A human naming a model is a different act from a human naming a class, with different confidence and different privacy weight |
| Provenance is per sidecar          | Per-mask author, session, tool, and algorithm version                  | Already partly present; it is what lets a read-only surface say who decided what                                              |

That fourth row is the one that connects this plan to the encyclopedia. A
reviewed mask with a named model is a labelled exemplar: a real point set, from
a real sensor, at a known range, for a known vehicle. It is the only way to
calibrate the synthetic-scan matcher against reality rather than against itself.

### Client-side or server-side

|                                          | Server-authoritative                   | Client-authoritative                              |
| ---------------------------------------- | -------------------------------------- | ------------------------------------------------- |
| Single truth                             | Yes                                    | No, every client is a fork                        |
| Revision history and rollback            | Yes, already built                     | Would have to be rebuilt in a browser             |
| Concurrent editors                       | Handled, with a defined conflict error | Undefined                                         |
| Readable by Go scoring and sweep tooling | Directly, today                        | Only after an import step                         |
| Works with no server                     | No                                     | Yes                                               |
| Responsive brush and lasso               | Needs a round trip per stroke          | Immediate                                         |
| Survives a cleared browser cache         | Yes                                    | No                                                |
| Untrusted contributor                    | Validated on arrival, lands `proposed` | Arrives as an opaque blob, still needs validating |

The honest reading is that client-side storage buys responsiveness and offline
work, and that neither of those requires client-side _authority_. The right
column never wins on its own terms.

That settles it twice over. Under the read-only boundary the browser stores
nothing at all, so the question is moot today. And if a write path is ever
reopened, the answer is already on the record: **server-authoritative,
client-buffered**. A browser would hold an in-memory edit session with its own
undo stack, which is a different thing from the sidecar's revision history and
should never be confused with it, and would submit the whole session as one
change carrying the revision and digest it loaded, against the same optimistic
save the macOS client already uses. The browser would get a fluid tool, the
server would keep the only truth, and an untrusted contribution would land as
`proposed`.

What that would cost is annotation with the server unreachable, which for a
contribution surface is not a real cost. What it is not worth is a second truth
store, which is why it is not being built now.

## Design passes

These are inputs to a design workstream, not its output.

**Scene with identification rail**

```text
+-------------------------------------------------+-----------------+
|                                                 | PASSING NOW     |
|          3D scene, tracks and points            |                 |
|                                                 | [icon] Estate   |
|              [box]     [box]                    | 1.4-1.8 t 31km/h|
|                    [box]                        | ~60 kJ  depth 3 |
|                                                 |-----------------|
|                                                 | [icon] Tall box,|
|                                                 |   short bonnet  |
|                                                 | 2.1-2.4 t 28km/h|
|                                                 | ~78 kJ  depth 2 |
+-------------------------------------------------+-----------------+
| [====timeline, class-depth density band====]                      |
+-------------------------------------------------------------------+
| 412 passes - 61% to depth 3 or better - median 74 kJ - vs 51 kJ   |
| for the Canadian fleet at these speeds                            |
+-------------------------------------------------------------------+
```

The rail is the surface: it is where a reader looks while the scene plays, so it
carries the class, the band and the energy rather than a name. Bands, not point
values, because the published precision matches the published depth. A vehicle
resolved only to depth 2 shows a wider mass band and a wider energy estimate,
which is the interface telling the truth about what the sensor got.

**Track observation**

```text
+----------------------+------------------------------------------+
| frames               |  points at frame 41, 22 m, 340 returns   |
| [##########|#######] |                                          |
|            ^41       |        (point cloud, orbit)              |
+----------------------+------------------------------------------+
| descriptor           |  candidates                              |
| linearity     0.31   |  1. Make Model 2019   d=0.08  rung 4 ok  |
| planarity     0.44   |  2. Make Other 2021   d=0.11             |
| sphericity    0.25   |  3. Make Third 2018   d=0.19             |
| height prof.  [/\_]  |  floor: 4,100 registered locally, passes |
+----------------------+------------------------------------------+
```

**Frame extract, read-only**

```text
+-----------+-----------------------------------------------------+
| objects   |  frame 41/120  [<] [>]      [bg] [fg] [ground]      |
| o1 car  R |                                                     |
| o2 car  P |            points, masked in colour                 |
| o3 ped  P |                                                     |
|           |                                                     |
+-----------+-----------------------------------------------------+
| o2: proposed by cluster-follow v3, not yet reviewed             |
| o1: reviewed by dd, 2026-09-14, rev 38      [open in macOS]     |
+-----------------------------------------------------------------+
```

Every mask says who decided it and whether a person has looked. The only action
is to open the pack in the macOS visualiser, which is where a mask can change.

## Out of scope: SLAM and parked vehicles

Both are explicitly excluded here and both are worth an investigation item
rather than silence.

**Parked vehicles.** A parked car is a background object by the time the
background model settles, so it is invisible to the foreground path that feeds
tracking. Identifying parked vehicles means matching against the settled
background snapshot instead, which is a different problem with different
economics: unlimited integration time, no motion, far denser returns, and a much
higher privacy weight, because a parked vehicle at an address overnight is a
far stronger identifier than one passing at speed. The privacy question is the
gate, not the engineering.

**SLAM.** The sensor is static by design, and the static-pose and spatial-priors
work is the current answer to "where is this capture". SLAM enters only with a
moving sensor, which is already deferred. Nothing here should acquire a
dependency on it.

Both are recorded as investigation items, not plans.

## Workstreams

| #   | Workstream                                                                       | Depends on                         | Effort |
| --- | -------------------------------------------------------------------------------- | ---------------------------------- | ------ |
| J   | ~~Confirm the web boundary~~: settled, the web reads                             | done                               | -      |
| K   | Taxonomy construction: uncertainty model, separations, linkage, nesting check    | shape descriptors, encyclopedia C2 | `L`    |
| K4  | Published visual field guide, print and offline                                  | K                                  | `M`    |
| K5  | Computed publishable depth per artefact, mode awareness, geometry quantisation   | K, B                               | `M`    |
| K6  | Deployment-mode declaration: survey versus continuous, and what each may publish | K5                                 | `M`    |
| K2  | Prototype table, distance function, rejection threshold calibration              | K, encyclopedia C2                 | `L`    |
| K3  | Occlusion-aware distance: per-observation visibility mask                        | K2                                 | `M`    |
| L   | Frame-extract read format, two index spaces, mapping contract                    | annotation pack                    | `M`    |
| M   | Index-set encoding measurement, then adopt or drop                               | L                                  | `S`    |
| N   | Svelte observation routes, all read-only                                         | scene API                          | `L`    |
| O   | ~~Browser annotation session~~: dropped by the read-only boundary                | -                                  | -      |
| P   | Model identity on a mask in macOS, and the exemplar corpus it creates            | annotation pack                    | `M`    |
| Q   | Scene identification layer and rail, with edition citation                       | K2, encyclopedia F                 | `M`    |
| R   | Design passes across all four surfaces                                           | wireframes                         | `M`    |

## Open questions

1. **What quantisation does each depth need?** Published precision has to match
   the published depth, or a reader recovers the entry from the geometry. The
   quantisation per depth is a number nobody has derived.
2. **Does a survey export need absolute times at all?** Playback needs relative
   timing, not wall-clock time. If absolute times can be dropped or coarsened
   without hurting the scene, the quiet-street constraint largely dissolves and
   surveys can publish deeper than the analysis currently allows.
3. **What is the minimum return count to attempt a match?** A number from
   measurement, not from taste, and probably a function of range rather than a
   constant. Tracked as [Q28](../../data/QUESTIONS.md).
4. **How many vehicle types are actually resolvable at each range?** The rung
   ladder should be derived from clustering the catalogue's own prototypes by
   descriptor distance at each range bin, not asserted. The answer also sizes
   the whole matcher. Tracked as [Q32](../../data/QUESTIONS.md).
5. **Can ride height be measured well enough to detect a modified vehicle?**
   If a matched vehicle's observed ride height can be compared against its base
   record, the sensor becomes the first thing that counts lifted and squatted
   vehicles in a local fleet. If it cannot, stance variants are catalogue
   bookkeeping with no observational payoff.
6. **How is an unmatched recurring silhouette turned into a catalogue
   proposal** without carrying observation data into the encyclopedia? Likely as
   a descriptor vector with no time and no place attached, and with a person
   filing it rather than a sensor.
7. **Which surface names a model on a mask?** The exemplar corpus needs a human
   to say "that point set is this model", and under the read-only boundary that
   has to happen in the macOS visualiser. It is a new field in an existing pane,
   not a new tool, but it is client work nobody has scheduled.

## Risks

| Risk                                                | Likelihood | Impact                                                           | Mitigation                                                                                                                                                                    |
| --------------------------------------------------- | ---------- | ---------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| A confident-looking identification is wrong         | High       | Credibility, and tenet 3                                         | Rungs, visible confidence, range-aware degradation, published match rates                                                                                                     |
| A read-only surface quietly grows a write path      | Medium     | Two review workflows, drift                                      | The boundary is recorded here and in the hub doc; the API serves extracts, it does not accept them                                                                            |
| Extract-local indices reach stored masks            | Low        | Every mask invalidated                                           | Mapping stays server-side; a test asserts that a stored mask cites canonical indices only                                                                                     |
| Identification lands before shape descriptors       | Medium     | Matcher built on nothing                                         | Sequence behind the descriptor plan, explicitly                                                                                                                               |
| Published geometry out-resolves its own class label | Medium     | The depth rule is undone by arithmetic                           | Quantise geometry per depth; test that a published scene cannot be matched back to an entry                                                                                   |
| A continuous deployment publishes a per-pass stream | Medium     | Regular users identified within days, at any depth               | Mode is declared and enforced at the export boundary, not left to a setting nobody reads                                                                                      |
| Synthetic prototypes do not match real observations | Medium     | The catalogue stops supplying prototypes free; corpus size binds | Measure before building the table; fall back to real exemplars and resize the ambition                                                                                        |
| Occlusion produces confident wrong matches          | High       | A hidden vehicle reads as a shorter one                          | Visibility mask in the distance function, and rejection when too little is observable                                                                                         |
| Parked-vehicle identification creeps in             | Medium     | Severe privacy exposure                                          | Named out of scope here, gated on a privacy decision                                                                                                                          |
| Labelling stays a one-person bottleneck             | High       | Exemplar corpus never reaches useful size                        | Accept it: the read-only boundary trades contribution volume for one trustworthy truth store, and the corpus target should be sized to what one reviewer can actually produce |

## Checklist

### Outstanding

- [ ] Workstream K: taxonomy construction, uncertainty model, linkage choice, and the nesting check (`L`)
- [ ] Workstream K4: published visual field guide, usable on paper and offline (`M`)
- [ ] Workstream K5: publishable depth computed from an artefact's own realised counts, with per-depth geometry quantisation (`M`)
- [ ] Workstream K6: deployment-mode declaration and the publishing rules each mode permits (`M`)
- [ ] Workstream K2: prototype table, distance function, and rejection-threshold calibration (`L`)
- [ ] Workstream K3: occlusion-aware distance with a per-observation visibility mask (`M`)
- [ ] Workstream L: frame-extract read format and index-space mapping contract (`M`)
- [ ] Workstream M: measure index-set encoding, adopt only if it pays (`S`)
- [ ] Workstream N: observation routes in Svelte, read-only throughout (`L`)
- [ ] Workstream P: model identity on a mask in the macOS annotation pane, exemplar corpus from reviewed masks (`M`)
- [ ] Workstream Q: scene identification layer and passing-now rail, citing the catalogue edition (`M`)
- [ ] Workstream R: design passes for scene, track, explorer and annotation surfaces (`M`)

### Deferred

- [ ] Browser annotation editing and public proposals: dropped by the read-only boundary. The storage argument is recorded above should it ever be reopened
- [ ] Parked-vehicle identification from background snapshots: gated on a privacy decision, not on engineering
- [ ] SLAM and any moving-sensor dependency: tracked by [motion capture architecture](lidar-motion-capture-architecture-plan.md)
