# Vehicle encyclopedia: concept and privacy contract

- **Status:** Proposed. Open for investigation; nothing here is built or committed to
- **Layers:** Separate open project; velocity.report consumes an embedded subset
- **Decided 2026-09-22:** Canadian market, separate project, year-marked annual editions, read-only web here, direct vision as a sourced domain
- **Plans:** [vehicle encyclopedia](../../plans/vehicle-encyclopedia-plan.md), [scene vehicle identification](../../plans/scene-vehicle-identification-plan.md)
- **Taxonomy:** [vehicle taxonomy](../../lidar/architecture/vehicle-taxonomy.md) owns the class tree and the visual guide

A public, wiki-editable catalogue of vehicle makes and models, carrying the
properties that turn a measured speed into a statement about harm: mass,
geometry, direct vision and blind zones, power, braking, published safety
results, and how many of each were sold and registered. This document is the single source of truth
for what the encyclopedia is, where its boundary with observation data runs, and
the privacy contract that governs the join. The two plans own delivery.

---

## Why it belongs to this project

The system measures speed. Speed alone has been published for fifty years. Mass
and front-end geometry are the other half of the harm equation and nothing in
the sensor can supply them, so they have to come from a catalogue. Paired, they
yield kinetic energy and blind-zone length for the traffic that actually passed
a particular kerb, which is a different and much stronger argument than a
percentile.

## Market and editions

The reference market is **Canada**. Units are metric, vehicle classes follow the
federal safety standards, the regional grain is the province or territory, the
insurance picture is provincial rather than one national market, and federal
source data arrives in both official languages. A record keeps a `market` field
anyway, because a nameplate sold here is frequently not the vehicle tested
elsewhere.

The catalogue publishes **year-marked editions on an annual cadence**: one
frozen, citable artefact per year. A published edition never changes; a
correction is a new patch, and new model years arrive in the next edition. A
consumer pins exactly one edition, and an identification records the edition it
resolved against.

That last rule is what makes an identification evidence rather than an opinion.
Tenet 3 requires a claim to be reproducible, and a scene published against one
edition must still resolve the same way years later. Without editions, every
catalogue correction silently rewrites the history of every published scene.

## Where it lives

The encyclopedia is its own repository and its own service, on the pattern the
spatial-priors work sets. velocity.report consumes a small versioned subset,
embedded in the binary and usable with no network; the full catalogue and its
public editor live online.

Tenet 4 requires the Pi to work offline, which a public wiki cannot promise.
Tenet 7 requires a release to be one versioned unit, which an embedded data
subset can be and a live catalogue cannot. And tenet 1 is easier to hold when
the public write path is not inside the binary holding the measurements. What
crosses the boundary is a data artefact pinned by revision at build time, never
a runtime dependency: if the online catalogue vanishes, every deployed sensor
keeps working with what it shipped with.

## Two artefacts, one join

| Artefact         | Contains                                            | Published as          |
| ---------------- | --------------------------------------------------- | --------------------- |
| The encyclopedia | Facts about manufactured products                   | Public reference data |
| A scene          | Geometry, speed, heading, class, per tracked object | Static derived export |

They are separate artefacts and they meet only at render time. A scene gains no
make or model field. An encyclopedia entry gains no observation column. The
identification a reader sees is computed from published geometry against the
published catalogue, so it is derived, reproducible, and carries no stored link
between a vehicle and a place.

## The privacy contract

Tenet 1 is absolute. What it forbids is a published artefact that singles
somebody out, and whether a given artefact does that is a number rather than a
judgement: the size of the anonymity set it leaves. The
[identifiability analysis](identifiability-analysis.md) derives it, and the
contract below is what falls out.

1. **Publishable depth is computed, not configured.** The class published
   against a pass is the shallowest of what the sensor resolved, what the
   realised counts in that artefact support, and what the deployment mode
   allows. On a busy road that can reach an entry; on a quiet one it stops at a
   body form. A depth nobody could have derived from the artefact's own contents
   is not published.
2. **Published precision matches the published class.** An export never carries
   numbers that out-resolve its own label, because a reader with the catalogue
   and arithmetic could otherwise recover the entry. Geometry is quantised to
   the precision of the class it is published with, which makes the rule a
   property of the artefact rather than a display convention.
3. **A counted period is not a labelled pass.** A per-model breakdown over a
   reporting period is a fleet census with nobody in it, and may go deeper than
   a scene, subject to small-cell suppression. A model name attached to a moving
   box at a stated minute is an event, and events link.
4. **Continuous deployments publish aggregates, not streams.** Repetition
   collapses anonymity geometrically while coarser classes buy back only
   logarithmically, so an always-on sensor cannot publish a timestamped per-pass
   stream at any granularity. A survey of twenty minutes on irregular days can.
