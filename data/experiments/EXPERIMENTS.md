# EXPERIMENTS

- [data/experiments/try/](try): proposed experiments awaiting execution.
  Each file describes a hypothesis, method, acceptance criteria, and
  required resources. Promote successful experiments to [data/explore/](../explore)
  with results.

## Try form template

Use this template for new files in [data/experiments/try/](try). Each experiment file should contain the following sections:

| Section                | Content                                                               |
| ---------------------- | --------------------------------------------------------------------- |
| **Title**              | `# Experiment: <short-title>`                                         |
| **Metadata**           | Status (Proposed), Owner, Related plan, Created date                  |
| **Goal**               | What decision this experiment informs and why it matters              |
| **Hypothesis**         | If we change X, then Y should improve because Z                       |
| **Method**             | Numbered steps                                                        |
| **Inputs and setup**   | Dataset/capture path, config/tuning path, exact commands, environment |
| **Success criteria**   | Checklist of metric thresholds and no-regression constraints          |
| **Risks and controls** | What could skew results and how to mitigate                           |
| **Outputs**            | Raw output paths, summary artefact, decision note target              |
| **Result**             | Fill after running: outcome, key numbers, pass/fail against criteria  |
| **Next action**        | Promote to `data/explore/`, iterate, or close as rejected             |
