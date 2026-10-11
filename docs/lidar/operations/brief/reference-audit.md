# Paper reference audit

- **Scope:** Paper and research-report references in `docs/lidar/operations/brief/` and `data/`, including proposals, experiment notes, source comments, author-and-year mentions, and named model repositories.
- **Status:** All identifiable works have BibTeX entries. Two generic post-encroachment-time study mentions remain unresolved.
- **Bibliography:** [data/maths/references.bib](../../../../data/maths/references.bib).

The October 10, 2026 audit added 62 entries to the existing 94, bringing the bibliography to 156.
The tables below map the additions to the names used in the source documents. Published papers
and their preprints share one entry; a different report or paper receives its own key. Papers
recorded as unsuitable or not adopted remain part of the bibliography.

Titles, authors, publication years, and identifiers were checked against arXiv, publisher-supplied
Crossref records, official proceedings, author-maintained model repositories, and institutional
report pages or cover sheets. This establishes bibliographic identity. It does not validate a
paper's numerical claims, reproduce its experiments, or establish suitability for the deployed
sensor. The briefings' reading and **Confirm** statuses remain the record of those limits.

## Corrected source context

The October 11, 2026 follow-up traced all six sources corrected in PR #739 through the
tracked repository, searching citation keys, authors, titles, and source names. No executable
source file cites these six entries directly. Their uses are architectural references and
future L7 design context; the links below identify the affected documents.

