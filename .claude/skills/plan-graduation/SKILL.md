---
name: plan-graduation
description: Graduate a completed plan to a symlink, consolidate content into its canonical hub doc, and clean up metadata. Enforces the two-PR rule and plan hygiene gates.
argument-hint: "<plan-file.md> [--dry-run]"
---

# Skill: plan-graduation

Graduate a completed plan into its canonical hub doc and replace the plan file with a symlink.

## Usage

```
/plan-graduation docs/plans/my-plan.md
/plan-graduation docs/plans/my-plan.md --dry-run
```

## Prerequisites

The plan file must already have:

- `- **Status:** Complete` on `main` (merged, not just on a feature branch)
- `- **Canonical:** [hub-doc.md](relative/path/to/hub-doc.md)` pointing to an existing hub doc

The **two-PR rule** is enforced by CI (`make check-plan-hygiene`, gate G10): a plan must be marked Complete on `main` before it can be replaced with a symlink. Never complete a plan and create its symlink on the same feature branch.

## Procedure

### 1. Verify eligibility

```bash
# Confirm the plan is marked Complete on main
git show origin/main:<plan-path> | head -10

# Confirm it has a Canonical link
grep '^\- \*\*Canonical:\*\*' <plan-path>
```

If Status is not Complete on `main`, stop. The completion must land on `main` first in a separate PR.

### 2. Identify the canonical hub doc

Read the `Canonical:` metadata from the plan header. The target must be under one of these allowed hub prefixes:

| Hub              | Scope                                   |
| ---------------- | --------------------------------------- |
| `docs/lidar/`    | LiDAR pipeline, clustering, QC          |
| `docs/radar/`    | Radar pipeline, time-series             |
| `docs/ui/`       | Web frontend, macOS app, PDF generation |
| `docs/platform/` | Go packages, deploy, DB, tooling        |
| `config/`        | Config specifications                   |
| `data/`          | Maths and data specifications           |

### 3. Consolidate content (if needed)

If the plan contains authoritative content not yet in the hub doc:

1. Move or merge that content into the hub doc.
2. Update the hub doc to reflect the final implementation state.
3. Do **not** add a "Graduated plan:" link in the hub doc — graduated plan links add noise and point to symlinks that resolve back to the hub doc itself.

### 4. Replace the plan file with a symlink

```bash
# Remove the plan file
rm docs/plans/<plan-file>.md

# Create a relative symlink to the canonical hub doc
cd docs/plans
ln -s <relative-path-to-hub-doc> <plan-file>.md
cd ../..

# Verify the symlink resolves
ls -la docs/plans/<plan-file>.md
cat docs/plans/<plan-file>.md | head -5
```

The relative path must go from `docs/plans/` to the hub doc location. Common patterns:

- `../lidar/architecture/topic.md` — LiDAR architecture hub doc
- `../platform/operations/topic.md` — Platform operations hub doc
- `../platform/architecture/topic.md` — Platform architecture hub doc
- `../ui/topic.md` — UI hub doc

### 5. Verify plan hygiene

```bash
make report-plan-hygiene
make check-plan-hygiene
```

Both must pass with 0 gate violations. Advisory notes about shared canonical targets are acceptable.

### 6. Verify link integrity

```bash
python3 scripts/check-relative-links.py
```

Known limitation: the link checker resolves paths relative to the symlink location (`docs/plans/`), not the symlink target. This can produce false positives on relative links inside the hub doc when read through the symlink. These are acceptable and do not need fixing.

### 7. Commit

Use the standard commit format:

```
[ai][docs] graduate <plan-name> to symlink
```

Stage only the changed files:

```bash
git add docs/plans/<plan-file>.md
git add <hub-doc-path>  # if content was consolidated
git commit -m "[ai][docs] graduate <plan-name> to symlink"
```

## Batch graduation

When graduating multiple plans in one PR:

1. All plans must already be Complete on `main`.
2. Group symlink creations by hub (LiDAR batch, Platform batch, etc.).
3. Run `make report-plan-hygiene` after each batch to catch issues early.
4. Update `docs/platform/architecture/canonical-plan-graduation.md` §Current State with the new symlink count.

## Hub doc cleanup rules

When a plan graduates:

- **Do not** add "Graduated plan:" links to hub docs. These links point to symlinks that resolve back to the hub doc itself — circular and unhelpful.
- **Do** remove any existing "Active plan:" line for that plan if the plan is no longer active.
- **Do** keep "Active plan:" lines for other plans that are still in progress against the same hub doc.

## Gates enforced by CI

`scripts/check-plan-canonical-links.py` enforces 8 gates via `make check-plan-hygiene`:

| #   | Gate                                                        |
| --- | ----------------------------------------------------------- |
| 1   | Non-symlink plan missing `- **Canonical:**` link            |
| 2   | `Canonical` target points under `docs/plans/`               |
| 3   | `Canonical` target points outside repo or to a missing file |
| 5   | Symlink resolves under `docs/plans/`                        |
| 6   | Symlink resolves outside the repository                     |
| 7   | Symlink resolves to a missing target                        |
| 8   | `Canonical` link appears more than once in the same header  |
| 9   | `Canonical` target not under an allowed hub prefix          |
| 10  | Symlink created before plan was Complete on `main`          |

Gate 4 (two plans sharing the same canonical target) is advisory only — some hub docs legitimately serve multiple plans.

## Dry-run mode

When `--dry-run` is specified, perform steps 1–2 and report what would happen without making changes. Output:

```markdown
## Graduation dry-run: <plan-file>.md

- **Status on main:** Complete ✅ / Not Complete ❌
- **Canonical target:** <path>
- **Target exists:** yes/no
- **Content to consolidate:** yes (list sections) / no
- **Symlink command:** ln -s <relative-path> <plan-file>.md
```
