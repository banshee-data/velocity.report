# S3 likelihood cost and containment: cloud comparison

- **Status:** Registered; development replays pending, full acceptance open
- **Layers:** L5 association
- **Experiment:** [l5id0011](../../../data/experiments/l5id/l5id0011.md)

## Question and arms

Can the likelihood cost charge an uncertain coasting track for its covariance
without losing vehicles, with containment deciding which competing bodies can
claim a cluster? All four arms use the same current-default tracker plus a solid-body shadow,
containment and complete processed cluster membership. `a0` disables both S3
options, `lik` enables likelihood cost, `claim` enables containment claim and
`lik-claim` enables both. The control is an experimental geometry control, not
the shipped default. Identity and W3 candidates remain off in every arm.

The checked-in `association-*.json` files, startup and replay aliases map to the
same tracker settings. Containment claim requires an enabled contained body and
full membership. It chooses between bidders and refuses none if every bidder
fails, so that this rule alone cannot send a contested cluster to seed a birth.

## Predeclared rule and cloud scope

The approved rule requires at least one-third fewer duplicate pairs per hundred
published vehicle tracks pooled over 23 first-segment sites and two kirk captures;
corpus vehicle-sized scored frames must remain at least 95 % of control. Kirk0
reviewed-label matches must remain at least 98 %, HOTA and IDF1 no more than 0.005
lower, switches no higher and fragmentation no higher than control × 1.1 + 1.
Only tuning-pass candidates are scored once on label0001's frozen held-out window,
where the labelled limits must hold again. These levels are unchanged.

The cloud execution registration was committed before new replays. Its available
screen is whole kirk0, the committed corpus/index and a six-second warm-up with
settling not required, matching the preceding development screens. Every arm
must repeat identically. This capture cannot pass the pooled or held-out rule,
qualify background settling, or establish physical geometry accuracy.

## Results and provenance

Pending the merged baseline and stamped replay tools. Raw output stays outside
Git; final numerical measurements, parameter hashes and build/source identifiers
will be recorded here. No candidate is adopted from an incomplete screen.
