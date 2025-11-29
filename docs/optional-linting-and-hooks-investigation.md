# Investigation: Optional Linting CI & Pre-commit Hooks

## Executive Summary

This document investigates three improvements for developer onboarding:

1. **Making linting CI jobs optional (advisory)** — Allow PRs to merge even if formatting isn't perfect
2. **Making pre-commit hooks opt-in** — Simplify onboarding while keeping hooks available for developers who want them
3. **Automated weekly lint jobs** — Dependabot-style scheduled workflow to auto-fix formatting issues

These changes reduce friction for new contributors while maintaining code quality for seasoned developers who prefer automated formatting.

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

## Recommendation 2: Make Pre-commit Hooks Opt-in (Not Removed)

### Why Make Hooks Opt-in?

Pre-commit hooks provide value for seasoned developers who want automatic formatting on every commit. However, they create friction for new contributors:

1. **Extra installation step** — `pip install pre-commit && pre-commit install`
2. **Environment complexity** — Needs Python even for Go/Web contributors
3. **First-run delay** — Downloads hook environments on first commit
4. **Failure confusion** — Hooks failing can block commits unexpectedly

**Solution:** Keep hooks available but make them opt-in, not required.

### Benefits for Seasoned Developers

Developers who enable pre-commit hooks get:

- **Automatic formatting** — Code is cleaned on every commit
- **Faster feedback** — Catch issues before CI runs
- **Consistent commits** — No "fix formatting" follow-up commits
- **File hygiene** — Trailing whitespace, large files, EOF newlines

### Migration Path

#### Step 1: Keep full `.pre-commit-config.yaml`

**Do NOT simplify or remove the config.** Keep all hooks for developers who want them. The existing `.pre-commit-config.yaml` in the repository should remain unchanged — it already has the complete configuration with all necessary options (`args`, `language`, `stages`, etc.).

#### Step 2: Update documentation to present hooks as optional

Update `README.md` to present hooks as opt-in:

```markdown
### Code Formatting

**Option 1: Format on demand (recommended for new contributors)**
```sh
make format        # Format all code before commit
```

**Option 2: Editor integration**
- VS Code: Install Prettier, ESLint, Go extensions
- Format-on-save handles most cases

**Option 3: Pre-commit hooks (recommended for regular contributors)**
```sh
pip install pre-commit
pre-commit install
```
Hooks auto-format code on every commit — no manual `make format` needed.
```

#### Step 3: Update dev-setup.sh messaging

Update `scripts/dev-setup.sh` to present hooks as optional but valuable:

```bash
# In print_next_steps():
echo -e "${BLUE}Code Formatting:${NC}"
echo "  make format                # Format code before committing"
echo ""
echo -e "${BLUE}Optional: Enable pre-commit hooks${NC}"
echo "  pip install pre-commit && pre-commit install"
echo "  # Auto-formats code on every commit (recommended for regular contributors)"
```

### Recommended Approach

1. **Keep full `.pre-commit-config.yaml`** — Don't remove any hooks
2. **Update documentation** to present hooks as opt-in
3. **Make CI lint advisory** — Safety net for contributors without hooks
4. **Add weekly auto-fix workflow** — Clean up any missed formatting (see below)

---

## Recommendation 3: Automated Weekly Lint Jobs (Dependabot-style)

### Why Automated Lint Jobs?

With advisory lint CI (yellow warnings instead of red failures), formatting issues may accumulate over time. A weekly automated workflow solves this by:

1. **Auto-fixing formatting** — Runs `make format` on the entire codebase
2. **Creating PRs automatically** — Like Dependabot for dependencies
3. **No developer action needed** — Formatting is cleaned up without manual intervention
4. **Low friction** — Contributors don't need to remember `make format`

### Proposed Workflow: `lint-autofix.yml`

Create `.github/workflows/lint-autofix.yml`:

```yaml
name: 🧹 Weekly Lint Auto-fix

on:
  schedule:
    # Cron format: minute hour day-of-month month day-of-week
    # Run every Monday at 6:00 AM UTC
    - cron: '0 6 * * 1'
  workflow_dispatch:  # Allow manual trigger

permissions:
  contents: write
  pull-requests: write

jobs:
  autofix:
    name: Auto-fix Formatting
    runs-on: ubuntu-latest
    steps:
      - name: Checkout code
        uses: actions/checkout@v4
        with:
          ref: main
          fetch-depth: 0

      - name: Setup Go
        uses: actions/setup-go@v4
        with:
          go-version: "1.21"

      - name: Setup Python
        uses: actions/setup-python@v5
        with:
          python-version: "3.14"

      - name: Setup Node.js
        uses: actions/setup-node@v4
        with:
          node-version: "20"

      - name: Setup pnpm
        uses: pnpm/action-setup@v4

      - name: Install Python dependencies
        run: |
          python -m venv .venv
          .venv/bin/pip install --upgrade pip
          .venv/bin/pip install -r requirements.txt

      - name: Install web dependencies
        run: cd web && pnpm install --frozen-lockfile

      - name: Run make format
        run: make format

      - name: Check for changes
        id: changes
        run: |
          if [[ -n $(git status --porcelain) ]]; then
            echo "has_changes=true" >> $GITHUB_OUTPUT
          else
            echo "has_changes=false" >> $GITHUB_OUTPUT
          fi

      - name: Create Pull Request
        if: steps.changes.outputs.has_changes == 'true'
        uses: peter-evans/create-pull-request@v7
        with:
          token: ${{ secrets.GITHUB_TOKEN }}
          commit-message: "[ci] Auto-fix formatting issues"
          title: "🧹 Weekly formatting cleanup"
          body: |
            This PR was automatically created by the weekly lint auto-fix workflow.

            ## Changes
            - Ran `make format` to fix formatting issues across the codebase
            - Go: `gofmt -s -w .`
            - Python: `black .` + `ruff check --fix .`
            - Web: `prettier --write .`

            ## Why this PR exists
            CI lint jobs are advisory (non-blocking) to reduce friction for contributors.
            This weekly cleanup ensures the codebase stays consistently formatted.

            ---
            _This PR can be merged without review if CI passes._
          branch: ci/weekly-lint-autofix
          delete-branch: true
          labels: |
            automated
            formatting
```

