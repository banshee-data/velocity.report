---
area: host
title: Roles, hosts and reproducibility
layers: Cross-cutting (binary roles, worker, replay)
owner: david
---

# host: roles, hosts and reproducibility

Experiments on what the one binary costs in each role, whether a worker reproduces a built tool,
and whether one capture under one fingerprint gives the same rows on every host. They put numbers
on the [architecture review](../../../docs/plans/platform-architecture-review-plan.md). Specs are
named `host0001` onwards; see the [README](../README.md).

The Pi steps run on a Pi 5 on the bench. The deployed device of record is a Pi 4 (4 GB), per the
[hardware module](../../../.github/knowledge/hardware.md), so a Pi 5 result does not describe a
deployment until the owner decides which device gates a release: the review's [open
questions](../../../docs/plans/platform-architecture-review-plan.md#open-questions-for-the-owner)
carry it. Every arm names `config/tuning.defaults.json` and pins its fingerprint in the spec's
body, as the experiment queue plan's [Section 12](../../../docs/plans/platform-experiment-queue-plan.md#12-keeping-a-spec-true-to-what-ran)
will require of every spec.
