# All included scene trails: an untrimmed inventory

The owner asked for the survey bound to include all the trails in its window, without trimming
at a selected radius. This inventory reads every indexed track sample in the checked-in scene
exports and records their extent; the [lane export plan](../../plans/lidar-survey-lane-exports-plan.md)
uses the full set as its input.

- **Status:** Complete for the indexed scene exports in this checkout; not a replay of the original recordings
- **Scope:** 25 scenes, all 25 indexed parts, all classes and support states
- **Related:** [earlier radius screen](survey-lane-range-review-2026-11.md), [trail support in the scene player](trail-support-cloud-2026-11.md)

## Method

Run `python3 tools/s2-archive/scene-trail-inventory.py`. For each scene manifest, the tool reads
all parts, every indexed compressed chunk and every track sample in every frame. It uses the
player's horizontal trail anchor: `bx, by` when both are finite, otherwise `x, y`. It includes
all object classes, all speeds, and all recorded or unknown support states. It applies no distance,
percentile, minimum-track-count or temporal-history filter within the exported part. The six
radii in the comparison below count what would fall beyond each threshold; they do not remove it.

The tool checks every indexed chunk count against its index and every part's frame count against
its header. It refuses missing or duplicate track identifiers in a frame and non-finite or missing
anchors rather than silently omitting them. Track identifiers are counted within each scene part;
a repeated ID in a different part is counted separately. The tool prints aggregate counts only,
not IDs, individual trajectories or coordinates.

These are **all samples in the selected scene assets**, not all observations from the original
survey. Every part's header reports `frame_stride: 2`; the source recordings and observation
stores are unavailable in this workspace, and all 3,237,075 exported samples have unknown support
in these files. They may include predicted or coasted estimates. This inventory therefore cannot
classify the samples as physically observed or establish a true full-recording bound. It also does
not decide which samples match a lane or whether an object was tracked correctly.

## Findings

The 25 scene parts contain **157,685 frames**, **3,237,075 track samples** and **104,314 part-local
track IDs**. The furthest stored horizontal anchor is **210.64 m**, in soma1. The current player
uses the box-centre anchor for 3,176,969 samples and the position fallback for 60,106. The tool
encountered no invalid anchors, missing or duplicate per-frame IDs, or index/header count mismatches.

| Distance from sensor | Scenes with samples beyond | Part-local IDs with samples beyond | Samples beyond |
| -------------------: | -------------------------: | ---------------------------------: | -------------: |
|                 60 m |                         25 |                             17,737 |        235,462 |
|                 70 m |                         23 |                             11,617 |        138,688 |
|                 80 m |                         22 |                              7,717 |         78,882 |
|                100 m |                         19 |                              2,871 |         20,750 |
|                150 m |                         12 |                                 83 |            633 |
|                170 m |                          3 |                                 13 |             53 |

Distances are strictly greater than each threshold. An ID can occur in several rows, and one
object can have more than one ID; these counts are not independent traffic counts. A track sample
need not form a visible line segment: the player separates gaps and masks building crossings.

There are also existing display limits. `scene-vectors.py` uses a 90 m vertex-based feature
selection, and `scene-fade.js` fades scene geometry between 150 m and 170 m using 3D distance.
The player normally reconstructs five seconds of trail history and caps a trail buffer at 2,000
points. Those limits are separate from the data inventory and must be reviewed for an untrimmed
view: 53 stored anchors already exceed 170 m in horizontal distance alone. The presence of a
sample does not establish that a player currently renders it as part of a visible trail.

### Per-scene extent

The maximum is the farthest stored XY trail anchor from the sensor, not the lane length, a sensor
specification or proof that the disc inside it was observed.

