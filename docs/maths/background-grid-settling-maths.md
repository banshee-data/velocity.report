# Background Grid Settling Maths

**Status:** Implementation-aligned math note
**Layer:** L3 Grid (`internal/lidar/l3grid`)
**Related:** [Ground Plane Maths](ground-plane-maths.md)

## 1. Purpose

The L3 background grid estimates a stable sensor-frame baseline in polar coordinates:

- index space: `(ring, azimuth_bin)`,
- state per cell: mean range, spread, confidence, freeze/lock metadata.

The model is an exponential moving average variant (often called EWA/EWMA/EMA in docs).

## 2. Cell State

For each cell `c`:

- `mu_c` = `AverageRangeMeters`
- `s_c` = `RangeSpreadMeters`
- `n_c` = `TimesSeenCount`
- `frozen_until_c`
- `recent_fg_c` = consecutive recent foreground pressure
- `locked_baseline_c`, `locked_spread_c`, `locked_at_count_c`

## 3. Observation Binning

Incoming polar points are mapped to `(ring, az_bin)`.

- Batch path (`ProcessFramePolar`) aggregates mean/min/max per cell per frame.
- Point-mask path (`ProcessFramePolarWithMask`) classifies each point directly.

Both paths use the same core threshold family and confidence dynamics.

## 4. Closeness Threshold Model

Base threshold for a point/observation at range `r`:

`tau = k_close * (s_c + k_noise * r + 0.01) + safety`

where:

- `k_close = ClosenessSensitivityMultiplier`
- `k_noise = NoiseRelativeFraction`
- `safety = SafetyMarginMeters`

In `ProcessFramePolarWithMask`, warmup sensitivity inflates tolerance for low-confidence cells:

`warmup_mult = 1 + 3*(100 - n_c)/100` for `n_c < 100`, else `1`

`tau_warm = tau * warmup_mult`

This prevents early false foreground while spread is still under-learned.

## 5. Neighbour Confirmation

Same-ring neighbours vote for background consistency.

Neighbour `j` confirms if:

`|mu_j - r_obs| <= k_close * (s_j + k_noise * mu_j + 0.01)`

If confirmation count reaches `NeighborConfirmationCount`, the point can be accepted as background even when direct cell residual is marginal.

## 6. Decision Logic

For each point:

1. If cell is frozen (`now < frozen_until_c`): classify foreground.
2. Else compute residual:
   `delta = |mu_c - r_obs|`
3. Evaluate locked-baseline gate (if locked):
   `delta_lock = |locked_baseline_c - r_obs|`
   `tau_lock = max(locked_multiplier*locked_spread_c + k_noise*r_obs + safety, 0.1)`
4. Background-like if any is true:
   - inside locked window,
   - `delta <= tau_warm`,
   - neighbor confirmation threshold reached.
5. Deadlock breaker:
   if low confidence floor + repeated FG pressure + not extreme divergence,
   force relearning to prevent stale ghost baselines.

## 7. State Updates

### 7.1 Background update (accepted)

If empty cell and seeding enabled: initialize from first observation.

Else exponential update:

`mu_c <- (1-alpha)*mu_c + alpha*r_obs`

`dev <- |r_obs - mu_old|`

`s_c <- (1-alpha)*s_c + alpha*dev`

`n_c <- n_c + 1`

If recovering after recent FG, use boosted `alpha`:

`alpha_eff = min(alpha * ReacquisitionBoostMultiplier, 0.5)`

### 7.2 Foreground update (rejected)

- Increment `recent_fg_c` (saturating).
- Decrement `n_c`, but not below `MinConfidenceFloor` (unless floor explicitly zero).
- If strong divergence:
  `delta > FreezeThresholdMultiplier * tau_warm`
  then set freeze:
  `frozen_until_c <- now + FreezeDuration`.

## 8. Locked Baseline Subsystem

Once `n_c` reaches lock threshold:

- snapshot `locked_baseline_c <- mu_c`
- snapshot `locked_spread_c <- s_c`

After lock, update very slowly only during sustained background:

`locked_baseline_c <- (1-beta)*locked_baseline_c + beta*r_obs` with `beta ~ 0.001`.

This is a drift-resistant reference used before standard EMA in classification.

## 9. Warmup and Settlement

Global warmup gate uses duration and/or minimum-frame constraints.

Until warmup complete:

- model still learns from observations,
- foreground outputs are suppressed (`mask=false`) to avoid early noise bursts.

Post-settle may use reduced alpha (`PostSettleUpdateFraction`) for stability.

## 10. Region-Adaptive Parameters

After settling, cells may be partitioned into variance-driven regions.

Per-region overrides can adjust:

- `NoiseRelativeFraction`,
- `NeighborConfirmationCount`,
- `SettleUpdateFraction`.

Mathematically this is a piecewise-parameter model over the polar grid.

## 11. Assumptions and Limits

1. **Unimodal per-cell background assumption**
   - Fails when one cell alternates between two persistent depths.
2. **Range-only state**
   - No explicit angular/elevation surface normal in L3 itself.
3. **Heuristic confidence counter (`n_c`)**
   - Operationally useful but not a calibrated probability.
4. **Freeze/lock thresholds are policy parameters**
   - Strongly affect adaptation-vs-stability tradeoff.
5. **Same-ring neighbor confirmation**
   - Reduces cross-elevation bias but can miss vertical consistency cues.

## 12. Interface to L4 Ground Plane

For long-running static mapping, L4 should consume L3 with reliability weighting, not hard binary gating:

`w_L3 = h(n_c, s_c, frozen_state, locked_state, recent_fg_c)`

This allows L4 to maintain geometric consistency even when L3 is temporarily unstable or reacquiring.