5. **No colour, ever.** LiDAR returns intensity, not colour. Colour is the
   strongest re-identifier after the plate, the sensor cannot see it, and the
   system will not infer it.
6. **One-way by construction.** No page answers "where has this model been
   seen".

Resolve, aggregate, discard remains the internal default: the pipeline may
resolve as far as the evidence allows on the device, increments an aggregate
counter, and persists no per-observation entry identity unless an operator
explicitly retains it locally for review.

Where a scene publishes classes rather than entries, the reader loses the
ability to audit one identification. The accountability moves to the method: the
taxonomy, the uncertainty model, worked per-dimension attributions, the computed
depth and its inputs, and aggregate match rates by range are all published, so
the reasoning is auditable even though individual answers are not disclosed.

## The class tree

An identification is a node in a tree, with a depth, a confidence and a reason,
never a bare name. The [vehicle taxonomy](../../lidar/architecture/vehicle-taxonomy.md)
is canonical for the tree, how it is built and how it is published as a field
guide. In short: at each range, two catalogue shells the sensor cannot tell
apart are the same class, so classes coarsen with distance and nest into a
hierarchy. Level 0 is the shipped seven-class label vocabulary and does not
change.

The publishable depth is the shallower of what the sensor resolved and what the
rarity floor allows, and the surface shows which depth it reached. Confidence falls with
range by construction, because a match runs against synthetic scans generated at
the same range and sparsity. A scene where distant vehicles visibly resolve to
coarser rungs is telling the truth about its own sensor.

## The shell

An approved entry carries an **outer shell**: transverse stations along the
vehicle, each with a top height, shoulder height, width and underbody clearance,
plus landmarks a station grid would blur and the axle positions everything else
is measured against. A few hundred bytes a vehicle.

The shell is the geometric core because it is contributable by a person with a
tape measure, inspectable as a table of checkable measurements, transformable by
a rigid shift of body stations against axle stations, and sufficient for
footprint, bonnet height, blind zone and matching. A detailed mesh is for
rendering and for close-range work, and is derived or donated rather than
required.

**Stance** rides on the shell as four numbers against a base record: suspension
lift front and rear, and fitted tyre diameter front and rear. Negatives cover
lowered and squatted vehicles. This matters because the sensor sees the modified
vehicle rather than the brochure, because a lift raises the bonnet height the
whole safety argument rests on, and because nothing currently counts how much of
a fleet is modified.

## The point-cloud bridge

Ray-casting a shell from a real sensor pose, with the real beam geometry and the
real range, produces the sparse returns the sensor would have seen. Those
synthetic returns go through the same L4 shape-descriptor code as an observed
cluster, so matching is a nearest-neighbour search in one shared space rather
than a bespoke comparison.

Naming a vehicle is **retrieval, not classification**: a table of generated
prototypes and one calibrated distance function, rather than a decision region
per model. A prototype table is not a weight file, because every row is a
readable statement about a real vehicle and every match is explainable in the
same terms. The number of answers is bounded twice over, by the rarity floor and
by what the sensor can resolve at a given range, so no decision is ever a
thousand-way choice. The [identification plan](../../plans/scene-vehicle-identification-plan.md)
carries the argument.

Reviewed annotation masks supply the calibration set: a reviewed mask with a
named model is a real point set, from a real sensor, at a known range, for a
known vehicle. It is the only way to calibrate the matcher against reality
rather than against itself.

## Review model

The encyclopedia uses the three states the annotation sidecar already defines:
`proposed`, `approved` and `rejected`. One review vocabulary across the project,
not two. Only an approved record gains a mesh and enters the identification set.
An algorithm's proposal never silently becomes a human's judgement, and neither
does a stranger's edit.

Public editing applies to the catalogue, and only to the catalogue. Point-level
annotation of sensor data stays in the macOS visualiser: the velocity.report web
surfaces read masks and identifications, and never write them. That boundary is
what keeps one review workflow rather than two truth stores.

## Boundaries

| Out of scope here                   | Where it lives                                                          |
| ----------------------------------- | ----------------------------------------------------------------------- |
| Parked-vehicle identification       | Deferred, gated on a privacy decision, not on engineering               |
| Moving-sensor SLAM                  | [motion capture](../../plans/lidar-motion-capture-architecture-plan.md) |
| Browser annotation editing          | Dropped: the web reads, macOS grades                                    |
| Registration records of individuals | Never in scope, at any grain, under any licence                         |
| Vehicle colour                      | Never in scope: the sensor cannot see it                                |
