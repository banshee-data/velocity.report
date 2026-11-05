# Containment claims with an assignment-cardinality safeguard

- **Status:** Engineering implementation; development screen in progress; default off
- **Experiment:** [l5id0018](../../../data/experiments/l5id/l5id0018.md)

The [original S3 screen](l5-likelihood-containment-2026-10.md) exposed a counterexample to the
claim's local guard: keeping a bidder in each cluster row can still reduce the number of global
assignments. This candidate tentatively applies the same refusals, then compares maximum
matching cardinality with the original matrix. If it fell, the whole matrix is restored.
Originally forbidden edges stay forbidden and surviving costs are unchanged. Accepted refusals
and whole-matrix fallbacks have separate continuity counters.

The optional `association.cardinality_guard` block enables this behaviour for startup and replay.
It is absent by default; the original four S3 files and shipped tuning fingerprint are unchanged.
The guard preserves matching size at this stage, not identity correctness or a no-birth invariant
for the entire lifecycle. A fallback can also discard otherwise useful refusals in the same
matrix; this conservative choice keeps the invariant simple and deterministic.

The preregistered development screen compares five checked-in parameter sets at one source
revision: contained-body control, claim, guarded claim, likelihood with claim, and likelihood
with guarded claim. All use kirk0 with six seconds of warm-up and the same evidence sampling.
The reviewed tuning labels are reconstructed from the frozen Hugging Face text files and raw
capture; exact packet and return digests must verify. This is development data, not independent
held-out qualification. No default promotion is permitted by this screen.
