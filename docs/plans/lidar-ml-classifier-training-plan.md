# LiDAR ML Classifier Training

## Status: Planned

## Summary

Train an ML model to replace rule-based classification using labelled track
data. This is Phase 4.1 of the LiDAR ML pipeline, targeting the v2.0
milestone.

## Related Documents

- [Product Roadmap](../ROADMAP.md) — milestone placement (v2.0)
- [Analysis Run Infrastructure](lidar-analysis-run-infrastructure-plan.md) — provides the run/track storage consumed by training
- [Track Labelling & Auto-Aware Tuning](lidar-track-labeling-auto-aware-tuning-plan.md) — generates the labelled data this pipeline requires
- [Parameter Tuning & Optimisation](lidar-parameter-tuning-optimisation-plan.md) — can run in parallel

---

## Dependencies

- Phase 4.0 (Track Labelling UI) must produce labelled tracks before training
  can begin.
- Analysis run infrastructure (Phase 3.7, ✅ implemented) provides the
  `lidar_run_tracks` table with `user_label` and `label_confidence` fields.

---

## Training Data Pipeline

```
Labelled Tracks (DB) → Feature Extraction → Training Dataset → Model Training → Model Deployment
```

## Feature Vector

Extract features from labelled tracks for ML training:

- **Spatial features (shape):** bounding box length/width/height averages,
  height p95 max, aspect ratios (XY, XZ)
- **Kinematic features (motion):** avg/peak/p50/p85/p95 speed, speed variance,
  max acceleration, heading variance
- **Temporal features:** duration, observation count, observations per second
- **Intensity features:** mean average, variance

## Model Approach

- **Algorithm:** RandomForest classifier (scikit-learn) as initial baseline
- **Validation:** 5-fold cross-validation, F1-weighted scoring
- **Export:** joblib serialisation with versioned metadata
- **Deployment:** Go-side `ClassifierFactory` selects between rule-based and ML
  classifiers, with automatic fallback to rule-based on error

## Implementation Location

- `tools/ml-training/features.py` — feature extraction
- `tools/ml-training/train_classifier.py` — training script
- `internal/lidar/ml_classifier.go` — Go-side model loading and inference
