# Visualiser lane painter: traffic layer, proposed lanes and limits plan

How the street painter in VelocityVisualiser shows where traffic ran (the lane field, as cumulative
track centres or box extents, coloured by direction of travel), uploads the field to Gerardus,
draws the travel lanes Gerardus proposes from it with their confidence grade, lets the operator
approve, edit or reject each one, gives every travel lane its own posted limit with the date it took
effect and splits a lane where the limit changes, saves the result to Gerardus, and hands the
accepted lanes to velocity.report as the snapshot the speed reports read.

- **Status:** Proposed; waits on the lane field route (lanes from tracks phase B) and on Gerardus
  W10a–c and W1c step 1. Not Active (D-32). Supersedes the paint-mode section (§2) of the
  [lanes from tracks plan](lidar-lane-proposals-from-tracks-plan.md), reconciled November 3, 2026
- **Decisions:** relies on D-21 (visualiser debug overlay controls), D-32, D-33, D-38, D-39 and
  Gerardus ADR-0006, ADR-0011 and ADR-0016 (accepted November 3, 2026: the unified design)
- **Layers:** macOS visualiser (alignment pane, street painter, Metal overlay); the vendored Gerardus
  proto; velocity.report's HTTP API as a client
- **Target:** unscheduled; proposed as the v0.7.19 slot with its companions
- **Companion plans:** [lane fields across the corpus](lidar-lane-field-batch-plan.md) (the batch that fills Send lane field's job for every site), [lanes from tracks](lidar-lane-proposals-from-tracks-plan.md) (the lane field
  and its route), [lane snapshot and attribution](lidar-lane-snapshot-and-attribution-plan.md) (what
  Send writes), [speed by direction and lane](platform-speed-by-direction-reporting-plan.md),
  [posted speed limits](posted-speed-limits-plan.md), [visualiser alignment client](lidar-visualiser-gerardus-alignment-plan.md),
  [light mode](lidar-visualiser-light-mode-plan.md), and Gerardus
  [LANE-PROPOSALS](https://github.com/banshee-data/gerardus/blob/main/docs/plans/LANE-PROPOSALS.md)
  (W10: fields, proposals, review) and
  [LANES-AND-LIMITS](https://github.com/banshee-data/gerardus/blob/main/docs/plans/LANES-AND-LIMITS.md)
  (W1c: the limit tags)
- **Canonical:** [Gerardus alignment pane](../ui/visualiser/gerardus-alignment.md)

What the painter draws is measured or inferred geometry at its acquisition time: the field is
where tracked objects were, counted; a proposal is an inference the operator confirms; a mark is
the operator's reading of the street. None of it is a survey or a specification. See
[Geometry priors: data purpose](../geometry-priors-data-purpose.md).

## Motivation

The street painter lets an operator mark travel lanes over the placed reference, but the only
evidence under the cursor is the settled background: kerbs, paint where the intensity shows it,
and the City's centrelines. Where traffic actually runs, how many lanes it uses, and which way,
are not drawn, so a lane is placed by eye and never checked. A lane also has no limit, so the
reports cannot use it.

The owner's ask is concrete: paint the lanes from where the sensor saw cars and people, see the
proposals, approve or edit them, see the direction of travel, and give each lane its own limit so
an intersection signed 25 mph one way and 30 mph the other reports correctly. This plan is the
visualiser's half of that: velocity.report counts the traffic, Gerardus proposes and stores, and
this is where the operator decides.

## Current state

- **Paint mode is built** ([hub doc](../ui/visualiser/gerardus-alignment.md#paint-mode-the-street-painter)):
  a top-down orthographic plan view locked north-up over the placed reference
  (`Alignment/StreetGeometry.swift`, `PlanView`), tools for surface, travel lane, parking space,
  a row of spaces and driveway, snapping within 0.5 m to kerbs, centrelines, box walls and other
  marks, Undo and Redo, a batch save with `EditStreetUse` that keeps the draft on conflict
  (`Alignment/StreetPainterSession.swift`, `Alignment/StreetUseEditor.swift`), and the marks drawn
  by kind with an arrowhead every 8 m along a lane in its direction of travel
  (`Alignment/StreetPaintStyle.swift`).
- **A lane's direction is its vertex order**, and **Reverse** turns it round. Its tags are
  `lane_type` (`general`, `bus`, `bike`, `turn`) and `width_m`. There is no limit field.
- **The pane streams Gerardus's layers** including city streets with their tags, kerbs, lane
  markings (W5) and street furniture (W6), each with a toggle, and reads the site's street use with
  `ListStreetUse`.
- **The visualiser accumulates frames** for an accumulated scan and computes each frame's
  transient boxes (`Alignment/FrameAccumulation.swift`): the pattern a live fallback field can
  reuse.
- **The accepted pose is in the alignment session** (`SitePose`, with its inverse for drawing), and
  the pose writeback to velocity.report is a v0.5.5 item.
- **Settings ▸ Servers** holds the Gerardus address and the painter's author name
  (`streets.author`), and the velocity.report HTTP address the site lookup uses.
- **Tracks arrive per frame** in `FrameBundle` with position, velocity, heading, class label and
  the published box; the renderer draws boxes and trails. There is no accumulated top-down view.

## Design

### Where it sits

Paint mode gains a **Traffic** section in the pane, above the tools, and two overlay layers in the
plan view: the field and the proposals. Both draw under the marks and over the reference, with the
debug-line pipeline the marks use for proposals and a textured quad per class group for the field,
so the Metal work stays in the painter's overlay and the point shader is unchanged (as the reference
palette did, hub doc, Frames and drawing). Outside paint mode neither layer is drawn, and the 3D
view behaves as it does today.

### The traffic layer: cumulative tracks and box extents

| Control         | Does                                                                                                                                                                                                                                                                                                                                   |
| --------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Source          | This recording: the open run's field from `GET /api/lidar/runs/{run_id}/lane-field` (lanes from tracks §1). The site: the summed field of every capture uploaded so far, in the site frame, from Gerardus's `GetLaneField`: the cumulative view. The pane names which is shown, with its provenance (captures, track count, field ids) |
| Show            | Centres (track centres per cell) or Extents (box footprint coverage per cell); the operator's two views of the same field                                                                                                                                                                                                              |
| Classes         | Vehicles, cycles, people; each a toggle with its track count                                                                                                                                                                                                                                                                           |
| Colour          | By direction of travel, from the cell's dominant heading bin: each direction gets a hue from a fixed, light-and-dark-mode-safe set (vehicles), with people and cycles in their own hues; the legend names each direction's bearing and bound                                                                                           |
| Floor           | The field carries no cell under five distinct tracks (ADR-0011); a higher floor fades thinner cells for reading                                                                                                                                                                                                                        |
| Opacity         | Of the quad; the field must never hide the paint or the kerbs under it                                                                                                                                                                                                                                                                 |
| Live (fallback) | When the server has no field for the open capture, accumulate centres and boxes from the stream as it plays, labelled "live, this replay only", using the accumulation pattern; never saved                                                                                                                                            |

The quad is one texture per class group in the sensor frame, cell 0.25 m, alpha by the log of the
count, hue by direction. It is placed by the same model matrix as the reference, so a pose nudge
moves the reference and leaves the field where the sensor saw it: a disagreement between kerbs and
traffic is then a pose error made visible, which is a feature.

Hovering a cell shows its counts per class and direction and its mean speed in the inspector. No
cell can be clicked through to a track: the field carries none.

**Send lane field.** Uploads the open run's three fields (the whole run and its two halves) to
Gerardus with the accepted record's id (`UploadLaneField`, chunked like a scan). It is blocked
without an accepted pose, while a save is pending, and when the server lacks `supports_lane_fields`,
and it follows the alignment pane's one-long-call-on-the-wire rule. After the upload the summed
field and the proposals are read again.

### Proposals: shown, approved, edited, rejected

Proposals come from Gerardus's `ListLaneProposals` (Gerardus LANE-PROPOSALS §5.1): the current
`travel_lane` proposal rows with their tags, each with its lateral profile at every station, and
`adopted_by` (an authored lane runs along it) and `review` (rejected or reopened) where they exist.
They are in the site frame and are drawn like the marks.

| Grade or state        | Drawn as                                                                                                              |
| --------------------- | --------------------------------------------------------------------------------------------------------------------- |
| High                  | A solid proposal line in the direction hue, arrowheads every 8 m, a faint band `width_m` across, a badge "high"       |
| Medium                | The same, dotted, a badge "medium"                                                                                    |
| Adopted               | Faint: an authored lane already runs along it (`adopted_by`)                                                          |
| Rejected              | Struck through, until reopened                                                                                        |
| Disagrees with a mark | Gerardus's `lane_against_evidence` problem on the mark: a warning badge where the traffic runs against it             |
| Crossing tagged       | Where a proposal crosses a band of people (`pedestrian_crossings`), a marker at each station; W5's crosswalk compared |
| Selected              | The white band and handles the marks use                                                                              |

The pane lists proposals by direction, sorted by grade, each with tracks, lateral spread,
direction agreement, half-window agreement, street, suggested type and width. Selecting one centres
the view and draws its lateral profiles as thin cross-lines, so the operator sees the ridge the line
was fitted to. The actions:

- **Approve** adds a new travel-lane mark with the proposal's vertices, `lane_type` and `width_m`,
  and the tags `proposal=<feature id>` and `confidence=<grade>`, and selects it; it is now an
  ordinary mark the operator can move vertex by vertex, as today. Approving is one undoable change.
- **Edit** approves and starts a vertex drag at the clicked vertex.
- **Approve all high** approves every unadopted `high` proposal, as one undoable change, for a site
  with clean traffic.
- **Reject** calls `ReviewLaneProposal(REJECTED)` with the proposal's row id and a stable request
  id, through the pending-request discipline the painter's Save uses. The rejection is stored in
  Gerardus, append-only, and holds across re-derivation until the evidence makes a different lane.
  **Reopen** reverses it.

Approvals reach Gerardus as marks, on Save; rejections are their own call. Gerardus tags the
proposals an approved lane adopted, so the next read draws them faint.

### Direction of travel

Direction stays what it is: the order of a lane's vertices, drawn with arrowheads, reversed with
**Reverse**. The painter adds:

- a **bound** shown on each lane (N, NE, E, and so on, from the site-frame bearing of the whole
  run), computed locally the way `StreetGeometry.laneDirection` already does, and written as the
  tag `bound` so Gerardus and the snapshot carry it;
- a **direction name** per group of lanes, "Kirkham St eastbound", from the display name of the
  city street the lane snapped to (`cnn` tag) and the bound; editable in the pane, kept as the tag
  `direction_name` on each lane of the group so a report can print it;
- a warning when an approved proposal's direction disagrees with the city street's `oneway` tag.

### Limits: one per lane

Selecting a travel lane shows a **Limit** section:

| Field          | Values                                                                                                                                                                                                 |
| -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Value          | The number on the sign; integer                                                                                                                                                                        |
| Unit           | `mph` or `km/h`; defaults to the unit of the last limit entered at this site, else `mph` for a US jurisdiction                                                                                         |
| Source         | `sign` (read from a sign at the site), `city` (the City's speed-limit record), `statutory` (the jurisdiction's default where no sign governs), `operator` (entered without either)                     |
| Jurisdiction   | ISO 3166-2, default `US-CA`, remembered per site                                                                                                                                                       |
| Note           | Free text, for example "school zone 15 mph when children are present"; schedules themselves are D-16's later item                                                                                      |
| Effective from | The date the limit took effect: the sign's installation or the ordinance date where known, the City record's date when prefilled from it, else the capture's date with a note that it is a lower bound |
| Effective to   | Optional; set when a limit is known to have ended                                                                                                                                                      |

**Prefill.** When a lane snaps to a city street segment whose Gerardus tags carry a City limit
(`speed_limit_mph` from SFMTA's record, the Gerardus plan's import), the fields are prefilled with
source `city` and the operator confirms. When there is none, the fields are prefilled with the
jurisdiction's statutory default, 25 mph for `US-CA`, with source `statutory`, so a lane never
silently carries the old per-report default as if it were posted. The operator can always change
the source to `sign`.

**A limit that changes over time is a new version of the same lane.** Changing a saved lane's limit
value does not overwrite it: the painter asks for the new effective date, keeps the lane's id and
geometry, and Save writes a new version; Gerardus keeps every version, and the snapshot carries the
dates so reports attribute each measurement to the limit that stood at its time (companion plans).
The previous version's **Effective to** is set to the day before unless the operator says otherwise.
When Gerardus's City record disagrees with a saved lane (its `lane_limit_disagrees_city` problem,
raised after a bundle picks up a newer record), the pane shows the City's value and date and offers
exactly that new version.

**A lane has one limit at a time.** Where the limit changes along a street, the **Split at sign** tool clicks
a point on a lane and splits it into two lanes sharing that vertex, both keeping the direction, type
and width; the downstream lane's limit fields open for the new value. Where Gerardus's street
furniture layer carries a pole with a speed-limit sign code (the Gerardus plan asks it to flag
these), the painter offers the split at the sign's foot.

**Join lanes** is the reverse. Gerardus derives a lane per city street segment, so one lane through an
intersection arrives as two proposals meeting at the node. The tool joins a lane to the one that
continues it, keeping every vertex so that both proposals are still adopted by geometry. It refuses
lanes whose ends are more than 5 m apart, that turn more than 45° at the joint, that travel in
opposite directions, or that carry different limits or lane types.

**Checks before Save** (shown, not blocking, in the style of Gerardus's own problems): a travel
lane with no limit; two lanes in one direction with different limits and no split between them;
a limit whose unit differs from the site's others.

The tags written are those the Gerardus plan defines: `speed_limit`, `speed_limit_unit`,
`speed_limit_source`, `speed_limit_jurisdiction`, `speed_limit_note`. The painter does not convert
units: a 25 mph sign is stored as 25 and `mph` (posted-limits plan, "A limit is signed in a unit").

### Sidewalks and crossings

Gerardus keeps people's cells out of the carriageway and tags where a lane crosses a band of them
(`pedestrian_crossings`); proposing sidewalk and crossing surfaces from the pedestrian field is its
W10f, later. Until then the operator draws `surface=sidewalk` and `surface=crossing` polygons with
the surface tool, using the people layer as the guide.

### Save, then send the snapshot

Save is unchanged: one `EditStreetUse` with every change, all or nothing, with the draft kept on
conflict and the retry rules of Gerardus's edits contract. After a successful save the pane offers
**Send lanes to velocity.report**:

1. It requires an accepted pose for the open capture's placement (the alignment session's
   `MANUALLY_PLACED` or `REGISTERED` record). Without one the button is disabled with the reason: a
   snapshot without a pose cannot place lanes in the sensor frame.
2. It sends `PUT /api/lidar/sites/{l16}/lanes` with the street-use features exactly as Gerardus
   returned them, the pose record id and pose, the author, the Gerardus server version and address,
   and a request id kept until the outcome is known, as the painter's own `pendingSave` is.
3. The server replies with the snapshot id and the directions it derived; the pane shows them, and
   the direction names come back for the operator to check.
4. A refusal (pose disagrees with the pose record; unknown site) is shown with the server's reason;
   nothing is retried on its own.

The two writes stay separate on purpose: Gerardus is authoritative and must succeed first;
velocity.report's copy follows. A save that succeeds in Gerardus but is never sent leaves the
reports on the previous snapshot, and the pane says so with a standing notice until it is sent.

### The operator's path, extended

| Step                          | What the pane does                                                                            | Calls                                        |
| ----------------------------- | --------------------------------------------------------------------------------------------- | -------------------------------------------- |
| Paint streets                 | As today                                                                                      | none                                         |
| Load traffic                  | Fetches the run's field and the site's summed field, and draws the chosen one under the marks | `GET …/runs/{id}/lane-field`, `GetLaneField` |
| Send lane field               | Uploads the run's three fields against the accepted record                                    | `UploadLaneField`                            |
| Propose                       | Reads Gerardus's proposals with their profiles and lists them by direction                    | `ListLaneProposals`                          |
| Approve, edit                 | Local; marks change as any mark does                                                          | none                                         |
| Reject, reopen                | Records the review in Gerardus                                                                | `ReviewLaneProposal`                         |
| Set limits, split at signs    | Local                                                                                         | none                                         |
| Save street use               | As today                                                                                      | `EditStreetUse`                              |
| Send lanes to velocity.report | Writes the snapshot with the pose                                                             | `PUT …/lanes`                                |

### Keyboard and menus

`T` toggles the traffic layer, `P` the proposals, `A` approves the selected proposal, `R` rejects
it, and `S` picks the split tool, all only while painting and none shadowing today's keys (Return,
Escape, Delete). The Edit menu gains the same items with their shortcuts, following
[menu-layout.md](../ui/visualiser/menu-layout.md).

### Tests

Unit tests in the style of `StreetPainterTests` and `AlignmentTests`, against a fake
velocity.report API and the fake Gerardus:

- the field texture is built from a synthetic cell list, the run's field placed by the model matrix
  and the summed field drawn in the site frame;
- the upload sends three fields with the record id, and is blocked without an accepted pose;
- approve makes a mark with `proposal` and `confidence`; reject and reopen send the right request
  with the row id and survive a reload; undo restores;
- split at a point yields two lanes sharing the vertex with tags copied and the downstream lane's
  limit cleared;
- prefill takes the city record when present and the statutory default otherwise, never silently;
- the snapshot request carries the features as returned, the pose record, and a stable request id
  across retries; a refusal leaves the pending request in place;
- light and dark mode hues pass the contrast rule in the light-mode plan.

## Scope

### Item 1: traffic layer and field upload

**Summary:** Fetch the run's field and the site's summed field and draw either as cumulative centres
or box extents, coloured by direction, with class toggles; a labelled live fallback from the
stream; Send lane field.

**Milestone:** v0.7.19 `M`

### Item 2: proposals and review

**Summary:** Read, draw and list Gerardus's graded proposals with their profiles; approve, edit and
approve-all-high as undoable changes; reject and reopen through `ReviewLaneProposal`.

**Milestone:** v0.7.19 `M`

### Item 3: direction names, limits and split at sign

**Summary:** Bound and direction name per lane, the limit fields with prefill and checks, and the
split tool, written as Gerardus's tags.

**Milestone:** v0.7.19 `M`

### Item 4: send the snapshot

**Summary:** After Save, write the lane snapshot to velocity.report with the pose and show the
derived directions; standing notice until sent.

**Milestone:** v0.7.19 `S`

### Item 5: hub doc and menu layout

**Summary:** Extend the alignment pane hub doc's paint mode section and the menu layout doc with
what shipped, when it ships (D-33).

**Milestone:** with Items 1 to 4 `S`

## Dependencies

- The lane field route (lanes from tracks phase B), Gerardus W10a (`UploadLaneField`,
  `GetLaneField`), W10c (`ListLaneProposals`, `ReviewLaneProposal`), and the snapshot endpoints
  ([lane snapshot and attribution](lidar-lane-snapshot-and-attribution-plan.md)); Item 1's live
  fallback works without them, nothing else does. `align.proto` is re-vendored with
  `make proto-gen-swift` once Gerardus ships them.
- An accepted pose in the alignment session, which exists today; the pose writeback endpoint for the
  server's agreement check, which is v0.5.5.
- Gerardus's tag validation and `crossing` surface value, for the limit checks and crossing
  approvals; the painter writes the tags either way, since Gerardus's tags are free-form today.
- Gerardus's City speed-limit import and sign-code flag, for prefill and the offered split; without
  them prefill is the statutory default and the split is manual.

## Risks

| Risk                                                                             | Likelihood | Impact | Mitigation                                                                                                                                                              |
| -------------------------------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| The field hides the reference under it and the operator snaps to the wrong thing | Medium     | Medium | The field is a quad under the marks with an opacity control; snapping never targets the field                                                                           |
| Approve-all puts wrong lanes into Gerardus                                       | Medium     | Medium | Only `high`, unadopted proposals; one undoable change; marks carry `proposal` and `confidence`; Gerardus's `lane_against_evidence` names a lane the traffic contradicts |
| The statutory prefill is taken as a posted limit                                 | Medium     | Medium | Source is a required field shown on the lane and in the reports; `statutory` prints as a default, not a sign                                                            |
| Pose moves after the snapshot is sent                                            | Medium     | High   | The snapshot names its pose record; the server refuses a snapshot whose pose disagrees with the pose record; re-send after accepting a new pose                         |
| Live fallback mistaken for a stored field                                        | Low        | Medium | Labelled "live, this replay only" in the legend and the inspector; never saved                                                                                          |
| Metal texture per class group costs frame time on large sites                    | Low        | Low    | Sparse cells rasterised once per fetch; the texture is bounded by the field's extent                                                                                    |

## Design decisions

- **Gerardus proposes, the painter decides.** No fitting runs in Swift except the labelled live
  fallback; proposals, grades and thresholds are Gerardus's, so the web viewer and the painter show
  the same lanes (Gerardus ADR-0016, accepted November 3, 2026).
- **A proposal becomes an ordinary mark.** Approval is an authored travel lane naming its proposal;
  a rejection is an append-only review in Gerardus. Neither changes the proposal row.
- **Direction stays the vertex order.** No new direction tag beyond the derived `bound` and the
  group's `direction_name`, so today's marks and GeoJSON keep their meaning.
- **A lane has one limit at a time; split where it changes along the street, version it where it
  changes over time.** The posted-limits plan's limit segments are lanes, so there is no separate
  limit-change record to keep in step.
- **Two writes, Gerardus first.** The snapshot is a copy with provenance, never the source.
- **Nothing the painter makes is committed.** Marks and reviews live in Gerardus, snapshots in
  velocity.report's database; screenshots for the hub doc come from the render
  tool's output in the Hugging Face dataset ([lane snapshot and attribution](lidar-lane-snapshot-and-attribution-plan.md)), not from files under `docs/`.

## Open questions

- Should the traffic layer open on the run's field or the site's summed field? The summed field is
  proposed, since it is what the proposals were read from.
- Should `medium` proposals be approvable, or only `high`? Both, with the grade copied onto the
  mark's `confidence` tag (Gerardus LANE-PROPOSALS §10); the owner may narrow it.
- Where does the direction name live when a group spans lanes drawn on two city street segments
  (an intersection approach drawn across the node)? The first segment's name is the proposal.
- Is a km/h site expected before the jurisdiction field needs more than a default?

## Checklist

### Complete

- [x] Plan written against the painter as built; reconciled with the unified design and the lanes from tracks plan's §2 on November 3, 2026
- [x] Item 1: traffic layer from both field sources and the live fallback, Centres and Extents, class toggles, floor and opacity, Send lane field against the accepted record, with `align.proto` re-vendored from Gerardus `e9dc3e0`; cells are quads on the marks' overlay rather than a texture (`claude/lane-painter-traffic`)
- [x] Items 2 and 3: proposals drawn and listed with their profiles, Approve, Approve all high, Reject and Reopen through `ReviewLaneProposal` with the row id and a stable request id, Split at sign, the bound and a direction name per lane, the limit row with the statutory prefill and the checks before Save (`claude/lane-painter-proposals`)
- [x] Item 4: Send lanes to velocity.report after a save, with the features as Gerardus returned them, the accepted record and pose, a request id kept until the outcome is known, the derived directions shown, and a standing notice until sent (`claude/lane-snapshot-attribution`)

### Outstanding

- [ ] Item 2 remainder: Edit (approve and start a vertex drag at the clicked vertex), the crossing markers from `pedestrian_crossings`, and the `T`, `P`, `A`, `R` and `S` keys with their Edit menu items
- [ ] Item 3 remainder: the City prefill and the offered split at a sign pole, which wait on Gerardus's speed-limit import and sign-code flag; the warning against a city street's `oneway`, which Gerardus raises as `lane_against_oneway` after Save
- [ ] Item 1 remainder: hovering a cell shows its counts per class and direction and its mean speed in the inspector
- [ ] Item 5: hub doc and menu layout updated at ship (`S`)

### Deferred

- [ ] Schedules on a lane's limit (D-16): [speed-limit schedules](../radar/architecture/speed-limit-schedules.md), after the posted-limits rows
- [ ] Successors and merge transitions drawn in the painter: [lane-stitching plan](lidar-lane-stitching-plan.md)

### Accepted residuals (no action planned)

- [ ] The painter draws no per-track trails in the plan view; the field is the only traffic layer there
- [ ] Turn lanes are drawn by hand; proposals leave turning traffic unclaimed
