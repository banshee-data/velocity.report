# Reference Papers: Key Concepts for velocity.report

A condensed knowledge summary derived from the papers in `references.bib`.
Organised by pipeline layer to match the velocity.report LiDAR processing
architecture.

## Foundational Algorithms

**DBSCAN** (Ester 1996) — density-based clustering that discovers arbitrarily
shaped clusters without requiring a pre-set cluster count. Core algorithm for
L4 clustering in our pipeline. Operates on eps (neighbourhood radius) and
minPts (minimum density) parameters.

**HDBSCAN** (Campello 2013) — extends DBSCAN to handle variable-density
clusters by building a hierarchy of density levels. Planned as an alternative
L4 engine for mixed-traffic scenes where pedestrians and vehicles produce
clusters at very different densities.

**RANSAC** (Fischler 1981) — Random Sample Consensus fits a model to noisy
data by iteratively sampling minimal subsets and counting inliers. Used in
offline ground-plane refinement. Key property: robust to high outlier ratios.

**PCA** (Jolliffe 2002) — eigenvector decomposition of the covariance matrix.
Applied in L4 to estimate oriented bounding box (OBB) heading from point
cloud clusters by finding the principal axis.

**Kalman Filter** (Kalman 1960) — recursive predict-update estimator for
linear systems. Foundation of the L5 tracker: predicts position and velocity
at each frame, then corrects with the new measurement.

**Hungarian Algorithm** (Kuhn 1955, Munkres 1957) — optimal O(n³) assignment
of detections to tracks, minimising total cost. Used in L5 for frame-to-frame
data association. Our `hungarian.go` follows the Munkres formulation.

**Mahalanobis Distance** (Mahalanobis 1936) — scale-invariant distance
accounting for covariance. Used in L5 tracker gating to decide which
detections fall within a track's predicted uncertainty ellipse.

**Welford's Algorithm** (Welford 1962) — numerically stable online computation
of mean and variance in a single pass. Used for streaming per-cell statistics
in L3 background estimation.

**Gaussian Mixture Models** (Stauffer 1999) — adaptive background models using
mixtures of Gaussians. Our L3 background uses a simplified single-component
EMA variant of this approach.

## LiDAR Processing and Detection (L1–L4)

**Range-image segmentation** (Bogoslavskyi 2016, 2017) — treats a LiDAR
rotation as a 2D image (rings × azimuth) and uses connectivity analysis for
fast segmentation. An efficient alternative to DBSCAN considered for L4.

**PointPillars** (Lang 2019) — fast 3D object detection from point clouds
using pillar-based encoding at 62 Hz. Represents the learned-detection
alternative to our heuristic L4+L6 pipeline.

**Patchwork** (Lim 2021) and **Patchwork++** (Lim 2022) — concentric zone
models for ground segmentation. More robust than simple height-band filters,
especially on tilted sensors. Planned upgrade path for L3/L4 ground removal.

**RangeNet++** (Milioto 2019) — semantic segmentation on range images.
Demonstrates the range-image representation concept used for LiDAR data.

**PCL** (Rusu 2011) — the Point Cloud Library provides VoxelGrid downsampling
and EuclideanClusterExtraction. Our voxel grid + DBSCAN pipeline mirrors the
PCL processing chain.

**Ground plane extraction** (Zermas 2017) — line fitting in polar slices for
fast ground removal. Informs the height-band approach in L4.

## Multi-Object Tracking (L5)

**SORT** (Bewley 2016) — Simple Online and Realtime Tracking: 2D Kalman filter
plus Hungarian assignment with a simple lifecycle model (tentative → confirmed
→ lost). Our L5 lifecycle follows SORT conventions.

**AB3DMOT** (Weng 2020) — 3D Kalman + Hungarian baseline for LiDAR MOT at
207 Hz. Validates our architectural choices and provides a benchmarking
baseline for 3D tracking performance.

**CenterPoint** (Yin 2021) — centre-based 3D detection and tracking using
greedy closest-point matching as a simpler alternative to Hungarian assignment.

**CLEAR MOT** (Bernardin 2008) — standard evaluation metrics (MOTA, MOTP) for
multi-object tracking. Used in L8 analytics run comparisons.

## Advanced Motion Models (L5h Planned)

**IMM** (Blom 1988) — Interacting Multiple Model filter that blends multiple
motion models (e.g. constant-velocity and constant-acceleration) using
Markovian switching. Planned for L5h engine upgrades.

**UKF** (Julier 1997) — Unscented Kalman Filter using sigma-point propagation
for nonlinear systems. Recommended over EKF for the CTRV motion model.

**RTS Smoother** (Rauch 1965) — fixed-interval smoother for post-hoc
trajectory refinement. Used in the evaluation-only RTS variant of L5h.

**Tracking and Data Association** (Bar-Shalom 1988) — Bayesian framework for
detection uncertainty. Theoretical basis for covariance-gated promotion in L3f.

## Scene Understanding and Mapping (L6–L7)

**SemanticKITTI** (Behley 2019) — 28-class point-level semantic labels for
LiDAR sequences. AV compatibility mapping targets this taxonomy in L6.

**nuScenes** (Caesar 2020) — 23-class AV taxonomy with large-scale labelled
data. Primary target for future learned classifier training in L6e.

**KITTI** (Geiger 2012) — standard 3D object detection benchmark. Our L6
class definitions (car, pedestrian, cyclist) calibrated against KITTI.

**OctoMap** (Hornung 2013) — probabilistic 3D occupancy using octrees.
Comparison point for our polar-grid tile-confidence model.

**HD Map maintenance** (Pannen 2020, Liu 2020, Li 2022) — approaches to
building and maintaining high-definition maps. Our L7 scene accumulation
follows the neighbourhood-scale variant of these paradigms.

**Long-term map maintenance** (Pomerleau 2014) — removing dynamic points from
accumulated maps. Validates our background settling and drift correction
strategy in L3.

**Procrustes alignment** (Schönemann 1966) — rigid alignment via SVD. Used in
L7 prior-to-observation registration.

## Trajectory Prediction (L7 Planned)

**Motion prediction survey** (Lefèvre 2014) — comprehensive survey of motion
prediction and risk assessment methods for intelligent vehicles.

**Lane-graph prediction** (Liang 2020) — scene-constrained trajectory
forecasting using lane graph structures. Planned for L7.

**Trajectron++** (Salzmann 2020) — heterogeneous multi-agent trajectory
forecasting. Planned L7 trajectory prediction engine.

**Constant velocity baseline** (Schöller 2020) — analyses how well simple
constant-velocity models predict pedestrian motion. Context for L7 motion
prior selection.
