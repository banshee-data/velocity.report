# EXPERIMENTS

- [data/experiments/try/](try): proposed experiments awaiting execution.
  Each file describes a hypothesis, method, acceptance criteria, and
  required resources. Promote successful experiments to [data/explore/](../explore)
  with results.

## Try form template

Use this template for new files in [data/experiments/try/](try).

```md
# Experiment: <short-title>

- **Status:** Proposed
- **Owner:** <name>
- **Related plan:** <docs/plans/... or none>
- **Created:** <Month DD, YYYY>

## Goal

<What decision this experiment informs and why it matters.>

## Hypothesis

<If we change X, then Y should improve because Z.>

## Method

1. <Step 1>
2. <Step 2>
3. <Step 3>

## Inputs and setup

- Dataset or capture: <path>
- Config/tuning: <path>
- Tools/commands: <exact commands>
- Environment: <device, OS, build>

## Success criteria

- [ ] <Metric threshold 1>
- [ ] <Metric threshold 2>
- [ ] <No regression constraint>

## Risks and controls

- Risk: <what could skew results>
- Control: <how to mitigate>

## Outputs

- Raw output paths: <paths>
- Summary artefact: <report path>
- Decision note target: <docs/DECISIONS.md entry or plan update>

## Result

<Fill after running: outcome, key numbers, pass/fail against criteria.>

## Next action

<Promote to data/explore/, iterate, or close as rejected.>
```
