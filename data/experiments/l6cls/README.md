---
area: l6cls
title: L6 classification
layers: L6
owner: david
---

# l6cls: L6 classification

Experiments on the track classifier (`internal/lidar/l6objects`): what class the tracks end with,
what that costs the metrics that gate on class, and which rules would change it. Specs are named
`l6cls0001` onwards; see the [README](../README.md). The class the following gate reads is
measured by the per-frame evaluator's `-class` section
([class evidence](../../../docs/lidar/operations/following-class-evidence-kirk-2026-10.md)).

Answered: [l6cls0001](l6cls0001.md), what the class gate holds back on the corpus
([report](../../../docs/lidar/operations/following-class-release-corpus-2026-10.md)).
