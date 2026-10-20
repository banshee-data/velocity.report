---
name: decision-review-integrate
description: >-
  Use high-capability reasoning to evaluate confirmed voice decisions and follow-ups, update
  downstream dependencies, and prepare a PR or return material questions to voice. Use after a
  decision-review discussion.
argument-hint: "<path/to/decision-review-packet.md>"
model: opus
---

# Skill: decision-review-integrate

Read [the canonical decision-review procedure](../decision-review/SKILL.md), then execute its
**Phase 3**, shared record rules, and validation requirements only. Read the procedure as a file;
do not restart preparation. Use the packet supplied by the user, or locate it from the current
PR or branch. Clarify only if several remain plausible. If none exists, return the preparation
invocation. If no entries are ready, return the voice handoff without inventing decisions.

This entry point requests high-capability reasoning. If that tier is unavailable, preserve the
integration handoff and report that it is pending.
