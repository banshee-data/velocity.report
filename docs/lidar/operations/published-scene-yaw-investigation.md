# Published vehicle boxes: Lombard–Broderick and Embarcadero–Broadway

The published scenes can draw a vehicle nearly perpendicular to its own path because the web
player mirrors its stored box heading. This report records the deployed evidence, the correction,
and the separate physical-pose questions that remain.

- **Status:** Investigated; web yaw transform corrected with geometric regression tests
- **Layers:** Public web viewer, scene export, L5 published box, L9 recording
- **Related:** [Published box](published-box-rotation-2026-10.md), [box refinement](published-box-refinement-2026-10.md), [web scene export plan](../../plans/lidar-web-scene-export-plan.md)

The investigation was performed on November 13, 2026. No replay or capture-volume access was
possible. These assets describe inferred geometry, a form of measured spatial data; see
[geometry priors: data purpose](../../geometry-priors-data-purpose.md).

The principal finding is a confirmed sign error in the deployed web player's yaw
transform. It mirrors box orientation while positions and paths retain the
correct handedness. This is sufficient to reproduce nearly perpendicular boxes
on the two published scenes without any error in the stored heading.

## Evidence and reproducible examples

The old `/scenes/<site>/` pages redirect by HTML refresh to `/surveys/<site>/`.
Fetched the public survey HTML, its exact versioned JavaScript modules, both
part headers, and the example chunks below. The deployed module is
`/modules/c8fdfc06e3/js/scene-player.js`. The player, both headers and both example
chunks were byte-identical to the investigation checkout before the correction. This establishes deployed-data
evidence independently of the unavailable capture volume.

No interactive browser surface was available. This investigation did not watch a
WebGL playback or take a browser screenshot. It executed the actual TrackVisual
class with real Three.js scene-graph objects and examined its world matrix, and
read the deployed scene frames directly. The world-matrix checks establish rendering geometry, not physical vehicle pose.

All times below are relative to the first retained scene frame, in seconds.
Track IDs are export-local and must be paired with the asset provenance.
Angles are folded modulo 180 degrees into 0–90 degrees for an undirected box
axis; a front/rear reversal is not counted as a sideways box.

| Site                    | Track | Inspect window                                            | Exact sample              | Frame | Chunk                  | Stored yaw vs velocity | Deployed drawn axis vs velocity |
| ----------------------- | ----- | --------------------------------------------------------- | ------------------------- | ----- | ---------------------- | ---------------------- | ------------------------------- |
| Lombard at Broderick    | `gi`  | +104.608 to +111.608 s; most obvious +108.008 to +111.608 | +109.807450 s (01:49.807) | 61569 | `chunk_0001.ndjson.gz` | 6.46°                  | 70.77°                          |
| Embarcadero at Broadway | `1v`  | +28.619 to +43.819 s; most obvious +35.219 to +38.219     | +36.819015 s (00:36.819)  | 12465 | `chunk_0000.ndjson.gz` | 1.78°                  | 88.71°                          |

Public entry points:

- https://velocity.report/surveys/lombard-broderick/
- https://velocity.report/surveys/embarcadero-broadway/
- https://velocity.report/surveys/lombard-broderick/assets/part-000/frames/chunk_0001.ndjson.gz
- https://velocity.report/surveys/embarcadero-broadway/assets/part-000/frames/chunk_0000.ndjson.gz

Lombard sample: `bx=9.56, by=-47.15`, `vx=-3.53, vy=-4.42`, speed 5.66 m/s,
`bh=-2.132` rad (−122.15°), `l=4.38, w=1.60` m. Velocity points −128.61°.
Its path curves from course −173.30° at +106.007 to −152.86° at +108.008 to
−128.61° at +109.807. At the middle sample, the centred box-centre displacement
over approximately ±0.4 s points −128.44°: stored axis discrepancy 6.29°, deployed
axis discrepancy 70.60°. Capture timestamp: September 2, 2026, 09:40:12.027 PDT
(`2026-09-02T16:40:12.026670Z`).

Embarcadero sample: `bx=-24.14, by=0.71`, `vx=-3.54, vy=3.57`, speed 5.03 m/s,
`bh=2.383` rad (+136.54°), `l=4.24, w=1.76` m. Velocity points +134.76°.
Its course changes from +115.89° at +33.019 to +134.76° at +36.819 to +177.65°
at +40.019, a turn-shaped path. At the selected sample the centred box-centre
displacement points +140.23°: stored discrepancy 3.69°, deployed discrepancy
83.24°. Capture timestamp: September 2, 2026, 15:23:26.486 PDT
(`2026-09-02T22:23:26.485960Z`).

