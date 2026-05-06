# LiDAR tuning guide

Entry point for all parameter tuning documentation. Start here when you need to
adjust how the pipeline detects, clusters, or tracks objects.

## Tuning stages

The tuning tools form a progression from manual knob-turning to fully automated
multi-round optimisation with human feedback:

| Stage                        | Document                                           | When to Use                                                          |
| ---------------------------- | -------------------------------------------------- | -------------------------------------------------------------------- |
| 1. Understand the parameters | [parameter-comparison.md](parameter-comparison.md) | Read first: side-by-side of defaults vs optimised, with impact notes |
| 2. Manual runtime tuning     | [config-param-tuning.md](config-param-tuning.md)   | Ad-hoc `curl` changes, validate with live data or PCAP replay        |
| 3. Automated sweep           | [sweep-tool.md](sweep-tool.md)                     | Systematic grid search across parameter ranges                       |
| 4. Multi-round auto-tuning   | [auto-tuning.md](auto-tuning.md)                   | Iterative grid search with automatic bound narrowing                 |
| 5. Human-in-the-loop tuning  | [hint-sweep-mode.md](hint-sweep-mode.md)           | HINT mode: human labels drive the objective function each round      |

## Glossary

Terminology used across the tuning docs. Linked from individual tool docs so the
definitions live in exactly one place.

| Term             | Meaning                                                                                   |
| ---------------- | ----------------------------------------------------------------------------------------- |
| **Sweep**        | A batch run that evaluates every combination in a parameter grid                          |
| **Combo**        | One specific set of parameter values being evaluated within a sweep                       |
| **Round**        | One complete sweep; auto-tuning runs multiple rounds with progressively narrower bounds   |
| **Top K**        | The best-scoring combinations from a round, used to narrow bounds for the next round      |
| **Settle**       | The warmup period where the background model stabilises before sampling begins            |
| **Scene**        | A named evaluation environment with labelled reference tracks for ground-truth scoring    |
| **Run**          | A single processing pass over data with fixed parameters, producing tracks for evaluation |
| **Manual sweep** | Operator-driven grid search: full control over param grid and metric visualisation        |
| **Auto-tune**    | Automated multi-round narrowing with weighted proxy metrics                               |
| **HINT**         | Human-Involved Numerical Tuning: human labels drive the objective function each round     |

## Quick troubleshooting

If the pipeline produces poor results (jitter, fragmentation, empty boxes),
the fastest path is the quick-fix section at the top of the diagnosis doc:

- [Pipeline Diagnosis: Quick Fixes](../troubleshooting/pipeline-diagnosis.md#quick-fixes)

## Operational baseline

Track labelling and core auto-tuning workflows are implemented and active in
production. The labelling pipeline integrates with the run browser and label APIs.
Analysis-run integration supports labelling and replay flows. Runtime and storage
components are in place for current usage.

Deferred phases (advanced labelling, extended auto-aware tuning) are tracked in:

- [`../../plans/lidar-track-labelling-auto-aware-tuning-plan.md`](../../plans/lidar-track-labelling-auto-aware-tuning-plan.md)

## Mathematical references

For algorithm-level detail on the parameters and their derivations:

- [MATHS.md](../../../data/maths/MATHS.md) §Config Mapping: complete key-to-source mapping
