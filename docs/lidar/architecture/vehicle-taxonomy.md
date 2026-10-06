# Vehicle taxonomy: a tree the sensor can justify

- **Status:** Proposed. Open for investigation; nothing here is built
- **Layers:** L4 perception (descriptors), L6 objects (classification), L9 endpoints (export)
- **Maths:** [Resolution-limited taxonomy and explainable retrieval](../../../data/maths/proposals/20260922-vehicle-taxonomy-resolution-maths.md)
- **Plans:** [scene vehicle identification](../../plans/scene-vehicle-identification-plan.md), [vehicle encyclopedia](../../plans/vehicle-encyclopedia-plan.md)
- **Related:** [label vocabulary](label-vocabulary.md), [vehicle encyclopedia hub](../../platform/architecture/vehicle-encyclopedia.md)

A hierarchy of vehicle classes in which every level is a distinction the sensor
can actually make. It is built by measurement rather than authored, it gets
coarser with range because the returns do, and it is the vocabulary shared by
the matcher, the human labeller and the published scene.

---

## 1. The principle

A conventional vehicle taxonomy is a category scheme: somebody decides that
"crossover" is a kind, and vehicles are sorted into it. That is a marketing
distinction and a roadside LiDAR cannot see it.

This taxonomy inverts the order. Take the catalogue's shells, reduce them to
descriptors, and ask which pairs the instrument could tell apart at a given
range. The groups that answer "no" are a class. Do it at every range and the
classes nest, because tolerance grows monotonically with distance, and a nested
family of partitions is a tree.

Names come last, and they are an editorial act performed on a measured cluster.
The tree is not a set of ideas about vehicles. It is a statement about what a
Pandar40P on a pole can support, and it will change if the sensor changes.

### Why this matters beyond tidiness

**It makes the depth honest.** An identification published at model level from
eleven returns at 60 m is a fabrication dressed as a measurement. When the depth
is derived, that answer is structurally impossible rather than merely
discouraged.

**It gives labellers and the classifier one vocabulary.** A human labelling
tracks and a matcher naming them currently work in separate schemes that have to
be reconciled afterwards. Sharing the tree removes the reconciliation, and with
it a standing source of drift.

**It explains itself.** A ladder of distinctions, each with the feature that
makes it and the range at which it becomes possible, is a thing a member of the
public can follow. That is worth more than an accuracy figure.

## 2. Relationship to the shipped label vocabulary

The [label vocabulary](label-vocabulary.md) ships seven user-assignable classes
today, with `CAR` covering cars, vans and trucks. That vocabulary is **level 0
of this tree and does not change**.

| Concern                | Level 0                       | Levels 1 and below                        |
| ---------------------- | ----------------------------- | ----------------------------------------- |
| Defined by             | The proto3 `ObjectClass` enum | A versioned taxonomy artefact             |
| Changes                | Rarely, with a proto revision | Per catalogue edition                     |
| Assignable by a person | Yes, today                    | Yes, through the visual guide             |
| Stability guarantee    | Wire-format stable            | Nodes may split or merge between editions |

Nothing in this document proposes a proto change, a migration, or a new enum
value. The deeper levels live in a separate field carrying a node identifier and
the edition that defined it. The display-versus-selectable label split already
planned for truck and motorcyclist applies unchanged.

## 3. The feature-availability ladder

What the sensor can distinguish is governed by which descriptor dimensions
survive at a given range and return count. The ladder below is the expected
shape. **The range column is an expectation to be measured, not a result.**

| Level | Name       | Survives on                                | Distinguishes                                     | Expected range  |
| ----- | ---------- | ------------------------------------------ | ------------------------------------------------- | --------------- |
| 0     | Road user  | Gross extent, maximum height, speed        | Person, cycle, vehicle, bus                       | Any             |
| 1     | Gross form | Height, ground footprint, boxiness         | Low-box, tall-box, long-box                       | Far             |
| 2     | Silhouette | Plus dominant visible plane slopes         | Bonneted or cab-forward; sloped or upright rear   | Mid to far      |
| 3     | Profile    | Plus vertical mass profile, roofline break | Hatch, saloon, estate, pickup, van, and size band | Mid             |
| 4     | Family     | Plus landmark positions                    | Model family                                      | Near            |
| 5     | Entry      | Plus fine geometry, ground clearance       | Model, year range, trim, stance                   | Close and dense |

Three of those features deserve a definition, because they are the ones that
carry the coarse levels and they are exactly what a sparse return pattern still
contains.

**Height.** The height above the ground plane of the topmost return. The single
most robust feature there is: one number, derived from the extreme of the
distribution rather than its shape, and the last thing to survive as returns
thin out.