These additional path calculations strengthen the renderer diagnosis; they are
still derivatives of inferred box centres, not independent physical trajectories.
Neither sampled export exposes whether a row is matched or coasting.

## The transform error

`public_html/src/js/scene-coords.js:8` maps a data point `(x,y,z)` to Three.js
`(x,z,-y)`. A heading ψ along the length axis therefore maps to
`(cos ψ, 0, -sin ψ)`. Three.js positive Y rotation of the model's +X length axis
already gives exactly that vector.

Before the correction, `public_html/src/js/scene-player.js` set the Y rotation to the
negated value of `bh`, falling back to `hdg`, then zero.

This produces `(cos ψ, 0, +sin ψ)`, equivalent to −ψ in the data's XY plane.
Its comment asserting that negation was needed was incorrect for the coordinate
mapping used here. Positions, trails and box centres are not mirrored, so the
box turns the opposite way to the path. The mismatch vanishes on X/Y-aligned
undirected axes and reaches 90° near diagonal headings. This explains why it can
look acceptable on a straight segment and suddenly sideways in a turn, and why
Embarcadero is affected much more broadly than Lombard.

Executed actual TrackVisual with bh=π/4: its local +X length direction becomes
`[0.70710678,0,+0.70710678]`; transformed data heading should be
`[0.70710678,0,-0.70710678]`. Repeated with the two real sample headings. This reproduction used the unmodified renderer before the correction. The yaw line dates to commit `2c6191ae6a` in
September, before the published box model was added.

Before the correction, the existing scene-coordinates and scene-player test files passed all 41 tests.
They test centres, height, trails and coordinates but do not assert nonzero box
yaw against the transformed heading vector. Passing them does not validate this
orientation contract.

## Wider footprint in checked-in published assets

Read all checked-in frames for each site, retaining classifications car, truck
or bus and instantaneous speed ≥3 m/s. Each row is one exported track-frame,
not one physical vehicle; sampling is about 5 Hz. Compared stored `bh` with
`atan2(vy,vx)`, then compared the deployed equivalent axis `-bh` with the same
course. No smoothing, fitting or replay was performed.

| Site                 | Moving track-frames | Median stored/course error | Median deployed/course error | Stored >45° |   Deployed >45° |
| -------------------- | ------------------: | -------------------------: | ---------------------------: | ----------: | --------------: |
| Lombard–Broderick    |              54,803 |                      1.12° |                        9.82° |  7 (0.013%) |     986 (1.80%) |
| Embarcadero–Broadway |              28,262 |                      1.30° |                       45.71° | 32 (0.113%) | 15,980 (56.54%) |

These full-scene statistics use the checked-in asset tree. Deployed headers and
the two example chunks were verified, not every remotely served chunk. They
measure consistency with velocity, which also observes the box heading in the
estimator; they are not physical-yaw accuracy or vehicle prevalence estimates.

## Pipeline and export trace

1. L1 packet decode and L2 frame assembly produce points in the stationary
   sensor's axes; L3 foreground and L4 clustering/rectangle fitting produce
   geometric observations. L5's Kalman state estimates position and velocity.
2. `internal/lidar/l5tracks/tracking_metrics.go:341` defines kinematic heading
   as `atan2(VY,VX)`. This is course, not a separately measured body yaw.
3. `internal/lidar/l5tracks/box_model.go` estimates a published box separately:
   rectangle-fit axis, quarter-turn axis/length labelling, course observation
   above 1 m/s, heading/yaw-rate filter and speed-dependent yaw-rate bound;
   extent memory and point containment place its centre. Body yaw can differ
   from filtered course during a turn, at low speed or with weak support.
4. `internal/lidar/l9endpoints/adapter.go:488` publishBox overwrites the legacy
   OBB yaw/extents/centre with BoxHeadingRad/BoxLength/BoxWidth/BoxX/BoxY when
   present. Position and velocity stay the tracked state. These published
   assets already have `bx/by` and the box model enabled in their build's
   defaults; diagnosing them as just the old 8%-EMA OBB is incorrect.
5. `internal/lidar/l9endpoints/recorder/proto_codec.go:137` and `:322` encode
   and decode heading and bbox heading independently, without a sign change.
