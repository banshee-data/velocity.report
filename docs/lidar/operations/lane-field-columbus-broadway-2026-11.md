# Where the cars ran: the first lane field, on Columbus Ave at Broadway

Phase A of the [lanes from tracks plan](../../plans/lidar-lane-proposals-from-tracks-plan.md) asks
for one real check before the field goes anywhere: build it from a real run and see whether its
vehicle ridges fall on the streets. This report builds the lane field of the first ten minutes of
the columbus-broadway capture, places it with the site's accepted Gerardus pose, and measures where
the ridges sit across the City's Columbus Ave and Broadway centrelines, per direction and in each
half of the run.

- **Status:** Measured, 2026-11-03, on a Mac; nothing adopted, since nothing is proposed. The field
  was uploaded to a local Gerardus test server only, not to arrow's
- **Layers:** L8 (`BuildLaneField`), the run route and `velocity lidar lane-field`
- **Related:** [lanes from tracks](../../plans/lidar-lane-proposals-from-tracks-plan.md),
  [lane painter](../../plans/lidar-visualiser-lane-painter-plan.md),
  [Geometry priors: data purpose](../../geometry-priors-data-purpose.md)

The field and the ridges are measured spatial data: where tracked vehicles were observed during
ten minutes on September 2, 2026, counted per 0.25 m cell. They are not a survey of the street and
say nothing about lanes no vehicle used in that window.

## Summary

- **The ridges fall on the streets, on the right-hand side of travel.** Across Columbus Ave the
  north-west-bound ridge sits 2.0 m right of the centreline and the south-east-bound ridges 3.0
  and 5.0 m to the other side; across Broadway the westbound ridges sit 2.0 and 4.5 m right of
  the centreline and the eastbound 1.5 and 4.0 m to the other side. Each ridge's heading agrees
  with its side: 980 of 1,212 heading counts on Columbus's north-west ridge, and 2,023 of 2,214 on
  Broadway's eastbound ridge, lie along the expected direction.
- **The halves agree to the bin.** Every ridge found in the whole run is found in both halves
  within one 0.5 m bin (Columbus: −1.5 and −2.0 m; +2.5 and +3.0 m; +5.0 m in both. Broadway:
  −4.5 and −2.0 m in both; +1.5 m in both; +3.5 and +4.0 m).
- **Most of the field lies on these two streets.** Within ±8 m of the centrelines and 40 m of
  the sensor lie 77 % (Columbus) and 88 % (Broadway) of the track-cells counted within ±15 m.
- **Gerardus accepts the files.** Gerardus main at `e9dc3e0` reads all three with
  `lane_fields::from_json`, the code `gerardus upload-lane-field` calls, and stores them over
  `UploadLaneField` in chunks against a manually placed record of the same sensor; a repeat in
  other chunks answers `already_stored` with the same field id, and `GetLaneField` returns the sum.
- **Analysis replays store no track observations.** The run is an analysis replay, as every
  archive and scene run is, and those never write `lidar_track_observations`
  (`DisableTrackPersistence`); the first build from the store was empty. The field is now read
  from the run's recording when the store has none: the frames' tracks with no miss, the run's
  own confirmed tracks, their final class.
- **From the recording the route is slower than the plan's second.** The whole run's field took
  3.2 s over HTTP and 8.0 s by the CLI (each half about 2.5 s); a run that stored its observations
  would read them from SQLite. Phase B's one-second criterion holds only for that case.

## Method

**Capture and run.** `20260902T1320_80858-0f4_columbus-broadway.pcapng` (4,903,944,004 bytes),
copied from arrow's `sf-street-speeds/raw/lidar/`. One analysis replay of its first 600 s on a
scratch server (`speed_mode` analysis, settle before recording), at the defaults
([`tuning.defaults.json`](../../../config/tuning.defaults.json), fingerprint `ab0c4035780d`): run
`69e7a647-c679-40d2-a2b5-263a0d2a95e3`, 12,002 frames, 599.9 s, 2,719 confirmed tracks
(640 car, 11 bus, 138 cyclist, 747 pedestrian, the rest bird, `dynamic` or unclassified).

**Fields.** `GET /api/lidar/runs/{run_id}/lane-field?part=…` and `velocity lidar lane-field` at
`e86b23638`, the three parts, byte for byte the same from both:

| Part        | Tracks | Grid      | Vehicle cells | Cyclist cells | Pedestrian cells | Largest vehicle cell (tracks, box) |
| ----------- | -----: | --------- | ------------: | ------------: | ---------------: | ---------------------------------- |
| all         |    688 | 529 × 545 |        34,184 |         2,187 |           10,777 | 17, 89                             |
| first_half  |    361 | 446 × 493 |        27,072 |           819 |            4,382 | 10, 44                             |
| second_half |    348 | 412 × 529 |        25,340 |            45 |            5,123 | 10, 46                             |

688 of the 2,719 tracks count: the rest are unclassified or `dynamic`, or have fewer than ten
observed instants, or are vehicles that never reached 2 m/s. A car's centre crosses a given 0.25 m
cell's centre rarely, so most cells are kept by `box_tracks` rather than `tracks`.

**Placement.** The site's newest accepted record on arrow's Gerardus,
`gms:01M5GKWEZRV8YEVPSRTA1YGE6Y` (`REGISTERED`, sensor `hesai-pandar40p`, L16 `808580f47`):
translation (−14.106, −41.894, 23.285) m and rotation quaternion (0.3128, −0.0071, 0.0223,
−0.9495), the sensor's +Y bearing 143.5° in the site frame. Cells and heading bins were placed in
the plane as Gerardus's `place` does. Centrelines are the `city_streets` layer in `frame=site`.

