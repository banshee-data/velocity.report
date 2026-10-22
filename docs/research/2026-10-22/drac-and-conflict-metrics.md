# DRAC, TTC and PET — evidence and measurement design

**Evidence:** Nasri et al., *Journal of Safety Research* (2026), https://doi.org/10.1016/j.jsr.2026.04.008. The October 22 briefing reported a study of 54.8 million trajectory observations at 139 Tucson intersections and stronger crash-count associations for DRAC than TTC/PET. Verify effect sizes, temporal resolution, model fit and inclusion criteria against the paper before public reuse. Hotspot/crash association is **not causal surrogate validation**.

For same-path closing motion with positive bumper gap `g` and positive closing speed `dv = v_follower - v_leader`, the simplified constant-velocity TTC is `g/dv` and simplified constant-leader-speed DRAC is `dv²/(2g)`, both conditional on their assumptions. DRAC units: m/s². PET is time from the first road user clearing a shared conflict region to the next entering it; it is not interchangeable with longitudinal TTC/DRAC. Curved or crossing paths require a distinct trajectory model.

**Measurement proposal:** calculate shadow metrics from synchronized smoothed 10 Hz tracks; use oriented physical footprints and lane/path geometry rather than centroid distance. Preserve uncertainty, closing-state flags, visibility and quality rejection reasons. Do not substitute a raw observed braking event for required braking (DRAC).

**Vehicle capability:** a hypothetical `DRAC / available_deceleration` is model-dependent. Brakes alone do not establish tire-road friction, load or available stopping force. Publish neither capability ratio nor implied avoidability without validated class assumptions and sensitivity ranges. Reaction time is absent from the simple formula and must be treated explicitly in any extended model.

**Validation:** synthetic following and crossing scenarios, ground-truth bumper gaps, derivative sensitivity, speed-near-zero handling, occlusion tests, sampling/downsampling experiments and false-positive review. No default threshold or scorecard change proposed.