6. `internal/scene/export.go:464` exports course as `hdg`, box yaw as `bh`,
   both rounded to 0.001 rad; centres, dimensions and velocity to 0.01 units.
   No yaw transformation is applied. Both headers say `cv_kf_v1/online`, so
   optional RTS trail smoothing is not implicated. Even for the smoother,
   yaw/box centre are independent of the substituted x/y/vx/vy state.
7. TrackVisual draws length along local X, width along local Z, height along
   local Y, using bx/by when present and `bh` preferentially. The yaw negation
   is introduced at this final step. No evidence of a degrees/radians swap,
   length/width export swap, or heading/velocity field interchange was found.

The native visualiser uses `rotationZ: boxHeading` in sensor XY rather than
this Three.js transform (`MetalRenderer.swift:1276`). The web finding does not
establish an equivalent native yaw-sign defect. Native code also uses zero as
an unavailable-heading sentinel (`bboxHeadingRad != 0`), unlike web's nullish
fallback; exact zero is a valid angle. This is a separate narrow review/test
candidate, not the cause of the two nonzero-yaw examples above.

## Frames, timestamps and provenance limits

Both published headers:

- `build_version=0.5.1-pre50`, `build_git_sha=57e9e9f1a3d5e6c43b36371aafbdcc6c4099cefb`.
- `tuning_hash=sha256:c927ea3afe40a1be0fb1812083763d05a37269c7bfe6fe1098f91866fdec570d`.
- `frame_stride=2`, angles radians, distances metres; `evidence.tracks=inferred`.
- Frame time is relative microseconds; absolute sample time is header start_ns
  plus t×1000. It is not wall time at publication or the original frame number/10.

Lombard: 6,009 frames, 1,201.60730997 s, generated `2026-11-02T00:26:08Z`,
source VRLOG SHA-256 `5aa809c9f33438785b8695e915c2bf35ed81113016824b3444363fac47ee6a7e`.
Embarcadero: 6,048 frames, 1,209.41569197 s, generated `2026-11-01T23:35:36Z`,
source VRLOG SHA-256 `07371903ef524a2a70377b4c239e186ec4cdb3e0351e2c0a80e9a7eacb737f0d`.
The source-VRLOG identifiers were read, not recomputed without those recordings.
DEVLOG's #903 entry records rebuilding the other 23 scenes at 57e9e9f1 in
analysis mode with settling. Current publisher defaults alone should not be
used to infer that historical invocation.

The headers call the frame `site/hesai-pandar40p`, reference `ENU`, while their
placement files explicitly state that scene data remains in sensor coordinates.
The latter agrees with scene-vectors.py, which inverse-transforms site geometry
into sensor space, and scene-horizon.js, which preserves sensor heading when
levelling. Treat the coordinate headers as imprecise provenance, not proof of
surveyed geodetic east/north. A wrong site registration could move roads relative
to tracks, but cannot explain a box axis mirrored relative to its own path.
No placement correction is warranted by the yaw finding.

The pages have no point-cloud overlay for these two scenes. Compact track JSON
does not preserve member returns, covariance, match/coast state, yaw ambiguity
or heading-source diagnostics. It cannot establish physical sideslip, correct
identity, containment of actual vehicle returns, or true body pose.

## Relationship to existing work

- `lidar-heading-coherence-sprint-plan.md`: genuine legacy OBB lock/guard issues,
  bounded release and default-off axis candidate; course alignment is explicitly
  a proxy. Relevant history, not sufficient explanation of already-published
  box-model assets.
- `solid-body-rectangle-heading-2026-10.md` and the geometry-convergence plan:
  replace unsupported heading beliefs with rectangle observations; note partial
  views, quarter-turn ambiguity and identity/containment coupling. They do not
  establish a render-coordinate contract.
- `published-box-rotation-2026-10.md` (geom0005) and
  `published-box-refinement-2026-10.md` (geom0006–10) are directly relevant.
  The latter made the box model default and reports residual turn lag 3.8°,
  reduced only to 3.5° by a failed course-leading arm. Low-speed length-axis
  ambiguity, merged support and short trucks remain. These are plausible
  residual estimator problems after correcting the renderer, not a reason to
  reinterpret an 89° renderer error as vehicle dynamics. The old report's
  opening assertion that web and native draw the same box needs qualification:
  they consume the same published fields but the web transform mirrors yaw.
- `scene-variants-kirk0-2026-11.md` (geom0014) reads exported fields; the shadow
  matches defaults exactly, repeat-default exports match, and driving variants
  turn less (median 0.23–0.34° per frame versus 0.80°). Those measurements do not
  test the screen-space yaw sign. Absolute heading-change magnitudes even stay
  invariant under this mirroring, so its result can be true while playback is
  visually wrong. That is not evidence to adopt a convergence candidate here.