**Cross-sections.** Stations every metre along each street's centreline within 40 m of the sensor
(81 on Columbus Ave, 77 on Broadway); each vehicle cell within 0.5 m along a station and 15 m
across it is binned by its signed offset (0.5 m bins, positive to the left of the centreline's
digitised direction: 311° for Columbus, 261° for Broadway). A ridge is a local maximum of the
three-bin smoothed `tracks` profile holding at least a tenth of the largest. Each bin's heading
counts are resolved along and across the centreline.

## Findings

Whole run, vehicle group:

| Street       | Offset (m) | Tracks |    Box | Heading along / across | Direction        |
| ------------ | ---------: | -----: | -----: | ---------------------- | ---------------- |
| Columbus Ave |       −2.0 |  1,187 | 19,181 | 980 with / 232         | north-west-bound |
| Columbus Ave |       +3.0 |    663 | 17,674 | 558 against / 183      | south-east-bound |
| Columbus Ave |       +5.0 |  1,221 | 13,669 | 1,066 against / 267    | south-east-bound |
| Broadway     |       −4.5 |    843 | 14,364 | 666 with / 87          | westbound        |
| Broadway     |       −2.0 |    778 | 19,789 | 613 with / 79          | westbound        |
| Broadway     |       +1.5 |  2,180 | 29,240 | 2,023 against / 191    | eastbound        |
| Broadway     |       +4.0 |    777 | 12,633 | 616 against / 88       | eastbound        |

Columbus's profile also has small peaks from −14 to −10 m and at +13 m whose headings run across
the street (99 to 110 across against 9 to 18 along): Broadway's traffic crossing Columbus's
stations near the intersection, not a lane of Columbus.

By half:

| Street       | Direction        | Whole run  | First half | Second half |
| ------------ | ---------------- | ---------- | ---------- | ----------- |
| Columbus Ave | north-west-bound | −2.0       | −1.5       | −2.0        |
| Columbus Ave | south-east-bound | +3.0, +5.0 | +2.5, +5.0 | +3.0, +5.0  |
| Broadway     | westbound        | −4.5, −2.0 | −4.5, −2.0 | −4.5, −2.0  |
| Broadway     | eastbound        | +1.5, +4.0 | +1.5, +3.5 | +1.5, +4.0  |

## Interpretation

The two streets' four directions each hold traffic on the side right-hand traffic puts them, with
headings that agree, and two ridges per direction where the street has two lanes' worth of width.
The halves agree to the half-metre bin, which is what Gerardus's two-window test (0.20 m) will ask
of its fitted lines on finer evidence. That is enough for Phase A: the field is placed where the
streets are, so Gerardus has something to fit.

## Limitations

- **One site, one window, one pose.** Ten minutes of one afternoon at one intersection, placed by
  one accepted pose. A pose error would move every ridge together; this check cannot tell a 0.5 m
  pose error from a 0.5 m near-side bias of the sensor.
- **Offsets are from the City's centreline, not from paint or kerbs.** The centreline is the
  City's record of the street, and the offsets say where traffic ran relative to it, not where a
  lane line is. Phase E compares fitted lanes with lines drawn in the painter.
- **Half-metre bins and a simple peak rule.** The ridges are local maxima of a smoothed profile;
  two lanes closer than a metre would read as one, and the rule was chosen before looking but is
  not Gerardus's fit.
- **The halves are not independent captures.** They are two five-minute windows of one visit, with
  the same weather, light and sensor; agreement between them is weaker evidence than agreement
  between visits.
- **No plot.** The plan asks for a top-down plot by eye, drawn by the render tool
  (`velocity lanes render`, snapshot plan Item 4) and published to the Hugging Face dataset; that
  tool is not built yet, so this report carries numbers only. Nothing drawn was committed (D-39).
- **Not uploaded to arrow.** The files were checked against a local Gerardus test server built
  from main; Phase C's Send lane field stores them on arrow.

## Provenance

| Item                 | Value                                                                                                       |
| -------------------- | ----------------------------------------------------------------------------------------------------------- |
| Capture              | `20260902T1320_80858-0f4_columbus-broadway.pcapng`, 4,903,944,004 bytes                                     |
| Replay build         | `0.5.1-pre50+1b0456c94` (the lane field's first commit; perception as main `39338d95b`)                     |
| Field build          | `e86b23638`                                                                                                 |
| Tuning               | `config/tuning.defaults.json`, fingerprint `ab0c4035780d`; run params hash `sha256:c927ea3a…`               |
| Run                  | `69e7a647-c679-40d2-a2b5-263a0d2a95e3`, frames 2026-09-02T20:20:36.986Z to 20:30:36.890Z                    |
| Field ids (Gerardus) | first half `sha256:041606af…`, second half `sha256:3808c1a3…`                                               |
| Gerardus             | main `e9dc3e0` (banshee-data/gerardus#68) for the upload check; arrow's server for the pose and centrelines |
| Pose record          | `gms:01M5GKWEZRV8YEVPSRTA1YGE6Y`, site `808580f47`                                                          |

The fields, the run database and the recording stay in the scratch area on the Mac
(`~/vr-scratch/lanes-20261103/`); none is committed.