**Boxiness.** The occupied fraction of the oriented bounding volume. The
occupancy column lattice already specified for selection work gives this
directly: half-metre columns of eight cubic voxels, one byte each, so boxiness
is a population count over the occupied voxels divided by the volume they could
have filled. A van approaches one; a saloon with a sloped bonnet and tapered
tail is much lower. Cheap, inspectable, and computed from a structure that
exists for other reasons.

**Plane slope.** At range the sensor sees few surfaces, and their orientation is
the signal that remains. A windscreen rake, a bonnet slope, a near-vertical box
side: each is a dominant normal direction estimable from a handful of returns.
This is what lets level 2 separate a bonneted car from a cab-forward van long
before any landmark resolves.

## 4. How the tree is built

Five steps. The mathematics of each is in the
[maths note](../../../data/maths/proposals/20260922-vehicle-taxonomy-resolution-maths.md);
this is the procedure.

**1. Generate prototypes.** For every approved catalogue entry and stance
variant, ray-cast the shell from a sensor pose with the real beam geometry and
compute the descriptor. Noise-free: a prototype is the ideal descriptor, and
uncertainty is applied later rather than baked in.

**2. Measure the uncertainty model.** Three separately measured quantities per
descriptor dimension: sensor measurement spread, binned by range and return
count, taken from repeated observations of one vehicle in reviewed masks; the
spread of that dimension across real examples of one catalogue entry; and the
residual between a prototype and dense observations of the vehicle it describes.
They combine in quadrature. None of the three is assumed.

**3. Compute separations.** For every pair of prototypes, the distance in
uncertainty-normalised coordinates at each range bin. A pair separated by less
than the rejection threshold is not distinguishable at that range.

**4. Agglomerate.** Complete-linkage hierarchical clustering. Complete linkage
because the required semantics is "every pair inside this class is
indistinguishable", which single linkage does not provide: it chains, producing
classes whose members can plainly be told apart.

**5. Cut per range, and verify nesting.** Cut at each range bin to obtain that
bin's classes. Pairs only ever merge as range grows, never separate, so the
relation nests. Whether the _partitions_ nest depends on degradation being
uniform across dimensions, which is an empirical question. Verify it. Where it
fails, build top-down instead, subdividing only within a parent, which
guarantees the tree at the cost of occasionally splitting a group that belonged
together.

### The resolution floor

Only sensor measurement spread depends on range. The other two contributions do
not, so identification does not keep improving as a vehicle approaches: it
approaches a floor set by how much a model varies within itself and how good its
shell is.

The consequence is worth stating plainly, because it is counter-intuitive and it
is checkable. **The tree has a maximum useful depth even at zero range, and that
depth is a property of the catalogue rather than the sensor.** Past that point,
a better sensor buys nothing without better shells.

## 5. Naming

Clusters are measured; names are written by a person looking at what fell out.
Four rules keep the names honest.

- A name describes what the sensor sees, not what a manufacturer calls it. "Tall
  box, short bonnet" is a name. "Crossover" is a market segment.
- A name at one level must read as a refinement of its parent. If it does not,
  either the tree or the name is wrong.
- A cluster that resists naming is a finding, not a naming problem. It means the
  descriptor grouped things for a reason nobody can see, which is worth
  investigating before it is papered over.
- Names belong to an edition. Node identity is by membership, so a node renamed
  between editions is still traceable, and a node whose membership changed is
  visibly a different node even if the name persists.

### The class code

Alongside its name, every node carries a short fixed-width code in which **every
position is something the sensor measures**.

| Position | Encodes                         | Example values                                                 |
| -------- | ------------------------------- | -------------------------------------------------------------- |
| 1        | Size band from ground footprint | 1 to 6                                                         |
| 2        | Body form                       | H hatch, S saloon, W estate, U utility, P pickup, V van, B bus |
| 3        | Height and bonnet band          | L low, M mid, H high                                           |

`4UH` is a large utility with a high bonnet. **An unresolved position is a
wildcard**, so the same vehicle reads `?U?` at sixty metres and `4UH` at twelve,
and the code displays its own resolution without a separate explanation.

Rental-industry category codes are the right instinct and the wrong vocabulary
to adopt directly: their categories are commercial rather than physical, half
their positions encode transmission and air conditioning that no LiDAR will see,
and they are somebody's standard, so adoption is a licensing question before it
is a design one. Borrow the shape, not the standard.

Publish crosswalks to the vocabularies people know: rental categories, European
segments, and the federal class definitions. Mark them approximate. A class
derived from what a sensor resolves will not align exactly with one derived from
interior volume or from price, and forcing the alignment would corrupt a
measured tree to fit a commercial one.

## 6. What the published export carries

