# Investigation: Optional Linting CI & Removing Pre-commit Hooks

## Executive Summary

This document investigates two improvements for developer onboarding:

1. **Making linting CI jobs optional** — Allow PRs to merge even if formatting isn't perfect
2. **Removing pre-commit hooks dependency** — Simplify onboarding by eliminating mandatory local hooks

Both changes aim to reduce friction for new contributors while maintaining code quality through clear documentation and accessible tooling.

---

## Current State

### Pre-commit Hooks (`.pre-commit-config.yaml`)

The repository currently uses [pre-commit](https://pre-commit.com/) with these hooks:

| Hook | Purpose | Trigger |
|------|---------|---------|
| `check-added-large-files` | Prevents files >1MB | All commits |
| `end-of-file-fixer` | Ensures newline at EOF | All commits |
| `trailing-whitespace` | Removes trailing spaces | All commits |
| `mixed-line-ending` | Normalizes line endings | All commits |
| `format-go` | Runs `make format-go` | On `*.go` files |
| `format-python` | Runs `make format-python` | On `tools/**/*.py`, `data/**/*.py` |
| `format-web` | Runs `make format-web` | On `web/**`, `docs/**` (`.js`, `.ts`, `.svelte`, `.css`, `.json`) |

**Onboarding friction:**
- Requires `pip install pre-commit` (global or venv)
- Requires `pre-commit install` to register hooks
- Can be slow on first commit (downloads hook environments)
- Failure can confuse new contributors

### CI Linting Jobs

| Workflow | Job | Behavior | Blocking? |
|----------|-----|----------|-----------|
| `go-ci.yml` | `format` | Runs `make lint-go`, fails if unformatted | Yes |
| `python-ci.yml` | `lint` | Runs `make format-python` (auto-formats but does not fail CI) | No |
| `web-ci.yml` | `lint` | Runs `pnpm run lint` | Yes |
| `docs-ci.yml` | `lint` | Runs `prettier --check` | Yes |

**Issue:** Most lint jobs are blocking. If a contributor forgets to format, the PR cannot merge.

---

## Recommendation 1: Make Linting CI Jobs Optional

### Option A: Use `continue-on-error: true` (Recommended)

Add `continue-on-error: true` to lint jobs. The job still runs and reports results, but won't block merge.

**Example change for `go-ci.yml`:**

```yaml
format:
  name: Format (advisory)
  runs-on: ubuntu-latest
  needs: test
  continue-on-error: true  # ← Allow PR to merge even if formatting fails
  steps:
    - name: Checkout code
      uses: actions/checkout@v4
    # ... rest of steps
```

**Pros:**
- Simple, single-line change per workflow
- Lint results are visible in PR (yellow warning instead of red X)
- Tests remain blocking (critical for correctness)

**Cons:**
- Yellow warning may be ignored over time
- Could lead to inconsistent formatting if not addressed

### Option B: Create Separate Non-Required Check

Use GitHub branch protection rules to mark specific checks as non-required:

1. Keep lint jobs as separate workflow jobs
2. In repository settings → Branch protection rules → Edit rule for `main`
3. Under "Require status checks to pass", only require `test` jobs, not `lint` jobs

**Pros:**
- More granular control
- Clear separation of concerns

**Cons:**
- Requires repository admin access
- Configuration lives outside code (not version-controlled)

### Option C: Matrix Strategy with "Soft" vs "Hard" Checks

```yaml
jobs:
  quality:
    strategy:
      fail-fast: false
      matrix:
        include:
          - check: test
            required: true
          - check: lint
            required: false
    continue-on-error: ${{ matrix.required == false }}
    steps:
      - run: make ${{ matrix.check }}
```

**Pros:**
- Single job definition
- Clear documentation of what's required

**Cons:**
- More complex workflow structure
- Harder to read at a glance

### Recommended Approach

Use **Option A** (`continue-on-error: true`) because:

1. **Minimal change** — One line per workflow
2. **Visible feedback** — Contributors see the warning
3. **Tests stay critical** — Only formatting is relaxed
4. **Reversible** — Easy to make strict again if needed

**Suggested implementation for each workflow:**

```yaml
# go-ci.yml - Format job
format:
  name: Format (advisory)
  continue-on-error: true
  # ... rest unchanged

# python-ci.yml - Lint job (already non-blocking via format-only approach)
# No change needed - already runs format, doesn't check

# web-ci.yml - Lint job
lint:
  name: Lint (advisory)
  continue-on-error: true
  # ... rest unchanged

# docs-ci.yml - Lint job
lint:
  name: Lint & Format Check (advisory)
  continue-on-error: true
  # ... rest unchanged
```

---

## Recommendation 2: Remove Pre-commit Hooks Requirement

### Why Remove Hooks?

Pre-commit hooks create friction for new contributors:

1. **Extra installation step** — `pip install pre-commit && pre-commit install`
2. **Environment complexity** — Needs Python even for Go/Web contributors
3. **First-run delay** — Downloads hook environments on first commit
4. **Failure confusion** — Hooks failing can block commits unexpectedly
5. **Opt-out friction** — `git commit --no-verify` feels wrong

### Alternative: Rely on Editor + CI

Modern development workflows often eliminate pre-commit hooks by:

1. **Editor integration** — Format-on-save in VS Code, GoLand, etc.
2. **CI feedback** — Advisory lint checks in PR
3. **Manual formatting** — `make format` before commit (documented)

### Migration Path

#### Step 1: Make hooks optional (not removed)

Update `README.md` to present hooks as optional:

```markdown
### Code Formatting

**Option 1: Format on demand (recommended)**
```sh
make format        # Format all code before commit
```

**Option 2: Editor integration**
- VS Code: Install Prettier, ESLint, Go extensions
- Format-on-save handles most cases

**Option 3: Pre-commit hooks (optional)**
```sh
pip install pre-commit
pre-commit install
```
```

#### Step 2: Document `make format` prominently

Ensure `make format` is the primary documented workflow:

```markdown
## Before Committing

Run `make format` to auto-format all code:
```sh
make format    # Formats Go, Python, and Web code
make lint      # Verify formatting (what CI checks)
```
```

#### Step 3: Remove hooks from required setup

Update `scripts/dev-setup.sh` to **not** install pre-commit by default:

```bash
# Current (in the "Git Hooks" section of print_next_steps):
echo "  pre-commit install         # Enable formatting/lint hooks"

# Recommended:
echo "  make format                # Format code before committing"
echo "  make lint                  # Check formatting (optional)"
echo ""
echo "Optional: Enable pre-commit hooks"
echo "  pip install pre-commit && pre-commit install"
```

#### Step 4: Simplify `.pre-commit-config.yaml` or remove

**Option A: Keep minimal hooks (file hygiene only)**

```yaml
repos:
  - repo: https://github.com/pre-commit/pre-commit-hooks
    rev: v4.6.0
    hooks:
      - id: check-added-large-files
        args: ["--maxkb=1024"]
      - id: end-of-file-fixer
      - id: trailing-whitespace
      - id: mixed-line-ending

# Note: Language-specific formatting handled by `make format`
# Run `make format` before committing, or use editor format-on-save
```

**Option B: Remove `.pre-commit-config.yaml` entirely**

If hooks aren't used/enforced, the config file may confuse contributors who expect it to work automatically.

### Recommended Approach

1. **Keep `.pre-commit-config.yaml`** but simplify to file-hygiene only
2. **Update documentation** to recommend `make format` as primary workflow
3. **Make CI lint advisory** (not blocking)
4. **Update dev-setup.sh** to not suggest pre-commit installation by default

This provides:
- Zero mandatory setup for formatting
- Clear, simple workflow: `make format && git commit`
- CI safety net for missed formatting
- Optional hooks for those who prefer them

---

## Implementation Checklist

### Phase 1: Make CI Lint Jobs Advisory

- [ ] Update `go-ci.yml`: Add `continue-on-error: true` to `format` job
- [ ] Update `web-ci.yml`: Add `continue-on-error: true` to `lint` job
- [ ] Update `docs-ci.yml`: Add `continue-on-error: true` to `lint` job
- [ ] Update job names to include "(advisory)" suffix

### Phase 2: Simplify Pre-commit Hooks

- [ ] Remove language-specific format hooks from `.pre-commit-config.yaml`
- [ ] Keep file-hygiene hooks (trailing whitespace, large files, etc.)
- [ ] Add comment explaining `make format` workflow

### Phase 3: Update Documentation

- [ ] Update `README.md` development section
- [ ] Update `scripts/dev-setup.sh` output
- [ ] Add "Before Committing" section emphasizing `make format`

---

## Alternatives Considered

### 1. GitHub Super-Linter

[Super-Linter](https://github.com/super-linter/super-linter) runs multiple linters in one action.

**Rejected because:**
- Adds complexity (new action to maintain)
- Current make targets work well
- Doesn't solve the blocking/non-blocking question

### 2. Husky (Node.js hooks)

[Husky](https://typicode.github.io/husky/) is another git hooks framework.

**Rejected because:**
- Requires Node.js (not all contributors use it)
- Adds another tool to the stack
- Doesn't fundamentally change the onboarding friction

### 3. Git Hooks via Shell Scripts

Custom `.git/hooks/pre-commit` script.

**Rejected because:**
- Hooks aren't version-controlled by default
- Requires manual setup (`cp hooks/pre-commit .git/hooks/`)
- More maintenance burden

### 4. Strict Linting + Auto-fix PR Bot

Bot that auto-formats PRs and pushes changes.

**Rejected because:**
- Adds external dependency
- Can conflict with ongoing work
- May surprise contributors

---

## Summary

| Change | Effort | Impact | Recommendation |
|--------|--------|--------|----------------|
| `continue-on-error` for lint jobs | Low | High | ✅ Do this |
| Simplify pre-commit config | Low | Medium | ✅ Do this |
| Remove pre-commit entirely | Medium | Medium | Consider later |
| Update documentation | Low | High | ✅ Do this |

**Net effect:** New contributors can:
1. Clone the repo
2. Make changes
3. Run `make format`
4. Commit and push
5. PR passes tests (lint is advisory)

No mandatory global tools, no hook setup, no surprises.
