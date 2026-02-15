# Quick Start: Addressing Pipeline Quality Issues

**Issue:** High jitter, fragmentation, misalignment, and empty boxes in tracking results
**Date:** 2026-02-14

---

## TL;DR

**Systematic Issues Found:**

1. **Overly conservative foreground extraction** → Empty boxes (tracks without points)
2. **Too-tight clustering** → Fragmentation (vehicles split into multiple tracks)
3. **High Kalman noise parameters** → Jitter (spinning boxes, erratic velocity)
4. **Tight association gate** → Misalignment and lost tracks

**Solution:** Deploy optimised parameters in `config/tuning.optimised.json`

**Expected Results:**

- 60-75% reduction in jitter, fragmentation, misalignment, empty boxes
- 15% improvement in foreground capture

---

## Quick Deployment

### Option 1: Use Optimised Configuration (Recommended)

```bash
# Apply the optimised configuration
cp config/tuning.optimised.json /path/to/your/active/config.json

# Or specify in command line
./velocity-report --config config/tuning.optimised.json
```

### Option 2: Run Auto-Tuning Sweep

If you want to refine parameters further:

```bash
# Run the quality-tuning sweep (takes ~3 hours)
./sweep --config config/sweep-quality-tuning.json --pcap your-capture.pcap
```

---

## Key Parameter Changes

The most impactful changes from current defaults:

| Parameter                 | Current | Optimised | Why                                                            |
| ------------------------- | ------- | --------- | -------------------------------------------------------------- |
| `closeness_multiplier`    | 8.0     | 3.0       | **Was allowing vehicle points to be classified as background** |
| `safety_margin_meters`    | 0.4     | 0.15      | **Was suppressing vehicle edge points**                        |
| `foreground_dbscan_eps`   | 0.3     | 0.7       | **Was fragmenting distant vehicles**                           |
| `gating_distance_squared` | 4.0     | 25.0      | **Was preventing track re-association**                        |
| `measurement_noise`       | 0.3     | 0.15      | **Was causing heading jitter**                                 |
| `process_noise_vel`       | 0.5     | 0.3       | **Was allowing velocity drift**                                |

See `docs/parameter-comparison.md` for full comparison table.

---

## What Was Wrong?

### Problem 1: Empty Boxes (EmptyBoxRatio > 0.15)

**Cause:**

- `closeness_multiplier=8.0` and `safety_margin_meters=0.4` are extremely conservative
- Vehicle points misclassified as background → clusters shrink → tracks have no points

**Fix:**

- `closeness_multiplier=3.0` (standard value)
- `safety_margin_meters=0.15` (reduced by 62%)

### Problem 2: Fragmentation (FragmentationRatio > 0.40)

**Cause:**

- `foreground_dbscan_eps=0.3` too small for distant vehicles (point spacing > 0.3m at 50m)
- `gating_distance_squared=4.0` (2m radius) too tight to merge fragments

**Fix:**

- `foreground_dbscan_eps=0.7` (standard value, merges sub-clusters)
- `gating_distance_squared=25.0` (5m radius, allows re-association)

### Problem 3: Jitter (HeadingJitterDeg > 45°, SpeedJitterMps > 2.0)

**Cause:**

- `measurement_noise=0.3` causes Kalman to distrust observations → amplifies PCA noise
- `process_noise_vel=0.5` allows velocity to drift independently of position

**Fix:**

- `measurement_noise=0.15` (trusts observations more)
- `process_noise_vel=0.3` (constrains velocity drift)

### Problem 4: Misalignment (MisalignmentRatio > 0.30)

**Cause:**

- High `process_noise_vel=0.5` allows velocity to evolve independently of displacement
- Position corrected by measurements, but velocity drifts → divergence

**Fix:**

- `process_noise_vel=0.3` (tighter coupling between position and velocity)

---

## Testing Strategy

### Incremental Validation (Recommended)

Don't deploy all changes at once. Test in stages:

**Stage 1: Foreground Only (15 min)**

- Apply: `closeness_multiplier=3.0`, `safety_margin_meters=0.15`, `neighbor_confirmation_count=3`
- Check: EmptyBoxRatio should drop from 0.15-0.25 to ~0.10

**Stage 2: Add Clustering (15 min)**

- Add: `foreground_dbscan_eps=0.7`, `foreground_min_cluster_points=5`
- Check: FragmentationRatio should drop from 0.40-0.50 to ~0.15

**Stage 3: Add Tracking (30 min)**

- Add: `gating_distance_squared=25.0`, `measurement_noise=0.15`, `process_noise_vel=0.3`
- Check: HeadingJitterDeg and SpeedJitterMps should halve

### Full Deployment

If all stages succeed, deploy complete `tuning.optimised.json` for overnight testing.

---

## Expected Improvements

| Metric             | Before      | After       | Change |
| ------------------ | ----------- | ----------- | ------ |
| HeadingJitterDeg   | 45-60°      | 15-25°      | ↓ 60%  |
| SpeedJitterMps     | 2.0-3.0 m/s | 0.5-1.0 m/s | ↓ 70%  |
| FragmentationRatio | 0.40-0.50   | 0.10-0.15   | ↓ 75%  |
| MisalignmentRatio  | 0.30-0.40   | 0.10-0.15   | ↓ 65%  |
| EmptyBoxRatio      | 0.15-0.25   | 0.05-0.10   | ↓ 60%  |
| ForegroundCapture  | 0.70-0.75   | 0.85-0.90   | ↑ 15%  |

---

## Detailed Documentation

- **Full Analysis:** `docs/pipeline-diagnosis.md` (17 pages)
  - Detailed explanation of each issue
  - Parameter interaction analysis
  - Alternative scenarios (highway, urban, nighttime)
- **Quick Reference:** `docs/parameter-comparison.md`
  - Side-by-side comparison table
  - Critical changes highlighted
  - Deployment recommendations

- **Optimised Config:** `config/tuning.optimised.json`
  - Ready-to-deploy configuration
  - All parameters tuned for urban street scenarios

- **Sweep Config:** `config/sweep-quality-tuning.json`
  - Auto-tuning configuration for further refinement
  - Targets jitter, fragmentation, misalignment, empty boxes

---

## Reverting Changes

If optimised parameters cause unexpected issues:

```bash
# Restore defaults
cp config/tuning.defaults.json /path/to/your/active/config.json
```

---

## Questions?

**Q: Why were the original parameters so conservative?**
A: Likely tuned for a different scenario (e.g., nighttime, dense urban) or to minimise false positives at the cost of false negatives.

**Q: Will these changes work for all scenarios?**
A: The optimised config is tuned for typical urban street scenarios. See `docs/pipeline-diagnosis.md` Section 5 for highway, dense urban, and nighttime adjustments.

**Q: Can I tune parameters further?**
A: Yes! Use `config/sweep-quality-tuning.json` for auto-tuning. Expected runtime: ~3 hours for 3 rounds.

**Q: What if results get worse?**
A: Revert to defaults immediately. Then try incremental deployment (Stages 1-3) to identify which change caused issues.

---

## Summary

**The pipeline is sound.** Issues stem from parameter mismatches across coupled subsystems (foreground extraction, clustering, tracking). The optimised configuration restores proper coupling and should deliver 60-75% quality improvements.

**Next step:** Deploy `config/tuning.optimised.json` and measure results.
