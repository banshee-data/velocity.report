---
name: decision-review-voice
description: >-
  Discuss a prepared decision-review packet using a lightweight model in voice or text, and save
  explicitly confirmed choices as ready for integration. Use after high-capability preparation;
  does not apply downstream changes.
argument-hint: "<path/to/decision-review-packet.md>"
model: haiku
---

# Skill: decision-review-voice

Read [the canonical decision-review procedure](../decision-review/SKILL.md), then execute its
**Phase 2** and shared record rules only. Read the procedure as a file; do not invoke
`/decision-review`, which requests the preparation model. Use the packet supplied by the user.
If none is supplied, locate the active packet from the current PR or branch; clarify only if
several remain plausible. If none exists, return the preparation invocation.

Use the host's available voice controls, or discuss in text with the modality recorded. This
entry point requests a lightweight model; it does not enable voice or claim a model switch.