| Stable BibTeX key      | Corrected source and evidence                                                                                                                                                        | Context after review                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Dames2017`            | [Dames's MRS 2017 paper](https://sites.temple.edu/pdames/files/2017/10/DamesMRS2017.pdf), abstract and §§ II–III                                                                     | [L7 plan](../../../plans/lidar-l7-scene-plan.md#multi-sensor-tracking-and-fusion) and [architecture index](../../architecture/LIDAR_ARCHITECTURE.md#multi-sensor-fusion): distributed target-count/position estimation and mobile-robot control. Estimator research for fixed-sensor fusion; does not establish canonical track identity or the proposed handoff scheme.                                                                                                                                                          |
| `Kim2017Cooperative`   | [Parker and Emmons (1997), original DOE manuscript record](https://digital.library.unt.edu/ark:/67531/metadc679987/), description                                                    | Same L7 plan and architecture index: distributed mobile-robot placement to maintain observation coverage. Removed the claim that it supplies decentralised track fusion. Legacy key retained.                                                                                                                                                                                                                                                                                                                                     |
| `Liu2020HDMap`         | [Bao et al. (2022), arXiv preprint 2206.05400](https://arxiv.org/abs/2206.05400), abstract and map-generation review                                                                 | [L7 mapping references](../../../plans/lidar-l7-scene-plan.md#scene-and-map-construction), [architecture index](../../architecture/LIDAR_ARCHITECTURE.md#scene-understanding-and-mapping), and [gap-analysis inventory](../../../../data/maths/paper-implementation-gap-analysis.md#source-inventory): mapping survey context. Corrected authors/year and distinguished Li's HDMapNet framework from the survey. September reading-status history retained.                                                                       |
| `VelodyneHDL64E`       | [HDL-64E S2/S2.1 manual](https://www.termocam.it/pdf/manuale-HDL-64E.pdf), firmware 4.07, Rev H NOV12, “Develop Your Own Application-specific Point-cloud Viewer” and Appendix E     | [L1 literature context](../../architecture/LIDAR_ARCHITECTURE.md#l1-packets-sensor-transport): comparative UDP layout. Corrected the date to 2012 and removed the unsupported protocol-origin claim. It is not the Pandar40P parser specification.                                                                                                                                                                                                                                                                                |
| `HesaiROS2`            | [Official README at commit `3ef8180`](https://github.com/HesaiTechnology/HesaiLidar_ROS_2.0/blob/3ef8180ac836027012bd6553761dc96a827af6dd/README.md), introduction and configuration | Same L1 context: separate packet and point-cloud topics illustrate the L1/L2 distinction. Corrected the year to 2023 and identified support for ROS 1 and ROS 2. Reference software, not a Go runtime dependency.                                                                                                                                                                                                                                                                                                                 |
| `OSMSimple3DBuildings` | [S3DB specification revision `3071724`](https://wiki.openstreetmap.org/w/index.php?title=Simple_3D_Buildings&oldid=3071724), building parts, relations, and height tags              | [Vector-scene import design](../../architecture/vector-scene-map.md#osm-simple-3d-buildings-as-the-preferred-structure-prior), L7 plan, [pipeline review](../../../../data/maths/pipeline-review-open-questions.md), and [spatial-priors review](../../../plans/spatial-priors-service-review.md#georeferencing-sources-and-fusion): proposed outline/part/roof priors. Heights are relative to local ground, not absolute altitude; corrected the Z-placement interpretation. The historical changelog mention remains accurate. |

The implementation trace checked:

- [L1 packet ownership](../../../../internal/lidar/l1packets/doc.go),
  [Pandar40P decoding](../../../../internal/lidar/l1packets/parse/extract.go), and
  [L2 frame assembly](../../../../internal/lidar/l2frames/frame_builder.go).
  The Pandar40P decoder has its own packet constants; the Velodyne manual is comparison material.
- [L5 prediction and association](../../../../internal/lidar/l5tracks/tracking_association.go):
  constant-velocity Kalman prediction, Mahalanobis gating, and Hungarian assignment. The
  distributed PHD estimator and mobile-robot control methods are not implemented here.
- The [canonical layer status](../../../../ARCHITECTURE.md#segmented-concept-status-chart):
  L7 scene fusion remains planned. No S3DB building-prior importer was found in executable
  source; the proposed tag mapping is a design contract.

These are source-content and implementation-context checks. No replay, physical sensor,
multi-sensor identity, or mapping-accuracy validation is claimed. All 156 citation keys remain
stable; legacy names do not encode the corrected author and year. The two unidentified PET
mentions below remain unresolved.

## References added from data

| Source mention                                                          | BibTeX key      | Source document                          |
| ----------------------------------------------------------------------- | --------------- | ---------------------------------------- |
| MinkowskiEngine: 4D spatio-temporal ConvNets                            | `Choy2019`      | [Unified scene semantics, R1][semantics] |
| On Calibration of Modern Neural Networks                                | `Guo2017`       | [Unified scene semantics, R5][semantics] |
| Unsupervised 4D LiDAR moving-object segmentation in stationary settings | `Kreutz2023`    | [Unified scene semantics, R6][semantics] |
| ERASOR: static-map construction by dynamic-object removal               | `Lim2021ERASOR` | [Paper implementation gap analysis][gap] |
| 4DMOS: receding moving-object segmentation using sparse 4D convolutions | `Mersch2022`    | [Unified scene semantics, R4][semantics] |
| Point Transformer V3                                                    | `Wu2024`        | [Unified scene semantics, R3][semantics] |
| AB3DMOT: full IROS paper, arXiv 1907.03961                              | `Weng2020b`     | [Paper implementation gap analysis][gap] |
| KISS-ICP: LiDAR-only odometry baseline                                  | `Vizzo2023`     | [Survey walk geometry gain][survey]      |

SemanticKITTI's model-label mapping already has its dataset paper, `Behley2019`. The proposal
now names that key alongside the mapping repository.

## References added from the briefings

The [January 2025 to October 2026 briefing][long-brief] is the source for the following rows
unless the source column says otherwise. The [October 10 weekly briefing][weekly] repeats some
of the same works; its duplicate mentions use the same keys.

### Roadside perception and datasets

| Source mention                                                         | BibTeX key     | Source                                               |
| ---------------------------------------------------------------------- | -------------- | ---------------------------------------------------- |
| CLIFE: camera-LiDAR fusion                                             | `Bang2026a`    | Not adopted list                                     |
| PRISA: intersection safety assessment                                  | `Bang2026b`    | PRISA section; also weekly briefing                  |
| Beam-wise statistical background subtraction; HighwayScene             | `Baumann2026`  | Background subtraction section; also weekly briefing |
| HetroD                                                                 | `Chen2026`     | Roadside and drone datasets                          |
| MR-LiDAR                                                               | `Cui2026`      | Roadside and drone datasets                          |
| RESOLVE                                                                | `Ding2026`     | Roadside and drone datasets                          |
| Occlusion-aware trajectory discontinuity correction                    | `Dong2026`     | Fragment stitching                                   |
| Spatial geometry analysis for vehicle clustering                       | `Fontalvo2026` | Fontalvo section                                     |
| IMM vehicle trajectory extraction, ESWA 262:125662                     | `Gong2025`     | Not adopted list                                     |
| Rectangle edge matching, Applied Sciences 16(5):2513                   | `Hong2026`     | Rectangle edge matching; also weekly briefing        |
| Fully interpretable statistical background subtraction                 | `Iglesias2025` | Background subtraction section; also weekly briefing |
| Training-free vision-language vehicle classification, arXiv 2602.09425 | `Li2026a`      | Not adopted list                                     |
| Trajectory repair under full occlusion and limited datapoints          | `Luo2025`      | Fragment stitching                                   |
| R-LiViT                                                                | `Mirlach2025`  | Roadside and drone datasets                          |
| Lidar-based object tracking with urban sensor nodes                    | `Schafer2026`  | Schäfer section; also weekly briefing                |
| UrbanTwin                                                              | `Shahbaz2026`  | Roadside and drone datasets                          |
| Cooperative safety auditing at urban intersections                     | `Shang2026`    | PRISA and New York auditing                          |
| Multi-target tracking with collaborative roadside units under fog      | `Shi2026`      | Not adopted list; Sensors 26(3):998                  |
| GSV2X                                                                  | `Xu2026`       | Not adopted list                                     |
| Roadside lidar-based scene understanding review                        | `Zhang2026b`   | Sources checked; ISPRS Journal 233:69–88             |

### Speed, injury, conflicts, and crash benchmarks

| Source mention                                                            | BibTeX key         | Source                                            |
| ------------------------------------------------------------------------- | ------------------ | ------------------------------------------------- |
| Are the speed-crash models applicable for low speeds?                     | `Ambros2025`       | Low-speed speed-change model                      |
| Heterogeneity of 30 km/h speed interventions                              | `Ambros2026`       | Low-speed speed-change model                      |
| Speed-limit compliance in San Francisco and Phoenix                       | `Campolettano2025` | Campolettano section                              |
| Dynamic benchmarks: spatial and temporal alignment                        | `Chen2025`         | Benchmark-alignment family                        |
| Visual distraction and speeding                                           | `Chun2026`         | Not adopted list                                  |
| Updated aggregate and individual speed-safety estimates                   | `Elvik2019`        | Low-speed speed-change model                      |
| Residential speeding from connected-vehicle data                          | `Feng2026`         | Feng section                                      |
| Young novice driver speed and headway at signalised intersections         | `Howell2026`       | Not adopted list                                  |
| Medium and tall hood leading edges and pedestrian torso injuries          | `Hu2026`           | Sources checked, IIHS pedestrians row             |
| Impact speed and pedestrian fatality: systematic review and meta-analysis | `Hussain2019`      | Monfort and Mueller section                       |
| Waymo crash types at 56.7 million miles                                   | `Kusano2025`       | Benchmark-alignment family; sources checked       |
| Time of day and day of week in Waymo and human crash rates                | `Kusano2026`       | Benchmark-alignment family; sources checked       |
| Micro-level behavioural conflict framework                                | `Laureshyn2010`    | Older work; PRISA angle convention                |
| Lower urban speed limits and pedestrian accident patients                 | `Lee2025`          | Not adopted list                                  |
| Modern US pedestrian injury curve with front-end height                   | `Monfort2025`      | Monfort and Mueller section; also weekly briefing |
| Pedestrian fatality risk as a function of car impact speed                | `Rosen2009`        | Monfort and Mueller section                       |
| From Stoplights to On-Ramps                                               | `Scanlon2026a`     | Benchmark-alignment family; sources checked       |
| High-resolution urban fatal crash-rate benchmarks                         | `Scanlon2026b`     | Previously reviewed work; weekly sources checked  |
| Impact speed and a pedestrian's risk of severe injury or death            | `Tefft2013`        | Monfort and Mueller section; also weekly briefing |
| Review of city-wide 30 km/h speed-limit benefits                          | `Yannis2024`       | Sources checked; superseded synthesis             |

### Reports and benchmark studies

| Source mention                                                       | BibTeX key          | Source                                           |
| -------------------------------------------------------------------- | ------------------- | ------------------------------------------------ |
| Economic and societal impact of motor-vehicle crashes, 2019, revised | `Blincoe2023`       | Valgo's white paper section                      |
| LiDAR vulnerable-road-user detection TechBrief, FHWA-HRT-25-007      | `Calvo2024`         | Sources checked; To fetch                        |
| Advanced infrastructure detection, FHWA-HRT-24-175                   | `Calvo2025`         | Sources checked; To fetch                        |
| Safe System fatal-case attribution, IRC-26-75                        | `Campolettano2026`  | Not adopted list                                 |
| National Survey of Speeding Attitudes and Behaviors, DOT HS 813 594  | `Cosby2024`         | Questionnaires section; sources checked          |
| ERSO speed and speeding thematic report                              | `ERSOSpeed2025`     | Low-speed speed-change model                     |
| Trendline report on KPI Speed                                        | `Folla2025`         | To fetch; previously named without a URL         |
| Human Crash Baselines for Robotaxis and Robotrucks                   | `Katz2026`          | Valgo's white paper section                      |
| CCNY sensing and AI/ML final report                                  | `Li2026b`           | PRISA and New York auditing; ROSA P 91974        |
| Speeding: 2024 Data, DOT HS 813 823                                  | `NHTSASpeeding2026` | Questionnaires section; sources checked          |
| Reconstructing Sober Driving Baselines                               | `Scanlon2026c`      | Previously reviewed work; weekly sources checked |
| AAA Foundation attitudes toward speeding                             | `Steinbach2026`     | Previously reviewed work; weekly sources checked |
| Rise of the machines: automated vehicles and human drivers           | `Teoh2026`          | Teoh section                                     |
| 2025 Traffic Safety Culture Index                                    | `Zhang2026a`        | Questionnaires section; also weekly briefing     |

## Existing entries checked

Every explicit BibTeX key in the data documents resolves in the bibliography. This includes the
active background, ground-plane, clustering, classification, and tracking maths; the
velocity-coherent and ground-plane proposals; the facet-registration review; and the paper
implementation gap analysis. The two narrative citations in the visibility-aware review map to
`Bonnabel2016` and `Hatleskog2024`, and its single-object tracker maps to `Pang2021`.

The geometry-coherent tracking proposal now names `BarShalom1988a`, `Jolliffe2002`,
`Fischler1981`, `Zhang2017`, and `Held2016`. Its L-shape citation used a different title;
the reference now gives Zhang, Xu, Dong, and Dolan's _Efficient L-Shape Fitting for Vehicle
Detection Using Laser Scanners_. The gap analysis's cache label `MOT16Benchmark2016` denotes
the paper already held as `Milan2016MOTBenchmark`, rather than another publication.

The gap analysis read the full IROS AB3DMOT paper, arXiv 1907.03961, now held as `Weng2020b`.
The existing `Weng2020` key remains the distinct ECCV workshop extended abstract, arXiv
2008.08063. The existing `Lim2021` entry now gives Patchwork's published title and arXiv
identifier; its earlier title ended with "with Tilted LiDAR", which is not the title of the
paper at its DOI. Six existing entries also gained explicit arXiv fields so the downloader
can resolve the preprints already cited in the source documents.

The weekly briefing calls the fatal-rate work a Kusano paper. The identified title has Scanlon
as its first author, so its key is `Scanlon2026b`. The SOBER preprint remains `Scanlon2026c`;
acceptance alone does not supply a journal volume, page range, or DOI. Publication dates in the
bibliography follow the primary records: for example, FHWA-HRT-25-007 was published in December
2024 even though it surfaced in the later briefing. These corrections do not change the
briefings' historical findings.

## Unresolved mentions and scope boundary

The weekly briefing's sources table names only "Post-encroachment time threshold studies
(Springer 2026, ScienceDirect midblock)". Neither mention supplies an author, title, DOI, or URL.
They remain **unresolved**: no BibTeX entry is invented for either. A title or stable link is
needed to close those two references.

The [fixed-sensor briefing][fixed] cites an active FMCSA project, a vendor announcement, and a
sponsored trade article, and records that no findings or final report were published. Those
are not papers. News releases, product pages, software release notices, general guidance pages,
performance specifications, and ongoing project listings elsewhere in the briefings are also
outside this paper-and-report inventory. Their links remain in the original documents.
References inside the external papers themselves are outside the audit's scope.

[semantics]: ../../../../data/maths/proposals/20260905-unified-scene-semantic-classification-maths.md#references
[gap]: ../../../../data/maths/paper-implementation-gap-analysis.md
[survey]: ../../../../data/experiments/try/survey-walk-geometry-gain.md
[long-brief]: research-briefing-2025-01-to-2026-10.md
[weekly]: research-briefing-2026-10-10.md
[fixed]: fixed-sensor-research-briefing-2026-10-09.md
