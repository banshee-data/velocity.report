# Lane review across the city: the street segment as the unit, one Approve, and a queue on the grid plan

How the travel-lane review in the macOS visualiser grows from one site and one proposal at a time
into a review of the city's streets: Gerardus derives and stores a lane per city street segment and
direction rather than per site, an approval is one decision Gerardus acts on and velocity.report
pulls, the city view colours each segment by what is left to review, each proposal says what its
grade rests on and what would raise it, stop lines are proposed from where traffic stops, and
direction names are derived from the street rather than typed per site. The grid is the City's own:
street segments keyed by CNN inside S2 cells. OpenStreetMap stays an optional backdrop and an export
target, never a key or a source.

- **Status:** Draft (D-32: not counted as active; the owner places it in a release slot). Written
  on November 7, 2026 from an evaluation of the painter as built after the v0.7.19 lane work and the
  batch lane job
- **Decisions:** relies on D-21 (visualiser overlay controls), D-32, D-33 (hub docs state what
  ships), D-36 (Gerardus owns static street geometry and accepted registration), D-38 (geometry is
  measured spatial data), D-39 (proposals in Gerardus, a snapshot here); Gerardus
  [ADR-0006](https://github.com/banshee-data/gerardus/blob/main/docs/adr/0006-boundary-with-velocity-report.md)
  (velocity.report never reports through Gerardus),
  [ADR-0008](https://github.com/banshee-data/gerardus/blob/main/docs/adr/0008-public-domain-first-osm-for-vectors.md)
  and [ADR-0009](https://github.com/banshee-data/gerardus/blob/main/docs/adr/0009-city-streets-by-default.md)
  (public domain first; city streets by default; nothing from OSM in a public-domain layer),
  [ADR-0011](https://github.com/banshee-data/gerardus/blob/main/docs/adr/0011-publish-snapshots-and-aggregates-not-trails.md)
  and [ADR-0016](https://github.com/banshee-data/gerardus/blob/main/docs/adr/0016-lane-fields-proposals-and-limits.md).
  Proposes one amendment to D-39's practice, not its locus: the snapshot is pulled, not sent (Item 2)
- **Layers:** L10 macOS visualiser (paint mode, city mode); L9 endpoints (`lanes pull` on a
  schedule); the web clips view; Gerardus (derivation keyed by street segment, server-side
  approval, a stop-line derivation, derived direction names)
- **Target:** unscheduled
- **Companion plans:** [lane painter](lidar-visualiser-lane-painter-plan.md) (the pane this
  changes), [lane fields across the corpus](lidar-lane-field-batch-plan.md) (the job that fills the
  queue, and the first Lanes to review list), [lane snapshot and attribution](lidar-lane-snapshot-and-attribution-plan.md)
  (what Item 2 pulls), [city view](lidar-city-view-plan.md) (where the queue is drawn),
  [lane stitching](lidar-lane-stitching-plan.md) (successors between segments), and Gerardus
  [LANE-PROPOSALS](https://github.com/banshee-data/gerardus/blob/main/docs/plans/LANE-PROPOSALS.md),
  [LANES-AND-LIMITS](https://github.com/banshee-data/gerardus/blob/main/docs/plans/LANES-AND-LIMITS.md),
  [CITY-STREETS](https://github.com/banshee-data/gerardus/blob/main/docs/plans/CITY-STREETS.md) and
  [INTERSECTION-MODE](https://github.com/banshee-data/gerardus/blob/main/docs/plans/INTERSECTION-MODE.md)
  (the OSM export)
- **Canonical:** [Gerardus alignment pane](../ui/visualiser/gerardus-alignment.md) and
  [city view](../ui/city-view.md), updated at ship (D-33)

A proposal is an inference from where tracked objects were, counted per cell; an approved lane is
the operator's reading of the street; a stop-line proposal is where traffic was observed to stop.
None of it is a survey or a specification. See
[Geometry priors: data purpose](../geometry-priors-data-purpose.md).

## Motivation

The lane work delivered in v0.7.19 and the batch job after it do what they set out to: a run's
tracks become a field, Gerardus proposes lanes from it, the painter shows them and the operator
approves, edits or rejects each one. Reviewing Ashbury at Downey on November 6, 2026 showed where
that stops scaling:

- **A lane arrives in pieces.** Gerardus derives a proposal per city street segment and direction
  inside one site's bundle, so one lane through an intersection is two proposals meeting at the
  node, and a lane that drifts against the City's centreline through a bend split in two until
  Gerardus's chaining was changed. The painter's Join tool stitches the pieces back by hand, after
  the fact.
- **One lane, one site.** A proposal belongs to the site (L16 bundle) whose fields it was derived
  from. Two sensors covering one block, or one sensor placed twice, make two sets of proposals and
  two sets of marks for the same lane.
- **Approval is three steps in two systems.** Approve makes a mark, Save writes it to Gerardus, Send
  copies the lanes to velocity.report, and a standing notice says the reports are stale until the
  third step. The owner asked why velocity.report does not read the accepted lanes itself.
- **The grade is opaque.** A proposal's `tracks` is basin-integrated evidence, not a count of
  distinct tracks, and the pane does not say what would raise `medium` to `high`. The first Ashbury
  result was misread for that reason.
- **Stop lines have no home.** Gerardus accepts driveways, spaces, travel lanes and surfaces; a stop
  line exists only as paint traced from the aerial intensity (W5). A lane ends at the City's
  intersection node, which is not where the traffic stops.
- **Work is found one site at a time.** The city mode's Lanes to review list (batch plan Item 6)
  counts proposals per site, but a segment's state is not drawn on the city, and the clips view says
  whether a lane job ran, not whether anyone has reviewed its result.

The grid this has to work on is already there: the City's street segments (CNN) and intersections
from DataSF under PDDL, S2 cells for the index (ADR-0003), and the city tiles that draw them at
three levels of detail. It owes nothing to OpenStreetMap, and must stay compatible with it on the
way out: tags that map onto OSM's, GeoJSON export, JOSM-reviewable change files, and an optional
reference to an OSM way id.

## Current state

- **Paint mode** ([hub doc](../ui/visualiser/gerardus-alignment.md#paint-mode-the-street-painter)):
  the plan view over the placed reference, the Traffic section with the run's, the site's and a
  live field, the Proposals section with Approve, Approve all high, Reject and Reopen, the tools
  (surface, lane, space, row, driveway, Split at sign, Join lanes, select), the limit row with the
  statutory prefill, Save as one `EditStreetUse`, and Send lanes to velocity.report after a save.
- **Outstanding in the painter plan:** Edit (approve and start a vertex drag), the cell hover
  inspector, the `T P A R S` keys, the crossing markers, the `oneway` warning, the City prefill and
  the split offered at a sign pole.
- **Gerardus** derives per street segment and direction from the site's summed fields
  (LANE-PROPOSALS §4, as built §11), keeps proposal ids across derivations, tags a proposal adopted
  when an authored lane runs along it, and records rejections in an append-only review table. A
  derivation is keyed by the site bundle, the placed fields and the carriageways. Derivation 2
  (banshee-data/gerardus#76) follows a lane that drifts against its centreline.
- **velocity.report** keeps an append-only lane snapshot per site, written by the painter's Send
  and by `velocity lidar lanes pull`, which still takes `--record-id` and `--pose` by hand because
  Gerardus serves no accepted record over HTTP. Whether `lanes pull` runs on a schedule is an open
  question of the snapshot plan.
- **City mode** draws the city tiles with a marker per placed site and lists the sites with
  proposals outstanding; choosing a row opens the site in paint mode. The web site viewer draws the
  field and the proposals read-only; the clips view shows Registered and Lanes per clip with a
  re-run button.
- **The lane field** carries `tracks`, `box_tracks`, eight heading bins and `speed_sum` per 0.25 m
  cell. Nothing reads `speed_sum` yet.

## Design

### The street segment is the unit (Gerardus)

A travel lane belongs to a city street segment and a direction, not to a site.

- **Derivation per segment.** Gerardus derives the proposals of a segment from every placed field
  that reaches it, across every site whose bundle contains the segment, and chains along the street
  through its intersection nodes where the lane continues (same street name, the ends within the
  chain's gap, the turn under the join rule's 45°). A proposal is keyed `(cnn, bound, index across
the street)`, stored once, and served to every site containing the segment. The lateral profiles
  stay per station, so the painter's evidence drawing is unchanged.
- **Where the chain ends.** At the intersection node the lane enters, as today, or at a confirmed
  stop line (below) when the site has one, or where the evidence ends.
- **One mark per lane.** An authored lane is stored against the segment too, so a second site that
  sees the same block reads the same mark and the same adoption. The painter shows which sites'
  fields a proposal rests on, so an operator at one site knows another's traffic is in it.
- **What this does to Join.** Most joins disappear, because a lane through a node arrives as one
  proposal. The tool stays for lanes the operator drew in pieces.

### One Approve, and velocity.report pulls (Gerardus, server, painter)

- `ReviewLaneProposal` gains the decision `APPROVED`. Gerardus answers it by authoring the travel
  lane itself: a new `TRAVEL_LANE` mark with the proposal's vertices, `lane_type`, `width_m`,
  `proposal` and `confidence`, the reviewer as author, and the limit the painter sends with the
  decision (the prefilled statutory default, or what the operator set). The mark is snapped, draped
  and checked as any mark, and comes back in the answer selected, so the operator edits it as today.
  Approve all high is one request with several decisions, applied whole or not at all.
- **Send lanes to velocity.report is removed.** velocity.report's `lanes pull` reads the accepted
  record and pose from Gerardus (Gerardus serves it over HTTP; the snapshot plan's Item 1
  remainder) and runs on the server on a schedule, and at once when Gerardus reports a change
  (`GetServerInfo` carries a street-use version per site; the server polls it). The snapshot stays
  velocity.report's append-only copy with provenance (D-39, ADR-0006): what changes is who copies.
- The standing notice goes with the button. The pane shows instead when the snapshot last caught up
  with Gerardus, from `GET /api/lidar/sites/{l16}/lanes`.

### The queue on the grid (city mode, clips view)

- **Segments coloured by state** in city mode, over the city tiles: to review (a proposal neither
  adopted nor rejected), adopted, rejected, stale (a field newer than the derivation that made the
  proposal, or a derivation older than Gerardus's current `DERIVATION`), and no evidence (a placed
  site whose fields reach the segment but propose nothing). The colours are the light-and-dark-safe
  set the painter's directions use, with state by line style, so a colour-blind reader has two cues.
  Hovering a segment names the street, the bound, the grade and the sites whose fields it rests on.
- **Click opens the segment**, not the site: the site that contains it (the nearest placed one when
  several do) opens in paint mode with the segment's proposals selected and the view centred on
  them, as the Lanes to review list does for a site today.
- **The list stays**, and gains a row per segment under each site, sorted by outstanding first,
  with filters for grade and state. The counts come from one `ListLaneProposals` per site as
  today; a segment-keyed listing (`ListLaneProposals` by S2 cell) replaces that when Gerardus
  serves it, so the city reads in one call per L13 cell rather than one per site.
- **The clips view** keeps Registered and Lanes, and Lanes becomes `n to review / m adopted`
  rather than a tick, read from the same listing. A clip whose field is older than the current
  derivation shows stale, and Re-run lane proposals clears it.

### Evidence and the gap to the next grade (painter)

Each proposal row shows:

- a small lateral-profile ridge, the density across the street at the proposal's weakest station,
  with the fitted centre marked, so the operator sees the ridge without selecting the row;
- one plain line for the grade: "medium: 44 tracks at 59 of 66 stations, spread 0.42 m, agreement
  0.97; high needs 30 at 80% of stations and the halves within 0.2 m". The thresholds are
  Gerardus's (LANE-PROPOSALS §4.7), read from `GetServerInfo` so the pane never hard-codes them;
- the word `tracks` replaced by what it is, "evidence", with the hover text saying it is the
  distinct tracks summed across the candidate's basin, not a vehicle count.

In the view, stations a chain reached by bridging a gap (no candidate of its own) are drawn
hollow, so a chain carried into an intersection shows where its evidence stops, and the operator
trims it with Split or lets Gerardus end it at the stop line.

### Stop lines from where traffic stops (Gerardus, painter)

The field's `speed_sum` and `tracks` give a mean speed per cell. Across a proposed lane, read the
mean speed station by station towards the node: where it falls below 1 m/s over a band two to four
cells long and rises again into the intersection, the band's upstream edge is a stop-line proposal,
a short line across the lane's width, perpendicular to the lane, tagged with the lane it belongs to
and the stations it was read from. Where W5's traced `stop_line` paint lies within 1 m of it, the
proposal takes the paint's position and says so.

- Served with the lane proposals as a `stop_line` proposal, drawn as a bar across the lane with the
  grade of the lane it belongs to.
- The operator confirms it as a `street_use` mark of a new kind, `STOP_LINE` (a two-vertex line;
  `street_use::KINDS` gains it, and the contract document), or has the lane end at it (the lane's
  last vertex snaps to the bar). Rejecting it is a review as for a lane.
- A confirmed stop line ends the lane's chain at the next derivation, so the lane stops where the
  traffic does and the proposal's length no longer reaches into the node.

This is the one item that is research before it is a feature: the band's thresholds are to be
read off the Ashbury and Kirkham fields first, where the stop lines are known, and recorded in the
LANE-PROPOSALS plan before any code reads them.

### Derived direction names (Gerardus)

`direction_name` becomes a tag Gerardus fills on save, as it fills `street`, `cnn`, `bearing_deg`
and `bound`: the segment's display name and the compass word of the bound, `Ashbury St southwestbound`.
The painter's Name field becomes an override, kept when set, and the same name reaches every lane
of that street and bound at every site. velocity.report's directions (snapshot plan, Attribution)
read the tag as they do now.

### The pane as steps, and the small items (painter)

- The pane's sections become the steps the From traffic to lanes section already lists, one open at
  a time: Traffic, Proposals, Lanes and limits, Save. The tools and the layers stay reachable from
  every step. Adopted and rejected proposals are hidden by default, with a toggle.
- The painter plan's outstanding items ship with this: Edit (approve and drag), the cell hover
  inspector with its mean speed, the `T P A R S` keys and their Edit menu items, the crossing
  markers, and the `oneway` warning.

### OpenStreetMap: compatible on the way out, never a dependency

Nothing changes in the stance (ADR-0008, ADR-0009): OSM is drawn only as an optional backdrop
behind consent, is never a snap target, and no mark or proposal takes geometry or tags from it.
Compatibility is export:

- the lane tags map onto OSM's where OSM has them (`lanes`, `oneway`, `turn:lanes`, `maxspeed` as
  `25 mph`), as a mapping in the GeoJSON export, never stored converted;
- a lane may carry `osm_way` as a reference the operator sets, by id only (ADR-0008 rule 4);
- the intersection-mode plan's JOSM change files take the reviewed lane counts and limits as tag
  discrepancies for a person to review; fitted geometry is never offered.

The key is `(cnn, bound, index)` inside an S2 cell, and the city view's segments are the City's.
A city without DataSF-style segments would need a public-domain centreline set with stable ids
before this plan applies there; OSM ways would not be a substitute, because every published lane
would then carry ODbL.

## Scope

### Item 1: proposals per street segment

**Summary:** Gerardus derives and stores a travel-lane proposal per `(cnn, bound, index)` from every
placed field reaching the segment, chains through nodes along the same street, and serves it to
every site containing the segment; authored lanes are stored against the segment; the painter shows
which sites' fields a proposal rests on.

**Milestone:** unscheduled `L` (Gerardus `L`, painter `S`)

### Item 2: one Approve, and the snapshot is pulled

**Summary:** `ReviewLaneProposal(APPROVED)` authors the lane in Gerardus with the limit sent;
Gerardus serves the accepted record over HTTP and a street-use version per site; `lanes pull` runs
on the server on a schedule and on change; Send lanes to velocity.report and its notice are
removed, and the pane shows when the snapshot last caught up.

**Milestone:** unscheduled `M`

### Item 3: the queue on the grid

**Summary:** City mode colours each segment by review state over the city tiles, a click opens the
segment in paint mode, the Lanes to review list gains segment rows and filters, and the clips view's
Lanes column shows to review and adopted counts and stale fields.

**Milestone:** unscheduled `M` {frontend}

### Item 4: evidence and the gap to the next grade

**Summary:** A lateral-profile ridge and a plain grade line per proposal with Gerardus's thresholds
read from the server, `tracks` named as evidence, and gap-bridged stations drawn hollow.

**Milestone:** unscheduled `S` {frontend}

### Item 5: stop lines from where traffic stops

**Summary:** Thresholds read off Ashbury and Kirkham first; then a `stop_line` proposal from the
field's mean speed, placed on W5's paint where it agrees, served with the lane proposals, confirmed
as a `STOP_LINE` mark or as the lane's end, and ending the chain at the next derivation.

**Milestone:** unscheduled `M` {math}

### Item 6: derived direction names and the pane as steps

**Summary:** `direction_name` filled by Gerardus on save with the painter's Name as an override;
the pane's sections as one-at-a-time steps with adopted and rejected proposals hidden by default;
the painter plan's outstanding items (Edit, hover inspector, keys, crossing markers, `oneway`
warning).

**Milestone:** unscheduled `M` {frontend}

### Item 7: hub docs

**Summary:** The alignment pane and city view hub docs, and the painter plan's checklist, state
what shipped, when it ships (D-33).

**Milestone:** with Items 1 to 6 `S`

## Dependencies

- Item 1 is Gerardus work first (a derivation keyed by segment, storage keyed by segment, the
  listing by S2 cell) and changes LANE-PROPOSALS §4 and ADR-0016's storage note; the painter half
  waits on it and on a re-vendored `align.proto`.
- Item 2 needs Gerardus to serve the accepted record over HTTP (the snapshot plan's Item 1
  remainder) and a street-use version per site; the pose agreement check of the pose writeback
  (v0.5.5) stays as the snapshot's guard.
- Item 3 needs the city view's city mode (built) and Item 1's listing by cell for one call per L13
  cell; it works from one call per site before that.
- Item 5 needs the lane field's `speed_sum` (built, unread) and W5's traced stop lines (built at
  3DEP's flight date).
- Items 4 and 6 need nothing new from Gerardus but the thresholds in `GetServerInfo` and the filled
  `direction_name`.

## Risks

| Risk                                                                                     | Likelihood | Impact | Mitigation                                                                                                                                                    |
| ---------------------------------------------------------------------------------------- | ---------- | ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Chaining through nodes joins a lane to a different street's traffic at a skewed junction | Medium     | Medium | Same street name and the join rule's 45° turn; the painter's Split remains; the hollow stations show a bridged joint                                          |
| Two sites' fields disagree on a segment (a pose error at one) and the merged ridge blurs | Medium     | Medium | The proposal lists the sites it rests on with each one's agreement; a site whose field runs more than 0.5 m off the others is flagged on the segment          |
| Server-side approval authors a lane the operator did not look at                         | Low        | Medium | Approve returns the mark selected; Approve all high stays limited to `high`, unadopted; `lane_against_evidence` names a lane the traffic contradicts          |
| The scheduled pull copies a half-edited street (saved, not finished)                     | Medium     | Low    | Snapshots are versioned by measurement time; a pull is a new version, never a replacement, and the pane shows which version the reports use                   |
| Stop-line thresholds read off two sites do not hold elsewhere                            | Medium     | Medium | Research first; the proposal carries its stations and the operator confirms; a confirmed stop line is an authored mark, so a bad proposal costs one rejection |
| An operator reads a `STOP_LINE` mark as a traffic-engineering specification              | Low        | Medium | It is tagged with the stations it was read from and the paint it agreed with; the hub doc says what it is                                                     |
| OSM backdrop tempts tracing                                                              | Low        | High   | Not a snap target (as built); the backdrop is labelled "display only" when on                                                                                 |

## Design decisions

- **The street segment is the unit, keyed by the City's id inside S2.** A lane is one record for
  the city however many sensors see it. CNN is public domain and stable; OSM way ids are neither
  ours nor licence-free.
- **Gerardus authors on approval; velocity.report pulls.** One decision, one authoritative write,
  one copy with provenance. D-39's locus holds; only the copying moves off the operator.
- **State is drawn on the grid, not only counted per site.** The city tiles exist for this.
- **A grade shows its own arithmetic.** No number is shown that the grade did not use, and the
  thresholds come from the server that applied them.
- **A stop line is observed, then confirmed.** It comes from the speed channel the field already
  carries, is placed on paint where paint agrees, and becomes a mark only by the operator's hand.
- **OSM compatibility is a mapping at export.** Nothing is stored in OSM's vocabulary or geometry.

## Open questions

- Should Item 1 store authored lanes against the segment (one mark for the city) or keep them per
  site with a derived link? The plan proposes the segment; the per-site marks of the 24 archive
  sites would be migrated once, by the segment they snap to.
- When two sites' fields reach one segment and only one has an accepted pose, does the segment
  derive from the placed one alone? Proposed: yes, with the unplaced site named on the proposal.
- How often does `lanes pull` run, and is a change notice from Gerardus worth building before a
  schedule is enough? Proposed: a schedule of one hour plus a poll of the street-use version every
  minute while the painter is connected.
- Is `STOP_LINE` a `street_use` kind or a lane vertex attribute? Proposed: a kind, so a stop line
  shared by two lanes is one mark.

## Checklist

### Complete

- [x] Evaluation of the painter and the batch job against a citywide grid, November 7, 2026, and
      this plan

### Outstanding

- [ ] Item 1: proposals and authored lanes per street segment `L`
- [ ] Item 2: `APPROVED` authors the lane; the snapshot is pulled on a schedule and on change; Send
      removed `M`
- [ ] Item 3: segments by state in city mode; segment rows in the list; the clips view's counts `M`
- [ ] Item 4: the ridge, the grade line, evidence named, hollow bridged stations `S`
- [ ] Item 5: stop-line thresholds read off Ashbury and Kirkham, then the derivation and the mark `M`
- [ ] Item 6: derived direction names; the pane as steps; the painter plan's outstanding items `M`
- [ ] Item 7: hub docs at ship `S`

### Deferred

- [ ] Sidewalk and crossing surfaces proposed from the pedestrian field (Gerardus W10f)
- [ ] Successors and merge transitions between segments: [lane stitching](lidar-lane-stitching-plan.md)
- [ ] Schedules on a lane's limit (D-16)

## Not in scope

- Editing lanes on the web. The macOS visualiser is the review instrument; the web stays read-only
  with counts.
- Importing lanes, limits or stop lines from OpenStreetMap, or uploading anything to it.
- Lane geometry for a city without a public-domain centreline set with stable ids.
