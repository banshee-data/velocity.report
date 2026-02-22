# LiDAR Parameter Tuning & Optimisation

## Status: Planned

## Summary

Systematically explore parameter space to optimise track quality metrics. This
is Phase 4.2 of the LiDAR ML pipeline, targeting the v2.0 milestone.

## Related Documents

- [Product Roadmap](../ROADMAP.md) — milestone placement (v2.0)
- [Analysis Run Infrastructure](lidar-analysis-run-infrastructure-plan.md) — provides run comparison and quality metrics
- [Sweep/HINT Mode](lidar-sweep-hint-mode-plan.md) — existing parameter sweep system (Phase 3.9, ✅ implemented)
- [ML Classifier Training](lidar-ml-classifier-training-plan.md) — benefits from tuned parameters

---

## Dependencies

- Analysis run infrastructure (Phase 3.7, ✅ implemented) provides the
  comparison framework.
- Sweep/auto-tune system (Phase 3.9, ✅ implemented) provides the runner;
  this phase adds objective-function-driven optimisation on top.

---

## Tuning Workflow

1. Define parameter grid
2. For each parameter combination:
   - Run analysis on reference PCAP
   - Compare to baseline run
   - Detect splits/merges
   - Compute quality metrics
3. Analyse results to find optimal parameters
4. Validate on held-out PCAPs

## Optimisation Objective

Maximise confirmed tracks while minimising splits, merges, and noise:

```
objective = w1 × confirmed_tracks
          − w2 × split_count
          − w3 × merge_count
          − w4 × noise_tracks
          + w5 × avg_track_duration
```

Default weights: w1 = 1.0, w2 = 5.0, w3 = 5.0, w4 = 2.0, w5 = 0.1.

## Quality Metrics

- Track count (total and confirmed)
- Split/merge count
- Noise track count
- Average track duration
- Average observations per track
- Classification accuracy (when labels available)

## Interactive Tuning UI

Web-based (SvelteKit) interface with parameter sliders, live preview, run
comparison visualisation, quality metric charts, and parameter recommendation.