**Internally the pipeline may resolve as deep as the tree allows. What is
published depends on the artefact, the site and the deployment mode**, and is
computed rather than configured. The reasoning is in the
[identifiability analysis](../../platform/architecture/identifiability-analysis.md);
the operational summary is here.

### Retention tiers

| Tier             | Holds                                                         | Lifetime                        | Leaves the device |
| ---------------- | ------------------------------------------------------------- | ------------------------------- | ----------------- |
| Transient        | The resolved entry for one track                              | The track's lifetime, in memory | Never             |
| Aggregate        | Counts by node and entry, per site per period                 | Persistent                      | As an aggregate   |
| Published scene  | Node identifier at publishable depth, plus quantised geometry | In the scene export             | Yes               |
| Published report | Counts by entry over a period, small cells suppressed         | In the report                   | Yes               |
| Review           | Point set and identity inside an annotation pack              | Operator-controlled             | Never             |

Resolve, aggregate, discard. A fleet mass distribution needs a counter per
entry, not a row per vehicle, so no per-observation entry identity is persisted
by default. An operator may retain more for review, explicitly and locally.

### Publishable depth

$$
k^*_{\text{published}} = \min\big(k_{\text{resolved}},\; k_{\text{volume}},\; k_{\text{mode}}\big)
$$

$k_{\text{resolved}}$ is what the sensor supported for that observation.
$k_{\text{volume}}$ is the deepest node whose realised count in the published
artefact reaches the anonymity floor, computed from the artefact's own contents
rather than from a site average. $k_{\text{mode}}$ is what the deployment
permits: a survey of twenty minutes on irregular days may publish a per-pass
class, while a continuously running sensor may not publish a timestamped
per-pass stream at any depth, because repetition collapses anonymity faster than
a coarser class restores it.

Depth is therefore a property of each published artefact, and a reader can check
it against the artefact's own counts.

### A counted period is not a labelled pass

A per-entry breakdown over a reporting period is a fleet census: nobody is in
it, there is no time to link on, and no pass to point at. It may go deeper than
a scene, subject to suppressing small cells, suppressing their complements, and
watching a cell's time series rather than only its value.

A model name against a moving box at a stated minute is an event, and events
link. That is the distinction that governs, not the label itself.

### Precision must match the label

An export never carries numbers that out-resolve its own class. Publishing
"4.87 m long, 1.91 m wide, 1.74 m tall" beside a body-form class hands the
reader the entry back, and a rule a determined reader can undo with arithmetic
is not a rule. Geometry is quantised to the precision of the node it is
labelled with, which makes the constraint a property of the artefact rather
than a display convention a later feature could undo.

### What is owed in exchange

Where a scene publishes a class rather than an entry, the reader loses the
ability to audit that one identification. The accountability moves to the
method: publish the tree, the uncertainty model, the computed depth with its
inputs, worked per-dimension attributions, and aggregate match rates by range.
The reasoning is auditable even though individual answers are not disclosed.

## 7. The visual guide

The taxonomy is published as a field guide, for labellers and for readers. One
page per node, and the same page serves both audiences.

| Element                                            | For the labeller           | For the reader                          |
| -------------------------------------------------- | -------------------------- | --------------------------------------- |
| Silhouette composite of member shells              | What this class looks like | What the machine means by the word      |
| The discriminating question                        | What to look at next       | Why the distinction exists              |
| The feature that answers it, and its tolerance     | How sure to be             | That the distinction is measured        |
| The range at which the question becomes answerable | Whether to attempt it      | Why distant vehicles get vaguer answers |
| Two members and one near-miss from a sibling       | Where the boundary runs    | That boundaries are real and narrow     |

Two rules for the labeller, printed on the guide: when unsure, go **up** a
level, never guess down; and an answer at a coarse level is correct, not a
failure to try.

The guide must work on paper and in the offline docs site. Somebody labelling at
a kerbside with no connection is a real case, and a field guide that needs a
network is not a field guide.

## 8. Open questions

1. Does descriptor measurement error degrade uniformly across dimensions with
   range? If yes, one dendrogram serves every range and the cut height alone
   selects depth. If no, the tree must be enforced top-down.
2. How many classes are resolvable at each range? The answer sizes the matcher
   and sets the ladder's real shape. Tracked as Q32 in
   [QUESTIONS.md](../../../data/QUESTIONS.md).
3. Where is the resolution floor, and therefore the maximum useful depth?
4. What quantisation does each level's geometry need so that published numbers
   cannot out-resolve the published label?
5. Do human labellers using the guide agree with each other, and with the
   matcher? Inter-labeller agreement on the guide's own questions is the test of
   whether the tree is usable by people as well as by code.
