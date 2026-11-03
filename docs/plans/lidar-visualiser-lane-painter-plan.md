# Visualiser lane painter: traffic layer, proposed lanes and limits plan

How the street painter in VelocityVisualiser shows where traffic ran (the lane field, as cumulative
track centres or box extents, coloured by direction of travel), draws the lanes, sidewalks and
crossings the server proposes from it with their confidence grade, lets the operator approve, edit or
dismiss each one, gives every travel lane its own posted limit and splits a lane where the limit
changes, saves the result to Gerardus, and hands the accepted lanes to velocity.report as the
snapshot the speed reports read.

- **Status:** Proposed; waits on the lane field and proposals endpoints and on the owner's decision
  on Gerardus ADR-0016 (proposed there). Not Active (D-32)
- **Decisions:** relies on D-21 (visualiser debug overlay controls), D-32, D-33, D-38 and Gerardus
  ADR-0006, ADR-0011 and the proposed ADR-0016; makes no new register entry (the companion server
  plan proposes one)
- **Layers:** macOS visualiser (alignment pane, street painter, Metal overlay); the vendored Gerardus
  proto; velocity.report's HTTP API as a client
- **Target:** unscheduled; proposed for the v0.7.1 slot with its companions
- **Companion plans:** [lane field and proposals](lidar-lane-field-and-lane-proposals-plan.md) (what
  this consumes and writes), [speed by direction and lane](platform-speed-by-direction-reporting-plan.md),
  [posted speed limits](posted-speed-limits-plan.md), [visualiser alignment client](lidar-visualiser-gerardus-alignment-plan.md),
  [light mode](lidar-visualiser-light-mode-plan.md), and Gerardus
  [LANES-AND-LIMITS](https://github.com/banshee-data/gerardus/blob/main/docs/plans/LANES-AND-LIMITS.md)
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
visualiser's half of that: the server proposes, Gerardus stores, and this is where the operator
decides.

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

| Control         | Does                                                                                                                                                                                                                         |
| --------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Window          | Which runs the field reads: the open capture, every capture of this deployment, or a date range. Fetches `GET /api/lidar/sites/{l16}/lane-field` and shows its provenance (runs, tuning fingerprints, digest)                |
| Show            | Centres (track centres per cell) or Extents (box footprint coverage per cell); the operator's two views of the same field                                                                                                    |
| Classes         | Vehicles, cycles, people; each a toggle with its track count                                                                                                                                                                 |
| Colour          | By direction of travel: each direction mode the server found gets a hue from a fixed, light-and-dark-mode-safe set (vehicles), with people and cycles in their own hues; the legend names each direction's bearing and bound |
| Floor           | Cells under this many distinct tracks are faded (default five, ADR-0011's floor; never below one)                                                                                                                            |
| Opacity         | Of the quad; the field must never hide the paint or the kerbs under it                                                                                                                                                       |
| Live (fallback) | When the server has no field for the open capture, accumulate centres and boxes from the stream as it plays, labelled "live, this replay only", using the accumulation pattern; never saved                                  |

The quad is one texture per class group in the sensor frame, cell 0.25 m, alpha by the log of the
count, hue by direction. It is placed by the same model matrix as the reference, so a pose nudge
moves the reference and leaves the field where the sensor saw it: a disagreement between kerbs and
traffic is then a pose error made visible, which is a feature.

Hovering a cell shows its counts per class and direction and its mean speed in the inspector. No
cell can be clicked through to a track: the field carries none.

### Proposals: shown, approved, edited, dismissed

Proposals come from `GET /api/lidar/sites/{l16}/lane-proposals` for the loaded field, matched
against the marks the painter holds (it passes them in the request, so a lane already painted shows
as confirmed or disagreeing rather than proposed again). They are in the sensor frame and are placed
into the site frame by the accepted pose before drawing, as the marks are.

| Grade or state        | Drawn as                                                                                                        |
| --------------------- | --------------------------------------------------------------------------------------------------------------- |
| High                  | A solid proposal line in the direction hue, arrowheads every 8 m, a faint band `width_m` across, a badge "high" |
| Medium                | The same, dotted, a badge "medium"                                                                              |
| Confirms a mark       | No new line; the existing mark gains a small check badge with the support count                                 |
| Disagrees with a mark | The existing mark gains a warning badge and a thin offset line where the data runs                              |
| Sidewalk, crossing    | A dotted outline polygon in the people hue; a crossing that confirms a W5 crosswalk marking shows a check badge |
| Selected              | The white band and handles the marks use                                                                        |

The pane lists proposals by direction, each with grade, support, lateral spread, half-window
agreement, suggested type and width, and three actions:

- **Approve** turns it into a mark with the suggested tags, `gms:source=fitted` and the fit
  provenance tags the server supplied, and selects it; it is now an ordinary mark the operator can
  move vertex by vertex, as today. Approving is one undoable change.
- **Edit** approves and starts a vertex drag at the clicked vertex.
- **Dismiss** hides it for this field digest. Dismissals are kept per site in the app's preferences,
  keyed by the field digest and the proposal's own id, so a dismissed lane does not return until the
  field changes. Nothing is sent anywhere.
- **Approve all high** approves every high-grade proposal that matches no mark, as one undoable
  change, for a site with clean traffic.

A proposal is never saved as a proposal: Gerardus receives marks, and only on Save.

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

| Field        | Values                                                                                                                                                                             |
| ------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Value        | The number on the sign; integer                                                                                                                                                    |
| Unit         | `mph` or `km/h`; defaults to the unit of the last limit entered at this site, else `mph` for a US jurisdiction                                                                     |
| Source       | `sign` (read from a sign at the site), `city` (the City's speed-limit record), `statutory` (the jurisdiction's default where no sign governs), `operator` (entered without either) |
| Jurisdiction | ISO 3166-2, default `US-CA`, remembered per site                                                                                                                                   |
| Note         | Free text, for example "school zone 15 mph when children are present"; schedules themselves are D-16's later item                                                                  |

**Prefill.** When a lane snaps to a city street segment whose Gerardus tags carry a City limit
(`speed_limit_mph` from SFMTA's record, the Gerardus plan's import), the fields are prefilled with
source `city` and the operator confirms. When there is none, the fields are prefilled with the
jurisdiction's statutory default, 25 mph for `US-CA`, with source `statutory`, so a lane never
silently carries the old per-report default as if it were posted. The operator can always change
the source to `sign`.

**A lane has one limit.** Where the limit changes along a street, the **Split at sign** tool clicks
a point on a lane and splits it into two lanes sharing that vertex, both keeping the direction, type
and width; the downstream lane's limit fields open for the new value. Where Gerardus's street
furniture layer carries a pole with a speed-limit sign code (the Gerardus plan asks it to flag
these), the painter offers the split at the sign's foot.

**Checks before Save** (shown, not blocking, in the style of Gerardus's own problems): a travel
lane with no limit; two lanes in one direction with different limits and no split between them;
a limit whose unit differs from the site's others.

The tags written are those the Gerardus plan defines: `speed_limit`, `speed_limit_unit`,
`speed_limit_source`, `speed_limit_jurisdiction`, `speed_limit_note`. The painter does not convert
units: a 25 mph sign is stored as 25 and `mph` (posted-limits plan, "A limit is signed in a unit").

### Sidewalks and crossings

Pedestrian proposals are approved into `surface=sidewalk` polygons as today's surface tool makes,
and into `surface=crossing` polygons once Gerardus accepts that value; until then the painter shows
crossing proposals and their match to W5 crosswalk markings but offers no approval for them.

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

| Step                          | What the pane does                                                                            | Calls                  |
| ----------------------------- | --------------------------------------------------------------------------------------------- | ---------------------- |
| Paint streets                 | As today                                                                                      | none                   |
| Load traffic                  | Fetches the field for the chosen window and draws it under the marks                          | `GET …/lane-field`     |
| Propose                       | Fetches proposals for that field, matched against the marks held, and lists them by direction | `GET …/lane-proposals` |
| Approve, edit, dismiss        | Local; marks change as any mark does                                                          | none                   |
| Set limits, split at signs    | Local                                                                                         | none                   |
| Save street use               | As today                                                                                      | `EditStreetUse`        |
| Send lanes to velocity.report | Writes the snapshot with the pose                                                             | `PUT …/lanes`          |

### Keyboard and menus

`T` toggles the traffic layer, `P` the proposals, `A` approves the selected proposal, `D` dismisses
it, and `S` picks the split tool, all only while painting and none shadowing today's keys (Return,
Escape, Delete). The Edit menu gains the same items with their shortcuts, following
[menu-layout.md](../ui/visualiser/menu-layout.md).

### Tests

Unit tests in the style of `StreetPainterTests` and `AlignmentTests`, against a fake
velocity.report API and the fake Gerardus:

- the field texture is built from a synthetic cell list and placed by the model matrix;
- a proposal is placed into the site frame by the pose and matches a mark within 0.5 m;
- approve makes a mark with the fit tags; dismiss persists per digest; undo restores;
- split at a point yields two lanes sharing the vertex with tags copied and the downstream lane's
  limit cleared;
- prefill takes the city record when present and the statutory default otherwise, never silently;
- the snapshot request carries the features as returned, the pose record, and a stable request id
  across retries; a refusal leaves the pending request in place;
- light and dark mode hues pass the contrast rule in the light-mode plan.

## Scope

### Item 1: traffic layer

**Summary:** Fetch the field and draw it as cumulative centres or box extents, coloured by
direction, with class toggles and a floor; a labelled live fallback from the stream.

**Milestone:** v0.7.1 `M`

### Item 2: proposals and approval

**Summary:** Fetch, draw and list graded proposals matched against the marks; approve, edit,
dismiss and approve-all-high as undoable changes.

**Milestone:** v0.7.1 `M`

### Item 3: direction names, limits and split at sign

**Summary:** Bound and direction name per lane, the limit fields with prefill and checks, and the
split tool, written as Gerardus's tags.

**Milestone:** v0.7.1 `M`

### Item 4: send the snapshot

**Summary:** After Save, write the lane snapshot to velocity.report with the pose and show the
derived directions; standing notice until sent.

**Milestone:** v0.7.1 `S`

### Item 5: hub doc and menu layout

**Summary:** Extend the alignment pane hub doc's paint mode section and the menu layout doc with
what shipped, when it ships (D-33).

**Milestone:** with Items 1 to 4 `S`

## Dependencies

- The companion server plan's endpoints (field, proposals, snapshot); Item 1's live fallback works
  without them, nothing else does.
- An accepted pose in the alignment session, which exists today; the pose writeback endpoint for the
  server's agreement check, which is v0.5.5.
- Gerardus's tag validation and `crossing` surface value, for the limit checks and crossing
  approvals; the painter writes the tags either way, since Gerardus's tags are free-form today.
- Gerardus's City speed-limit import and sign-code flag, for prefill and the offered split; without
  them prefill is the statutory default and the split is manual.

## Risks

| Risk                                                                             | Likelihood | Impact | Mitigation                                                                                                                                      |
| -------------------------------------------------------------------------------- | ---------- | ------ | ----------------------------------------------------------------------------------------------------------------------------------------------- |
| The field hides the reference under it and the operator snaps to the wrong thing | Medium     | Medium | The field is a quad under the marks with an opacity control; snapping never targets the field                                                   |
| Approve-all puts wrong lanes into Gerardus                                       | Medium     | Medium | Only high-grade, unmatched proposals; one undoable change; marks carry `gms:source=fitted` and support                                          |
| The statutory prefill is taken as a posted limit                                 | Medium     | Medium | Source is a required field shown on the lane and in the reports; `statutory` prints as a default, not a sign                                    |
| Pose moves after the snapshot is sent                                            | Medium     | High   | The snapshot names its pose record; the server refuses a snapshot whose pose disagrees with the pose record; re-send after accepting a new pose |
| Live fallback mistaken for the server's field                                    | Low        | Medium | Labelled "live, this replay only" in the legend and the inspector; never saved                                                                  |
| Metal texture per class group costs frame time on large sites                    | Low        | Low    | Sparse cells rasterised once per fetch; the texture is bounded by the field's extent                                                            |

## Design decisions

- **The server proposes, the painter decides.** No fitting runs in Swift except the labelled live
  fallback; grades and thresholds are the server's, so the web and the painter agree
  (companion plan, Design decisions).
- **A proposal becomes an ordinary mark.** Approval is not a separate state in Gerardus; the fit
  provenance travels as tags (Gerardus ADR-0016, proposed).
- **Direction stays the vertex order.** No new direction tag beyond the derived `bound` and the
  group's `direction_name`, so today's marks and GeoJSON keep their meaning.
- **A lane has one limit; split where it changes.** The posted-limits plan's limit segments are
  lanes, so there is no separate limit-change record to keep in step.
- **Two writes, Gerardus first.** The snapshot is a copy with provenance, never the source.

## Open questions

- Should the field be fetched for the deployment by default, or for the open capture? The server
  plan asks the same; the painter follows its default.
- Does a dismissed proposal stay dismissed when the field digest changes only because a new run was
  added? Keyed by digest it returns, which may be right.
- Where does the direction name live when a group spans lanes drawn on two city street segments
  (an intersection approach drawn across the node)? The first segment's name is the proposal.
- Is a km/h site expected before the jurisdiction field needs more than a default?

## Checklist

### Complete

- [x] Plan written against the painter as built and the companion server plan

### Outstanding

- [ ] Item 1: traffic layer, texture, controls, live fallback (`M`)
- [ ] Item 2: proposals list, drawing, approve, edit, dismiss (`M`)
- [ ] Item 3: bound, direction names, limit fields, prefill, split at sign (`M`)
- [ ] Item 4: send the snapshot, pending request, standing notice (`S`)
- [ ] Item 5: hub doc and menu layout updated at ship (`S`)

### Deferred

- [ ] Schedules on a lane's limit (D-16): [speed-limit schedules](../radar/architecture/speed-limit-schedules.md), after the posted-limits rows
- [ ] Successors and merge transitions drawn in the painter: [lane-stitching plan](lidar-lane-stitching-plan.md)

### Accepted residuals (no action planned)

- [ ] The painter draws no per-track trails in the plan view; the field is the only traffic layer there
- [ ] Turn lanes are drawn by hand; proposals leave turning traffic unclaimed