| Scene                 | Frames | Samples | Part-local IDs | Maximum (m) | IDs beyond 100 m |
| --------------------- | -----: | ------: | -------------: | ----------: | ---------------: |
| 1st-mission           |   6008 |  324239 |           6644 |      144.39 |               67 |
| 3rd-folsom            |   6008 |  164379 |           5376 |      157.31 |              107 |
| ashbury-downey        |   6137 |   28667 |           1560 |      107.77 |               40 |
| broadway-franklin     |   5588 |   58518 |           1751 |       98.85 |                0 |
| bush-powell           |   6009 |  129710 |           4210 |      117.27 |               26 |
| california-leon-baker |   6261 |   33774 |            951 |       64.24 |                0 |
| columbus-broadway     |  10165 |  358754 |           8994 |      153.77 |              116 |
| columbus-north-point  |   5519 |   72511 |           2201 |      169.58 |              150 |
| embarcadero-bay       |   4776 |  148040 |           3109 |      155.83 |              114 |
| embarcadero-broadway  |   6048 |  187209 |           5299 |      145.42 |              175 |
| embarcadero-bryant    |   6813 |  220403 |           9439 |      184.63 |              455 |
| embarcadero-folsom    |   6064 |  150883 |           4669 |      159.05 |              170 |
| franklin-mcallister   |   6065 |  176212 |           4579 |      135.97 |                4 |
| fulton-divisadero     |   6049 |   93896 |           2889 |       92.67 |                0 |
| haight-divisadero     |   5775 |   32949 |           1042 |       65.92 |                0 |
| howard-6th            |   6008 |   96359 |           2971 |      157.89 |               68 |
| hyde-ofarrell         |   6005 |   90098 |           2032 |       88.36 |                0 |
| laguna-eddy           |   6554 |   55862 |           1664 |       71.86 |                0 |
| lombard-broderick     |   6009 |  149305 |           6433 |      164.63 |              430 |
| lombard-laguna        |  11638 |  242609 |          11378 |      179.09 |              522 |
| marina-broderick      |   6011 |   68039 |           1908 |      158.48 |              142 |
| marina-webster-beach  |   6010 |   48711 |           1811 |      161.64 |              165 |
| pierce-haight         |   6507 |   64897 |           1529 |      100.17 |                1 |
| soma1                 |   3315 |   60106 |           5719 |      210.64 |               18 |
| van-ness-sacramento   |   6343 |  180945 |           6156 |      143.86 |              101 |

## Implications for the export

Use every included source trail to build the local survey extent; do not cap it at 60, 70, 80,
100 m, a percentile or a minimum count. Keep support and uncertainty as attributes rather than
pretending every included estimate is observed. Any conservative search envelope must contain every
input trail; the envelope itself does not prove that all of its interior was covered. Preserve the
complete saved lane geometry associated with the survey, splitting at road segment boundaries only
when the fragments can be reassembled without loss. Record ambiguous and unmatched associations for
review instead of forcing or silently discarding them.

The survey input should eventually come from the complete, pinned local observation/recording
window, with counts reconciling the input to the computed extent. Test the exact same extent and
lane product in the web and game adapters. No raw trails or IDs are added to those exports; this
inventory's aggregate report does not change the existing publication boundary.

## Provenance

| Item                 | Value                                                                                                                                                                                                                                                |
| -------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Input                | Every checked-in `public_html/src/scenes/*/assets/manifest.json` and its referenced headers, indexes and frame chunks: 609 files. The manifest's additional foreground clip and background are separate products and are not counted as part tracks. |
| File-set fingerprint | SHA-256 `34c90152de87c1e2dc4c63ac78dcf4a3afde47ba479f852e70c2d9d2b071c85a`: sorted paths relative to the scenes directory, each UTF-8 path followed by NUL and the binary SHA-256 of that file.                                                      |
| Export provenance    | The 25 headers each name their own source VRLOG digest. Twenty-four name build `0.5.1-pre52` and `cv_kf_v1/online`; soma1 names `0.5.1-pre31`, does not name a git SHA or estimator, and uses the fallback anchor for all its samples.               |
| Reproduction         | `python3 tools/s2-archive/scene-trail-inventory.py`; the tool accepts scene IDs or `--scenes-dir` for another set.                                                                                                                                   |
| Boundary             | No PCAP, original VRLOG, current Gerardus lane-field store or capture database was available for this measurement. No raw result was written into the repository.                                                                                    |

The earlier replay summaries use different cases, windows and builds and are not directly
comparable to these scene exports. This inventory records all included samples even when a
long-range estimate is noisy; it does not turn those estimates into verified observations.
