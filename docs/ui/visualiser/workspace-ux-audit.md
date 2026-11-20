# VelocityVisualiser workspace audit

- **Status:** observed behaviour and design exploration; viewport-first direction preferred
- **Scope:** macOS workspace, Alignment buildings and roofs, navigation and panel use
- **Evidence:** native UI inspection of app commit `ae6ac66fb6790f488c3affc5b23d126c0869e4a6`

The November 14, 2026 inspection found working building visibility and roof colouring,
but an interface that makes their effect difficult to distinguish from other building
layers. Panel stacking is the larger space problem: Inspector alone consumes 520 points,
and Camera, Roads, Alignment and refinement can add further columns.

This report records the observations needed to challenge the layout before implementing
a redesign. The operator selected the largest possible 3D viewport with panels appearing
on demand as the leading preference. The [Claude Design exploration](https://claude.ai/design/p/98edced9-30f9-4a24-9d80-4e1558a1bf72) compares a viewport-first <!-- agent-doc-link: ignore (owner-held source, not yet moved into the repository) -->
workspace with a compact inspector. The existing
[macOS VelocityVisualiser evolution project](https://claude.ai/design/p/9f0769cb-0f23-47e5-932f-a862d24de83c) <!-- agent-doc-link: ignore (owner-held source, not yet moved into the repository) -->
was consulted without modification.

## Method and limits

The audit used the running Debug app, a real local Gerardus service reporting
`0.2.0+e506bd70e39d`, and Ashbury at Downey's cached reference. It changed reversible
viewer state: playback, selection, panel visibility, layer visibility and camera framing.
It did not accept a pose, edit labels or marks, approve proposals, run refinement, or
regenerate source data. Selecting a run row loaded a foreground-only VRLOG; replay was
then paused. Acceptance and registration were consequently unavailable in that run.

The initial connection used the fixture service. Switching to local Gerardus recovered
the service connection but closed Alignment. A later reference stream failed opening
SQLite cells in the temporary data overlay. Copying those cells into a local audit
snapshot allowed the stream to complete; the NAS originals were preserved. This is an
environment-specific recovery, not evidence that the app or server handles every storage
failure correctly. A UI automation timeout during placement was not a confirmed app
deadlock: a process sample showed rendering, and the app later answered UI inspection.

There are 43 PNG evidence captures, accessibility snapshots and a manifest with full SHA-256
hashes in the local `velocity-ui-audit` evidence bundle. Capture IDs below are stable
provenance identifiers. The raw bundle, process sample and local database snapshots are
outside Git; this report remains readable without them. Native menu screenshots were
unavailable from the app capture API; View was inspected as accessibility text.
The bundle includes the operator's mismatch screenshot and the corrected build's
follow-up views. The original catalogue contains 39 captures.

The reference reported 347 building parts, 1,109 roof planes, 5,987 building walls,
204 footprints, 1,426,021 3DEP points and two registered scans containing 87,299 points.
These are server-reported inventory counts. Toggling a layer and seeing its appearance
change establishes a viewer behaviour, not positional accuracy, correct roof shape,
complete coverage, or physical validation. The source dates differ; details must remain
available when the interface is simplified.

## Flow index

| Flow                    | Entry and progression                                             | Observed result                                                                                                                 | Capture IDs      |
| ----------------------- | ----------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- | ---------------- |
| F01 Connection          | Launch → File/Reconnect → status → Servers                        | Connected; fixture recovery offered in Align; changing service closed Align                                                     | 01–04, 19        |
| F02 Survey playback     | Toolbar site chooser → dated survey → Replay                      | Searchable survey choices; selecting one starts playback                                                                        | 05–06            |
| F03 Track review        | Inspector → track row                                             | Empty detail area becomes class, history, velocity and geometry controls; no labels changed                                     | 09, 11           |
| F04 Filters             | Filters with Inspector open                                       | Horizontal strip with in-box and range controls, rather than a second side pane                                                 | 10               |
| F05 Camera              | Camera → Orbit/Fly                                                | Numeric orbit controls and viewpoint actions; no viewpoint saved                                                                | 13               |
| F06 Game                | Game on → Fly controls → Game off                                 | Visible throttle/turn/brake/exit instructions; motion controls were covered in the earlier functional fix, not re-measured here | 14               |
| F07 Align preparation   | Align → matching site → Place from hint → reference stream        | Hint pose could exist while reference failed; retry succeeded after local storage repair                                        | 07–08, 12, 21–22 |
| F08 Building inspection | Align layers → isolate Buildings → Slope/Shape → off/on           | Roof appearance changed; colour choice survived off/on                                                                          | 22–28            |
| F09 City comparison     | City Tiles off independently → City mode on/off                   | City Tiles and City mode are separate; City mode suppresses site roofs while Buildings remains checked                          | 25–30            |
| F10 Alignment selector  | Choose site → bounded searchable list → Close                     | Close worked; duplicate human labels remained; no site changed in this probe                                                    | 31–32            |
| F11 Street painter      | Align → Paint streets → return                                    | Top-down task replaces Align body; existing marks/proposals visible, no changes saved                                           | 33               |
| F12 Refinement          | Align → Refine site → close                                       | Additional column, two accepted scans; no solve or acceptance started                                                           | 34               |
| F13 Provenance          | Reference data details                                            | Source dates and bundle preparation available in a popover                                                                      | 35               |
| F14 Roads               | Generate road geometry entry → existing corridor → scroll → Frame | 21-patch corridor loaded; controls below long site inventory; contradictory audit/coverage text                                 | 15, 36–39        |
| F15 Run browser         | Runs → row                                                        | Modal list of 1,517 runs; clicking row loaded VRLOG and dismissed browser asynchronously                                        | 17–18            |
| F16 Annotation          | Annotation menu → Open Annotation Window                          | Separate empty window offers Generate/Open; pack editing not exercised                                                          | 40               |
| F17 Panel composition   | Open Inspector, Filters, Camera, Road and Align                   | Independent panels coexist and constrain the scene                                                                              | 16               |

The cross-app selector probe is inconclusive. Its search field remained in the
visualiser's accessibility tree after clicking Claude, but that does not establish
that a popover was visibly floating over another app. Do not turn this into either a
pass or a reproduced overlap defect. Repeat with a whole-desktop observation and a
second application during automated UI regression work.

## Building and roof checks

| Check                                           | Result                                                       | What remains unproved                                       |
| ----------------------------------------------- | ------------------------------------------------------------ | ----------------------------------------------------------- |
| Real reference loads after preparation          | Passed after local cell snapshot repair                      | Cold preparation, every site and remote server storage      |
| Buildings on/off                                | Passed; roof fill and part outlines changed                  | Geometric correctness and completeness                      |
| Slope/Shape                                     | Passed; radio state and rendered colouring changed           | Shape classification accuracy and colour accessibility      |
| Restore Buildings after switching it off        | Passed; Shape selection retained                             | Persistence across app restart and a different site         |
| Isolate from other Alignment layers             | Passed through twelve individual toggles                     | A one-action solo/restore workflow does not exist           |
| Distinguish grey city boxes from site buildings | Passed using separate Overlays → City Tiles control          | Discoverability for an unfamiliar operator                  |
| City mode round trip                            | Site display returned without re-preparing                   | Checked layers need explicit effective-visibility feedback  |
| Registration/acceptance with this VRLOG         | Correctly unavailable without settled background/full frames | End-to-end write workflow not exercised                     |
| Geometry versus the physical site               | Not tested                                                   | Requires dated reference comparison and physical validation |

## Observations to address

These are observations or source-confirmed constraints. The preferences in the next
section are proposed responses, not findings from user research.

| ID  | Priority | Observation and implication                                                                                                                                                                                                                                                                                                                                                                 | Evidence                                 |
| --- | -------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------- |
| O01 | High     | Inspector is fixed at 520 pt even with no selected track. At the inspected 1,748 pt window this is about 30% of width. Its narrow track list sits beside a largely empty detail region.                                                                                                                                                                                                     | 09–11; `ContentView` and `SidePanelView` |
| O02 | High     | Camera (330 pt), Road (340 pt), Inspector (520 pt), Alignment (340 pt) and refinement can coexist. Their nominal widths plus the 600 pt viewport minimum exceed common window sizes.                                                                                                                                                                                                        | 16, 34; `ContentView`                    |
| O03 | High     | Buildings off leaves grey city context boxes. City mode is another separate control; it suppresses the site's buildings while their checkbox stays on. Requested visibility and effective visibility are not explained together.                                                                                                                                                            | 25–30; `drawnReferenceVisibility`        |
| O04 | High     | Every reference layer arrived enabled. Roof inspection required twelve separate toggles, and city context required a menu action. Dense overlapping geometry obscured the question being inspected.                                                                                                                                                                                         | 22–28                                    |
| O05 | High     | Reference errors expose RPC/SQLite paths. A hint pose and nudge controls can exist while the reference is unavailable. The UI lacks a concise stage summary distinguishing pose ready, reference failed and registration unavailable.                                                                                                                                                       | 08, 12                                   |
| O06 | Medium   | The toolbar gives task entry, source, navigation, rendering, point size and debug similar prominence. Nine letter buttons require remembered meanings or help. Its width is already crowded at a large window.                                                                                                                                                                              | 02, 22                                   |
| O07 | Medium   | The pinned Alignment header, reviewer and nudge grid occupy substantial vertical space. Layers and roof colouring are below these, and appear only after streaming succeeds. There is no visible roof colour legend; its explanation is help text.                                                                                                                                          | 07, 22–28                                |
| O08 | Medium   | Survey playback and Alignment site selection both present “site” choices, with different inventories and consequences. The Alignment list also contains repeated `kirk static-0` labels with the same short token. Identity fixes alone do not disambiguate their meaning.                                                                                                                  | 05, 31                                   |
| O09 | Medium   | Road controls and Drive sit below a long prepared-site inventory. A selected survey and existing corridor do not remove the need to scroll through unrelated sites.                                                                                                                                                                                                                         | 36–38                                    |
| O10 | High     | While the mesh audit was pending, Road text reported 21 loaded patches but “No resident patches”. It later completed with 4,474 overlapping triangles and 1,028 seam pairs. Coverage still reported inventory 0 m, surfaced 1.96 km (0%) and inferred width 1.99 km, with a mesh-failed gap and raw coordinates. Pending-state wording and inventory denominators need separate correction. | 38–39, 42–43                             |
| O11 | Medium   | Track rows exposed as unnamed accessibility buttons in the Inspector. A sighted operator can read them, but assistive navigation and reliable automation lose their track identity.                                                                                                                                                                                                         | 09–11 accessibility snapshots            |
| O12 | Low      | Disconnected and empty Annotation states have very different guidance. Annotation offers clear next actions; the initial main workspace gives little direction.                                                                                                                                                                                                                             | 01, 40                                   |
| O13 | High     | A subsequent operator screenshot showed road surfaces rotated relative to the building reference. Source inspection found roads bypassing the inverse sensor pose used by buildings; the site's roughly 171° heading explains the near half-turn. This is a frame defect, not a styling preference.                                                                                         | Operator capture 41; road draw uniforms  |

Filters deserve a narrower claim than “large pane”: the observed implementation uses
a shallow strip over the viewport. The opportunity is to keep its active criteria and
count legible when closed, and to avoid a permanently open row when no filtering is needed.
The native View menu had Clear, Camera Pane and Enter Full Screen; toolbar crowding should
not be described as proof that the native menu bar itself contains the same controls.

## Preferences to challenge

| Proposed default                                               | Why try it                                            | Challenge / alternative                                                                     | Evaluation task                                                        |
| -------------------------------------------------------------- | ----------------------------------------------------- | ------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| One contextual panel, optional pin for comparison              | Protects scene space and makes the current task clear | An expert may need Align beside a selected track; tabs could add switching cost             | Compare roof fit against a selected track at 1,100 pt and 1,440 pt     |
| Compact resizable inspector, 300–360 pt starting width         | Avoids paying 520 pt for an empty selection           | Dense labels and plots may become too cramped; a vertical track list may need its own mode  | Label five tracks without horizontal clipping or accidental edits      |
| Named layer chooser, expert keys retained                      | Makes Buildings, boxes and city context discoverable  | Frequent toggles may be slower in a popover; permit a small pinned set                      | Isolate roofs, compare points, restore prior visibility                |
| Task tabs or rail: Tracks / Align / Roads / Camera             | Reduces simultaneous columns                          | Modes can hide unrelated active state; a tab alone must not alter playback or geometry      | Switch tasks and verify framing, selection, filters and layers persist |
| On-demand filters with count and criteria summary              | Reduces unused chrome while preserving state          | Hidden filters are dangerous; offer explicit reset and visible scope                        | Explain why a known track is absent without opening several surfaces   |
| Inspect buildings in a deliberately chosen context             | Avoids every layer competing on arrival               | Automatic defaults may surprise experienced operators; offer presets and reversible solo    | Distinguish measured points, inferred roofs and city boxes unaided     |
| Progressive details for provenance and reviewer                | Keeps the primary preparation/inspection path short   | Hiding source dates or author identity can weaken decisions; keep a concise visible summary | Identify source age and reviewer before accepting a pose               |
| Dim rather than hide filtered tracks as an optional comparison | Preserves spatial context                             | Can be visually noisy and changes current filter semantics                                  | Compare time-to-find and mistaken selections under both behaviours     |

No preference above authorises changing established single-key shortcuts. In particular,
the mockup's convenience keys are illustrative; the production key map remains the one
in [menu-layout.md](menu-layout.md).

## Design and implementation handoff

The new Claude Design project uses the existing velocity.report design system and asks
for two interactive directions at 1,440 × 900 and 1,100 × 760. Its brief includes clean
viewport, selected-track/filter, unprepared Align, loading, recoverable error,
Buildings/Slope/Shape, Roads and Game states. Mockup counts and speeds are illustrative.
The design must retain the difference between local view changes, a proposed pose and
an accepted recorded decision.

Direction A leads the exploration following the operator's preference. Direction B
remains a comparison, not a reason to keep a persistent inspector by default. The
review feedback corrects the mockup's guessed layer names, removes convenience keys
that conflict with production shortcuts, and matches the existing roof slope palette.

The road frame correction is independent of the proposed redesign: roads are shifted
from their corridor ENU origin into the prepared building reference's ENU, then both
use the same inverse sensor pose. Drawing, picking, overhead framing and driving must
use that placement together. A standalone corridor keeps its own ENU frame. Regression
cases cover a 171° heading, neighbouring origins, load order and pose nudges; this
establishes transform consistency, not physical road accuracy.

The corrected Debug build was inspected with the existing Ashbury corridor and isolated
Buildings layer: their orientation agreed in the overhead view (capture 42), and closing
the panes retained that view at full width (44). A patch-click probe did not establish
a selected patch, so interactive picking is not claimed as passed; its transformed ray
is covered by the renderer regression. The complete native suite passed with 846 XCTest
cases and 1,990 Swift Testing tests. The layout redesign itself is still a mockup and
implementation handoff, not an implemented production UI.

Implement in reviewable slices after comparing the directions:

1. **Layer clarity:** put city context, footprint boxes and inferred buildings in one
   discoverable layer surface; show effective visibility and a persistent roof legend.
   Add reversible solo/restore. Keep provenance available and avoid implying that
   inferred geometry has become physically validated.
2. **Panel composition:** replace independent visibility flags with an explicit active
   task plus optional pinned comparison. Preserve existing session objects, scene
   framing and keyboard routes. Start with a compact, resizable inspector; retain a
   large enough plot area and accessible track names.
3. **Toolbar hierarchy:** group survey/source and playback context; retain Game/Exit
   Game; move infrequent render/debug controls behind named surfaces with visible state.
4. **Preparation state:** show site match, prepare, stream, inspect and review stages;
   make retry target the failed stage. Keep layer availability visible even before
   data arrives. Add cancellation only with tested stale-response protection.
5. **Road inventory:** use the selected site's corridor first, put the full site list
   behind search, and reconcile inventory, support and resident-mesh metrics before
   styling their status.

Acceptance criteria include a useful viewport at the compact size; no clipped primary
actions; unchanged framing and playback when switching task; independent source and site
identity; an explanation for checked-but-hidden layers; no writes from layer or nudge
controls; accessible track labels; and explicit confirmation before recording decisions.

Regression coverage should combine the existing in-process gRPC contract checks with
UI flows for preparation failure/retry, partial layers, stale site responses, Buildings
off/on, Slope/Shape retention, City mode round trips, filter scope and panel resizing.
Do not replace endpoint tests with snapshots. Conversely, endpoint success alone cannot
prove that a control is discoverable, a popover dismisses, or the right layer is drawn.
Keep cross-app selector dismissal and compact-window keyboard navigation in the UI gate.

The audit did not exercise Annotation pack editing, persistent labels, cold data
regeneration, acceptance/refinement writes, every site's source completeness, restart
persistence, or a field comparison. Those remain explicit follow-up checks.
