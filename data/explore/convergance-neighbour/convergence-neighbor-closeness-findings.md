# LiDAR Neighbor & Closeness Convergence Analysis

**Date:** October 31, 2025
**Test Type:** 40-minute multi-parameter sweep with live LiDAR data
**Parameters:** Fixed noise=0.030, closeness={1.5,2.0,2.5,3.0}, neighbor={0,1,2,3}
**Configuration:** 16 combinations × 50 iterations × 3s interval = ~40 minutes total

## Executive Summary

**Critical Finding:** The neighbor confirmation parameter has a **dramatic** impact on convergence stability, with `neighbor=1` showing 94% better stability compared to other values.

### Optimal Configuration

- **Noise:** 0.030
- **Closeness:** 2.5
- **Neighbor:** 1
- **Performance:** 99.96% acceptance ± 0.0014 (EXCELLENT stability)
- **Coverage:** 58,716 cells ± 66

## Key Findings

### 1. Neighbor Confirmation Impact ⭐ CRITICAL

The neighbor confirmation count is the **most important parameter** for convergence stability:

| Neighbor | Avg StdDev   | Stability Rating | Acceptance Rate |
| -------- | ------------ | ---------------- | --------------- |
| **1**    | **0.001995** | **EXCELLENT**    | **99.95%**      |
| 0        | 0.032924     | POOR             | 99.22%          |
| 2        | 0.034843     | POOR             | 99.23%          |
| 3        | 0.033721     | POOR             | 99.22%          |

**Impact:** Neighbor=1 provides **94.1% improvement** in stability (16.5× better) compared to other values.

### 2. Why Neighbor=1 Works Best

**Neighbor=0 (No confirmation required):**

- ❌ Too permissive - accepts noise/outliers
- ❌ High variation in acceptance rates
- ❌ Unstable background model
- Initial acceptance: 76.7% → converges slowly to 99.9%

**Neighbor=1 (Require 1 neighbor confirmation):**

- ✅ Perfect balance - filters noise while accepting real patterns
- ✅ Fast convergence (stabilises within 10-15 iterations = 30-45 seconds)
- ✅ Consistently high acceptance (>99.9%)
- ✅ Minimal variation throughout test period
- Initial acceptance: 98.4% → converges rapidly to 99.99%

**Neighbor=2,3 (Require 2-3 neighbor confirmations):**

- ❌ Too restrictive - rejects valid points
- ❌ High variation similar to neighbor=0
- ❌ Slower convergence
- Initial acceptance: 65% → converges slowly to 99.8%

### 3. Closeness Parameter Impact

When neighbor=1, closeness has **minimal impact** on stability:

| Closeness | Avg StdDev   | Avg Acceptance | Stability |
| --------- | ------------ | -------------- | --------- |
| 1.5       | 0.002554     | 99.93%         | EXCELLENT |
| 2.0       | 0.002218     | 99.94%         | EXCELLENT |
| **2.5**   | **0.001358** | **99.96%**     | **BEST**  |
| 3.0       | 0.001849     | 99.95%         | EXCELLENT |

**Recommendation:** Closeness=2.5 shows slight advantage but all values 1.5-3.0 work well with neighbor=1.

When neighbor≠1, closeness variation is **irrelevant** - all show poor stability.

### 4. Convergence Speed Analysis

**Optimal Config (closeness=2.5, neighbor=1):**

```
Iteration 0:    Overall=99.02%, Range=0.9%
Iteration 10:   Overall=99.97%, Range=0.002%
Iteration 25:   Overall=99.98%, Range=0.00002%
Iteration 49:   Overall=99.98%, Range=0.0005%

Convergence: 99.9% improvement in stability
Time to stability: ~30-45 seconds (10-15 iterations)
```

**Poor Config (closeness=2.0, neighbor=0):**

```
Iteration 0:    Overall=76.67%, Range=23%
Iteration 10:   Overall=99.67%, Range=0.4%
Iteration 25:   Overall=99.81%, Range=0.04%
Iteration 49:   Overall=99.86%, Range=0.01%

Convergence: Slower, never reaches EXCELLENT stability
Time to stability: >2.5 minutes, still variable
```

### 5. Configuration Performance Matrix

| Closeness                         | Neighbor | Overall Acc  | StdDev       | Stability        | Cells StdDev |
| --------------------------------- | -------- | ------------ | ------------ | ---------------- | ------------ |
| 2.5                               | **1**    | **99.96%**   | **0.001358** | **⭐ EXCELLENT** | **66**       |
| 3.0                               | 1        | 99.95%       | 0.001849     | EXCELLENT        | 58           |
| 2.0                               | 1        | 99.94%       | 0.002218     | EXCELLENT        | 433          |
| 1.5                               | 1        | 99.93%       | 0.002554     | EXCELLENT        | 53           |
| 2.5                               | 3        | 99.43%       | 0.019777     | GOOD             | 47           |
| 3.0                               | 2        | 99.39%       | 0.024263     | MODERATE         | 78           |
| ... all neighbor=0                | ...      | 99.22%       | 0.032-0.033  | POOR             | 52-292       |
| ... all neighbor=2 (except 2.5,3) | ...      | 99.02-99.24% | 0.033-0.049  | POOR             | 53-61        |
| ... all neighbor=3 (except 2.5)   | ...      | 99.00-99.24% | 0.033-0.049  | POOR             | 59-65        |

