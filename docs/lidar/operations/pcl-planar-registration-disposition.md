# PCL planar registration belongs inside facet arm B

- **Status:** OQ-06 answered; bounded comparator retained, no separate experiment or runtime port
- **Owner plan:** [Facet E2, arm B](../../plans/lidar-facet-registration-experiment-plan.md#e2-isolate-the-registration-benefit)
- **Related:** [Geometry convergence, section 4](../../plans/lidar-tracker-geometry-convergence-plan.md#4-what-is-needed), [PCL review](pcl-algorithm-review.md)
- **Source:** PCL 1.15.1, `3346843ea6b97697a1577293aa9cdf236b5c7ffb`

## Disposition

Retain one offline planar point-registration comparator within facet E2 arm B, subject to that
plan's E1 and funding gates. Do not create another experiment spec or a separate W4 workstream.
It can check whether the method of aligning an identical bounded history changes pose or visible
support. W4 owns censored extent inference and revision of admitted evidence; the comparator
neither estimates unseen extent nor replaces that belief. A useful negative result ends the
comparator without adding a runtime dependency.

The distinction matters because arm B already retains body-local geometry and explicitly uses
robust point-to-plane residuals, per-return anisotropic noise, frozen maps, and overlap abstention.
A stock PCL point-to-point or NDT objective is not that estimator. It can be a simpler comparator,
but cannot isolate an optimiser change while also changing the residual model.

## Correction to OQ-01: the public 2-D estimator suppresses z translation

OQ-01's Q2-03 finding was wrong. It inspected the final translation expression without following
the centroids passed into it. All four public estimateRigidTransformation overloads feed the
iterator implementation. That implementation [clears centroid z at lines 139–141][two-d], then
forms a yaw rotation and the centroid translation at lines 165–182. The resulting z translation
is zero on this path, even if source and target have different mean z. This correction supersedes
the earlier claim that a z offset is reproduced; it is not an upstream change.

The new [registration-check.cpp](../../../tools/pcl-review/registration-check.cpp) executes the
whole-cloud and correspondence overloads. Source points (0,0,1), (1,0,2), and (0,1,3) are paired
with targets rotated 90° about z, translated (2,−1), and raised 7 m. Both return tx = 2,
ty = −1, yaw = 90°, tz = 0, within a matrix tolerance of 0.00001. The transform preserves source
z, so it does not align the deliberately shifted target heights. This synthetic check refutes
our previous z-translation example; it does not test real registration or physical accuracy.

XY projection is still needed if the surrounding correspondence search and fitness are intended
to be planar: choosing a transformation estimator does not configure those components. Preserve
the original XYZ and ordinals outside any projected search copy. Grade, roll, pitch, and
articulation remain the facet plan's validity conditions, not extra fitted degrees of freedom.

## What the other PCL options establish

- [WarpPointRigid3D, lines 71–90][warp] parameterises tx, ty, and yaw. That limits motion, not
  the objective. [TransformationEstimationLM, lines 174–201][lm] uses Euclidean point residuals
  by default; supplying this warp does not create B's robust anisotropic point-to-plane loss.
- [NDT2D, lines 119–150][ndt-cov] forms XY cell distributions and regularises their covariance.
  Its [transformation loop, lines 412–451][ndt-fit] uses x, y, and yaw and can modify a Hessian
  for numerical definiteness. Those changes do not supply data-only observability or calibrated
  uncertainty. Grid density and regularisation introduce another comparison factor.

Select TransformationEstimation2D inside an explicitly XY correspondence loop for the bounded
comparator. Leave NDT2D and the LM/warp alternative out of the initial comparison; they do not
justify two extra variants before the simpler comparison answers its question. This chooses a
future harness contract, not an already implemented iterative registration tool. The new fixture
only tests the transformation estimator.

## Comparator contract and counterexamples

Use B's exact reviewed membership, capture instants and deskew, initial pose/uncertainty, motion
prior, body origin, history horizon, retained-point cap, and frozen pre-update map. Do not use
future review poses for matching or initialise only the comparator from physical truth. Freeze
the XY correspondence distance, overlap threshold, robust rejection policy, iteration limit,
and tuning budget before E2 scoring, under the same development/tuning split as B. Declare every
loss or weighting difference: the selected PCL point-to-point reference does not implement B's
uncertainty model. It must not export a covariance or feed its prior back as new information.

Score paired pose and physical-endpoint errors, supported coverage, abstentions, and cost under
E2's existing scorecard. Keep raw directional extrema of the identical admitted history beside
the registered support. This distinguishes pose alignment from adding samples or changing which
samples count. Registration residual or smoother boxes alone cannot pass H2/H2s. No new acceptance
threshold or campaign parameter set is introduced until the owner plan's implementation step.

Three counterexamples constrain interpretation; these are analytical cases, not new measurements:

1. A long featureless side supplies a normal but no reliable translation along itself. Fixed
   point correspondences can manufacture tangential information; perturb and rematch, and report
   the data-only direction separately from the motion prior.
2. Two vehicles with identical visible sides and different unseen bumper positions produce the
   same visible registrations. Better alignment does not determine their hidden lengths.
3. A mistakenly admitted neighbouring vehicle can enlarge an accumulated footprint while reducing
   an alignment residual. Preserve B's de-duplication and rollback tests and W4's revisable
   evidence; never corroborate a dimension merely by repeatedly reusing the same points.

## Validation and next checkpoint

The pinned checkout is clean. The new target builds against its headers and existing common
library, with no registration-library or product dependency. CTest passes the registration
regression and the existing segmentation contract test. Repository `make test`, `make lint`, `make check-md-links`, and `make check-no-results` pass.
The macOS test build stamped BuildInfo; its tracked placeholder was restored before the final
lint pass. Source-link paths/ranges and dependent plan wording were checked. Raw logs remain under `/private/tmp/velocity-pcl-review/`.
No capture replay, pose comparison, extent gain, or facet promotion was performed for OQ-06.
OQ-07 next decides whether static hypothesis verification supplies a distinct duplicate-track arm.

[two-d]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/registration/include/pcl/registration/impl/transformation_estimation_2D.hpp#L113-L182
[warp]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/registration/include/pcl/registration/warp_point_rigid_3d.h#L71-L90
[lm]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/registration/include/pcl/registration/transformation_estimation_lm.h#L174-L201
[ndt-cov]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/registration/include/pcl/registration/impl/ndt_2d.hpp#L119-L150
[ndt-fit]: https://github.com/PointCloudLibrary/pcl/blob/3346843ea6b97697a1577293aa9cdf236b5c7ffb/registration/include/pcl/registration/impl/ndt_2d.hpp#L412-L451
