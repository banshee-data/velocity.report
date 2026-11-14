# Following evidence baseline reproducibility campaign

This report reserves the question, methods and provenance for the bounded current-baseline
following/experimental DRAC campaign. Results will be appended after execution; no run or
physical validation is claimed by this initial report.

- **Status:** Preregistered; execution pending
- **Related:** [Experiment l8fol0006](../../../data/experiments/l8fol/l8fol0006.md),
  [offline evidence exporter](following-evidence-export.md)

## Question and method

Can one explicit development source/site/version regenerate exact analytical evidence with
complete interval/exclusion accounting? Use the unchanged baseline parameter set and frozen
rule in the experiment specification. Begin with a bounded Kirk pilot and expand only when
its engineering checks and existing eligibility policy permit it.

## Arms and provenance

The sole arm is [weekend-following-baseline.json](../../../config/experiments/weekend-following-baseline.json),
an exact copy of defaults with zero changed keys. Code, binary stamps, file and semantic
fingerprints, source hashes, policy/registry identities and output archive digests will be
recorded with the executed cases.

## Findings and limitations

Pending. The campaign distinguishes same-DB export regeneration, raw pipeline repeats and
physical validation. Ordinary capture traffic and apparently plausible boxes do not establish
true vehicle geometry, identity, relative-speed accuracy or metric qualification.