- The state-estimation and Swift physical-pose plans require independent physical
  references and observed-yaw bounds. Follow those gates for estimator acceptance;
  do not score the course observing itself as physical heading truth or jump to
  an advanced motion model to address a confirmed presentation error.

## Correction and bounded next work

The player now uses the positive published yaw with the existing coordinate mapping.
`scene-box-orientation.test.mjs` constructs expected corners by rotating in data XY and then
transforming the points into scene axes, independently of the player's Euler rotation. It checks
both fill and edges, positive and negative oblique headings, wrapping, zero, and legacy fallback.
The two existing published chunks supply complete turn sequences and non-sequential frame visits,
without copying raw captures or introducing a network dependency. All 16 new tests pass.

The correction changes only the displayed orientation. It leaves scene bytes, track position,
box centres, extents, speeds, and timestamps untouched. It does not alter the native visualiser
or claim physical-heading acceptance. A new site build versions the changed JavaScript; the
source scenes need no replay or re-export. Browser visual confirmation remains outstanding.

Validation: `make test-public-html` passed all 313 tests, including the 16 new orientation
tests. `make build-docs` completed. Both built survey pages selected the corrected player under
`/modules/dd39272dff/js/scene-player.js`, and the two example chunks in the built site matched
their source bytes. These are local build and geometry checks, not deployment or browser evidence.

The follow-up sequence is:

1. Use the existing example chunks to compare a high-oblique and overhead view
   before/after; preserve positions, speeds, centres, dimensions, frame times and
   export bytes. Recheck a straight segment near an axis and one near a diagonal.
   Serve the changed module under a new content version and verify the public
   HTML selects it. Do not regenerate a day of scenes for a viewer-only defect.
2. Once reconnected: first locate the two exact source recordings/manifests and
   verify recording/build/config fingerprints. Before replay, inspect the old
   VRLOG at the identified windows with returns, memberships and matched/coast
   diagnostics; distinguish true observations from predicted tails. Old export
   local IDs are not stable identities across a new replay.
3. Replay just those turns plus warm-up/settling context in an isolated database
   at current main and, if reproducibly available, the frozen publication build.
   Run from checked-in parameter sets only; begin with defaults, no parameter
   search. Record source hashes, segment boundaries, exact retained times,
   run/build/config identities, drops and settling. Compare VRLOG -> JSON ->
   web and native corners at the same frames before judging tracking.
4. Independently annotate a small set of observable yaw/anchor keyframes across
   entry, apex and exit, plus a straight control and a low-speed/partial-view
   case at each site. Mark ambiguity/occlusion and assisted origin honestly.
   Compare stored box yaw, filtered velocity course, centred displacement course,
   and physical yaw bounds separately. Report lag, signed lateral discrepancy,
   axis labelling, centre/extent error and identity continuity. Consider estimator
   experiments only for reproducible residuals that survive viewer parity.

Confidence: very high in the deployed web sign error and its effect on the two
exact examples; high that it is the dominant broad sideways appearance at
Embarcadero; no physical-pose conclusion from compact published assets alone.
Current-main replay behaviour and native end-to-end playback remain unverified.

## Frozen fetched-byte fingerprints

These are the bytes read during investigation, before the renderer correction. The original
source recordings were unavailable; the source-VRLOG hashes above are declared provenance.

| Artifact                           | SHA-256                                                            |
| ---------------------------------- | ------------------------------------------------------------------ |
| Versioned deployed scene-player.js | `2125467511c6be14093bdc588463e645d1ddf1dd3662f5e66754c10cf6b9817d` |
| Lombard header                     | `b4f84c07c9ec51d7413c666a2065ec0fa6c0a8847193b015fdc10556759ce7d8` |
| Embarcadero header                 | `27e4df861f296d9b87da3dc5f9072b14e438b2c351a2fdfba0d67ec0c3ba3aa3` |
| Lombard chunk 0001                 | `7be192c9dbb9303dc5f128c220dad2231337ba0c9560596bc081beee3f3e1d9d` |
| Embarcadero chunk 0000             | `15d8952f882534fb8ba6a169db37fc99f2b09c13b22805d6517ded8b2ef40cfc` |

Investigation checkout: `3169005ac1b9fa5e9473d479e83296431be59abd`. The fix was made in a
separate checkout from main at `64a47d630`. Existing uncommitted research work was preserved.
No physical validation or current-main replay claim is made.