### How It Works

1. **Schedule:** Runs every Monday at 6:00 AM UTC (configurable)
2. **Format:** Runs `make format` which invokes Go, Python, and Web formatters
3. **Detect changes:** Checks if any files were modified
4. **Create PR:** If changes exist, creates a PR with clear description
5. **Auto-merge:** The PR can be merged without review if CI passes

### Configuration Options

| Option | Default | Description |
|--------|---------|-------------|
| Schedule | `0 6 * * 1` | Every Monday at 6 AM UTC |
| Branch name | `ci/weekly-lint-autofix` | Target branch for PR |
| Labels | `automated`, `formatting` | PR labels for filtering |
| Auto-merge | Manual | Can enable auto-merge via branch rules |

### Enabling Auto-merge (Optional)

For fully hands-off operation, enable auto-merge for the automated PR:

1. In repository settings, enable "Allow auto-merge"
2. Set branch protection to require CI checks but not reviews for `automated` PRs
3. Or use the `peter-evans/enable-pull-request-automerge@v3` action

### Benefits

| Benefit | Description |
|---------|-------------|
| **Consistent codebase** | Formatting issues cleaned up weekly |
| **No manual intervention** | Developers don't need to run `make format` |
| **Visible history** | Each cleanup is a single PR with clear description |
| **Low noise** | Only creates PR if there are actual changes |
| **Flexible schedule** | Can be daily, weekly, or triggered manually |

---

## Implementation Checklist

### Phase 1: Make CI Lint Jobs Advisory

- [x] Update `go-ci.yml`: Add `continue-on-error: true` to `format` job
- [x] Update `web-ci.yml`: Add `continue-on-error: true` to `lint` job
- [x] Update `docs-ci.yml`: Add `continue-on-error: true` to `lint` job
- [x] Update job names to include "(advisory)" suffix

### Phase 2: Keep Pre-commit Hooks (Opt-in)

- [x] Keep full `.pre-commit-config.yaml` unchanged
- [x] Update `README.md` to present hooks as optional but recommended for regular contributors
- [x] Update `scripts/dev-setup.sh` to show hooks as optional

### Phase 3: Add Weekly Lint Auto-fix Workflow

- [x] Create `.github/workflows/lint-autofix.yml`
- [x] Configure schedule (weekly recommended)
- [ ] Test workflow manually via `workflow_dispatch`
- [ ] Consider enabling auto-merge for automated PRs

### Phase 4: Update Documentation

- [x] Update `README.md` development section
- [x] Add "Code Formatting" section with three options
- [x] Document the weekly auto-fix workflow

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

### 4. External Auto-fix Bots (e.g., Restyled.io)

Third-party services that auto-format PRs.

**Not recommended because:**
- Adds external dependency and cost
- Modifies PRs in-flight (can conflict with ongoing work)
- May surprise contributors

**Better alternative:** Weekly scheduled workflow (Recommendation 3) creates separate PRs instead of modifying existing ones.

---

## Summary

| Change | Effort | Impact | Recommendation |
|--------|--------|--------|----------------|
| `continue-on-error` for lint jobs | Low | High | ✅ Do this |
| Keep pre-commit hooks (opt-in) | Low | Medium | ✅ Do this |
| Weekly lint auto-fix workflow | Medium | High | ✅ Do this |
| Update documentation | Low | High | ✅ Do this |

### For New Contributors

1. Clone the repo
2. Make changes
3. Run `make format` (optional)
4. Commit and push
5. PR passes tests (lint is advisory)

No mandatory global tools, no hook setup, no surprises.

### For Seasoned Developers

1. Clone the repo
2. Run `pip install pre-commit && pre-commit install`
3. Make changes and commit — formatting happens automatically
4. Benefit from clean, consistent commits

### For the Codebase

1. Weekly auto-fix workflow catches any missed formatting
2. Creates a single PR with all fixes (like Dependabot)
3. Codebase stays consistently formatted without manual intervention