## Detailed Time-Series Convergence

### Optimal Configuration: closeness=2.5, neighbor=1

**Progression:**

- **0-10 sec (iterations 0-3):** Rapid initial convergence from 99.0% → 99.9%
- **30-45 sec (iterations 10-15):** Achieves stable state >99.97%
- **45+ sec:** Minimal variation, range <0.0001%

**Metrics:**

- Early period (0-30s): Average 99.86%, Range 0.9%
- Middle period (60-90s): Average 99.98%, Range 0.00002%
- Late period (120-150s): Average 99.98%, Range 0.0005%

**Convergence improvement:** 99.9% (early range → late range)

### Comparison: closeness=2.0, neighbor=2 (Poor Config)

**Progression:**

- **0-30 sec:** Very slow convergence from 65% → 99.5%
- **30-90 sec:** Gradual improvement to 99.8%
- **90+ sec:** Still shows variation, never fully stable

**Metrics:**

- Early period: Average 95.8%, Range 34.6%
- Middle period: Average 99.83%, Range 0.04%
- Late period: Average 99.87%, Range 0.01%

**Convergence improvement:** 100% but final stability is 20× worse than optimal

## Recommendations

### Production Configuration

```
noise_relative: 0.030
closeness_multiplier: 2.5
neighbor_confirmation_count: 1
seed_from_first_frame: true
```

**Rationale:**

1. **Neighbor=1 is critical** - provides 94% better stability than other values
2. **Closeness=2.5** - slight advantage over other values when paired with neighbor=1
3. **Noise=0.030** - from previous analysis, optimal for convergence
4. **Fast convergence** - stable within 30-45 seconds
5. **High acceptance** - 99.96% with minimal variation
6. **Consistent coverage** - 58,716 cells with only 66 cell stddev (0.11%)

### Alternative Configurations

If closeness=2.5 causes issues in specific environments:

- **Alternative 1:** noise=0.030, closeness=3.0, neighbor=1 (nearly identical performance)
- **Alternative 2:** noise=0.030, closeness=2.0, neighbor=1 (slightly higher cell variation)

**Do NOT use:**

- neighbor=0 (too permissive, unstable)
- neighbor=2 or neighbor=3 (too restrictive, poor stability)

## Why Neighbor=1 is Optimal

The neighbor confirmation parameter acts as a **noise filter with minimal false rejection**:

### Mathematical Intuition

- **Neighbor=0:** Accept any point (no spatial validation)
  - Problem: Random noise gets accepted → unstable background

- **Neighbor=1:** Accept if point has ≥1 nearby point confirming the measurement
  - Sweet spot: Real surfaces naturally have neighboring points
  - Filters: Isolated noise/outliers that don't repeat
  - Minimal cost: Real surfaces rarely have isolated single points

- **Neighbor=2,3:** Require multiple confirmations
  - Problem: Edge points and sparse regions get rejected
  - Result: Over-filtering causes instability from accept/reject cycling

### Empirical Evidence

From 800+ samples across 16 configurations:

- Neighbor=1 shows 16.5× better stability
- 99.9% convergence improvement in first 30 seconds
- Maintains stability over extended periods
- Works consistently across all closeness values

## Next Steps

1. ✅ **Deploy to production:** Update default parameters
2. ✅ **Validate with PCAP:** Confirm behavior with recorded data
3. ⬜ **Long-term monitoring:** Track convergence metrics in production
4. ⬜ **Edge case testing:** Validate in sparse/dense environments
5. ⬜ **Document in codebase:** Update parameter descriptions and defaults

## Files Generated

- `convergence-neighbor-closeness-40min.csv` - Summary statistics (16 configurations)
- `convergence-neighbor-closeness-40min-raw.csv` - Time-series data (800 samples)
- `convergence-neighbor-closeness-findings.md` - This analysis document

## Appendix: Statistical Summary

**Total test duration:** 39 minutes 25 seconds
**Total samples collected:** 800 (50 per configuration)
**Total parameter combinations:** 16
**Distance buckets analysed:** 11 (1m, 2m, 4m, 8m, 10m, 12m, 16m, 20m, 50m, 100m, 200m)

**Best configuration metrics:**

- Overall acceptance: 99.9643% ± 0.001358
- Nonzero cells: 58,716 ± 66 (0.11% variation)
- 1m bucket acceptance: 99.99% ± 0.001%
- 4m bucket acceptance: 99.97% ± 0.002%
- 8m bucket acceptance: 99.98% ± 0.003%
- Convergence time: 30-45 seconds
- Stability rating: EXCELLENT
