# Python Virtual Environment Consolidation Plan

**Status**: Implementation Plan
**Date**: November 5, 2025
**Goal**: Consolidate from dual Python venv system to single shared repository-level venv

## Problem Summary

The repository currently has **two conflicting Python virtual environment approaches**:

1. **Root-level venv** (`.venv/`) - Intended for shared use by all Python tools

   - Referenced by: Makefile data visualization targets, `scripts/venv-init.sh`, `scripts/install-data-deps.sh`
   - Used by: `data/multisweep-graph/`, plotting scripts
   - **Issue**: Not created by main setup workflows

2. **PDF Generator-specific venv** (`tools/pdf-generator/.venv/`)
   - Referenced by: Makefile PDF targets, `scripts/dev-setup.sh`, Go server, CI
   - Used by: PDF generation, formatting, linting
   - **Issue**: Isolated from other Python tools, duplicates dependencies

This creates:

- ❌ Duplicate dependency management (two `requirements.txt` files)
- ❌ Scripts that assume different venv locations
- ❌ Confusion about which Python environment to use
- ❌ Wasted disk space with duplicate packages

## Target Architecture

### Single Shared Virtual Environment

```
velocity.report/
├── .venv/                           # ✅ Single shared environment
│   ├── bin/
│   │   └── python3                  # Used by all Python scripts
│   └── lib/python3.x/site-packages/
│
├── requirements.in                  # ✅ Human-editable dependency list
├── requirements.txt                 # ✅ Pinned versions (generated)
│
├── tools/
│   ├── pdf-generator/
│   │   ├── pdf_generator/           # Python package
│   │   └── (no .venv/)              # Uses root .venv
│   └── grid-heatmap/
│       └── plot_grid_heatmap.py     # Uses root .venv
│
└── data/
    └── multisweep-graph/
        ├── plot_*.py                # Uses root .venv
        └── (no requirements.txt)    # Uses root requirements
```

### Dependency Management

1. **Single source of truth**: `requirements.in` at repository root
2. **Pinned dependencies**: `requirements.txt` (generated with `pip-compile`)
3. **All Python tools** use packages from root `.venv`
4. **One command**: `make install-python` sets up everything

## Implementation Steps

### Phase 1: Consolidate Dependencies

**1.1 Merge requirements files**

```bash
# Merge tools/pdf-generator/requirements.in into root requirements.in
# Ensure all packages are listed
```

Current root `requirements.in` has:

- pandas, matplotlib, numpy, seaborn, scipy
- requests, PyLaTeX, pytest, pytest-cov, responses

PDF generator adds (if not already present):

- black, ruff (dev tools)
- Any PDF-specific packages

**1.2 Generate consolidated requirements.txt**

```bash
pip install pip-tools
pip-compile requirements.in
```

**1.3 Verify no critical packages lost**

Compare before/after to ensure PDF generator, plotting, and data analysis tools have all dependencies.

### Phase 2: Update Makefile

**2.1 Consolidate Python variables**

```makefile
# Before (inconsistent):
VENV_PYTHON = .venv/bin/python3
PDF_DIR = tools/pdf-generator
PDF_PYTHON = $(PDF_DIR)/.venv/bin/python

# After (unified):
VENV_DIR = .venv
VENV_PYTHON = $(VENV_DIR)/bin/python3
VENV_PIP = $(VENV_DIR)/bin/pip
VENV_PYTEST = $(VENV_DIR)/bin/pytest
PDF_DIR = tools/pdf-generator
```

**2.2 Update install-python target**

```makefile
.PHONY: install-python

install-python:
	@echo "Setting up Python environment..."
	@if [ ! -d "$(VENV_DIR)" ]; then \
		python3 -m venv $(VENV_DIR); \
	fi
	@$(VENV_PIP) install --upgrade pip
	@$(VENV_PIP) install -r requirements.txt
	@echo "✓ Python environment ready at $(VENV_DIR)"
	@echo ""
	@echo "Activate with: source $(VENV_DIR)/bin/activate"
```

**2.3 Update all Python targets**

Replace references:

- `$(PDF_PYTHON)` → `$(VENV_PYTHON)`
- `$(PDF_PYTEST)` → `$(VENV_PYTEST)`
- `cd $(PDF_DIR) && .venv/bin/*` → `$(VENV_PYTHON)` or `$(VENV_PYTEST)`

Targets to update:

- `test-python`
- `test-python-cov`
- `pdf-report`
- `pdf-config`
- `pdf-demo`
- `pdf-test`
- `format-python`
- `lint-python`
- All plotting targets (already use `VENV_PYTHON`)

**2.4 Update format-python and lint-python**

```makefile
format-python:
	@echo "Formatting Python code..."
	@if [ -x "$(VENV_DIR)/bin/black" ]; then \
		"$(VENV_DIR)/bin/black" . || true; \
	elif command -v black >/dev/null 2>&1; then \
		black . || true; \
	else \
		echo "black not found; run 'make install-python' to install"; \
	fi
	# Similar for ruff

lint-python:
	@echo "Checking Python formatting..."
	@if [ -x "$(VENV_DIR)/bin/black" ]; then \
		"$(VENV_DIR)/bin/black" --check .; \
	# etc.
```

### Phase 3: Update Scripts

**3.1 Update scripts/dev-setup.sh**

```bash
# Change from:
cd tools/pdf-generator
python3 -m venv .venv
source .venv/bin/activate

# To:
cd "$ROOT_DIR"
python3 -m venv .venv
source .venv/bin/activate
pip install --upgrade pip
pip install -r requirements.txt
```

**3.2 Keep scripts/venv-init.sh as-is**

Already creates root `.venv` - just update documentation to reference it as the canonical approach.

**3.3 Evaluate scripts/install-data-deps.sh**

Options:

- **Option A**: Remove it (redundant with `make install-python`)
- **Option B**: Keep it as thin wrapper that calls `make install-python`
- **Option C**: Keep it for installing additional data project deps (if needed)

**Recommendation**: Remove it. Users can run `make install-python` instead.

### Phase 4: Update Go Server

**4.1 Update internal/api/server.go**

```go
// Change from:
defaultPythonBin := filepath.Join(pdfDir, ".venv", "bin", "python")

// To:
defaultPythonBin := filepath.Join(repoRoot, ".venv", "bin", "python")
```

Update comment:

```go
// Default location (repo/.venv/bin/python) is used when
// the env var is unset.
```

### Phase 5: Update Configuration Files

**5.1 Update .vscode/settings.json**

```json
{
  "python.defaultInterpreterPath": "./.venv/bin/python"
}
```

**5.2 Update .github/workflows/python-tests.yml**

```yaml
# Change from:
- name: Setup Python environment
  run: |
    python -m venv tools/pdf-generator/.venv
    tools/pdf-generator/.venv/bin/pip install --upgrade pip
    tools/pdf-generator/.venv/bin/pip install black ruff
    echo "${{ github.workspace }}/tools/pdf-generator/.venv/bin" >> $GITHUB_PATH

# To:
- name: Setup Python environment
  run: |
    python -m venv .venv
    .venv/bin/pip install --upgrade pip
    .venv/bin/pip install -r requirements.txt
    echo "${{ github.workspace }}/.venv/bin" >> $GITHUB_PATH
```

**5.3 Update .gitignore**

Ensure `.venv/` at root is ignored:

```gitignore
# Python virtual environments
.venv/
venv/
*.pyc
__pycache__/
```

Remove specific `tools/pdf-generator/.venv` if present.

### Phase 6: Update Documentation

**6.1 Update main README.md**

Replace Python setup section:

````markdown
### Python Development

The repository uses a **single shared Python virtual environment** for all Python tools (PDF generator, data visualization, analysis scripts).

**Setup:**

```sh
make install-python  # Creates .venv and installs all dependencies
```
````

**Activate manually (optional):**

```sh
source .venv/bin/activate
```

**What's installed:**

- PDF generation: PyLaTeX, reportlab
- Data analysis: pandas, numpy, scipy
- Visualization: matplotlib, seaborn
- Testing: pytest, pytest-cov
- Formatting: black, ruff

````

**6.2 Update tools/pdf-generator/README.md**

```markdown
## Setup

The PDF generator uses the repository's shared Python environment:

```sh
# From repository root:
make install-python
````

This creates `.venv/` at the repository root with all dependencies.

## Running

All make targets use the shared environment automatically:

```sh
make pdf-config      # Create config template
make pdf-report CONFIG=config.json
make pdf-test        # Run tests
```

If you need to run scripts directly:

```sh
source .venv/bin/activate
python -m pdf_generator.cli.main config.json
```

````

**6.3 Create migration guide**

Add section to README or TROUBLESHOOTING.md:

```markdown
### Migrating from PDF Generator .venv

If you previously had `tools/pdf-generator/.venv`, you can safely remove it:

```sh
rm -rf tools/pdf-generator/.venv
make install-python  # Creates new shared .venv
````

All functionality remains the same.

````

### Phase 7: Clean Up

**7.1 Remove old PDF generator requirements files**

Options:
- **Option A**: Delete `tools/pdf-generator/requirements.in` and `requirements.txt`
- **Option B**: Keep them with a comment: "# See root requirements.in"
- **Option C**: Keep them as symlinks to root files

**Recommendation**: Option A (delete). Reduces confusion.

```bash
git rm tools/pdf-generator/requirements.in
git rm tools/pdf-generator/requirements.txt
````

**7.2 Clean up data project requirements**

`data/multisweep-graph/requirements.txt` lists unpinned deps (pandas, matplotlib, numpy).

Options:

- Delete it (use root requirements)
- Keep it as documentation of what that specific project needs

**Recommendation**: Delete it, document in README that root venv has all deps.

**7.3 Archive old scripts**

If removing `scripts/install-data-deps.sh`:

```bash
git rm scripts/install-data-deps.sh
```

Or keep with deprecation notice.

### Phase 8: Testing

**8.1 Create fresh environment**

```bash
# Remove all venvs
rm -rf .venv tools/pdf-generator/.venv

# Set up new unified environment
make install-python
```

**8.2 Test all Python functionality**

```bash
# PDF generation
make pdf-config
make pdf-test

# Data visualization (need test data)
make plot-noise-sweep FILE=test.csv
make plot-multisweep FILE=test.csv

# Grid stats (need running server)
make stats-live

# Formatting/linting
make format-python
make lint-python

# Testing
make test-python
make test-python-cov
```

**8.3 Test Go server PDF generation**

```bash
make dev-go
# Trigger PDF generation via API
# Verify it finds .venv/bin/python correctly
```

**8.4 Test CI**

Push to branch and verify GitHub Actions pass with new venv setup.

## Rollout Plan

### Step 1: Create implementation branch

```bash
git checkout -b consolidate-python-venv
```

### Step 2: Implement in order

1. Update `requirements.in` (Phase 1.1-1.3)
2. Update Makefile (Phase 2.1-2.4)
3. Update scripts (Phase 3.1-3.3)
4. Update Go server (Phase 4.1)
5. Update configs (Phase 5.1-5.3)
6. Update docs (Phase 6.1-6.3)
7. Clean up (Phase 7.1-7.3)
8. Test everything (Phase 8.1-8.4)

### Step 3: Testing checklist

- [ ] Fresh install works: `make install-python`
- [ ] PDF generation works: `make pdf-test`, `make pdf-report`
- [ ] Plotting works: All `plot-*` targets
- [ ] Stats collection works: `stats-live`, `stats-pcap`
- [ ] Formatting works: `make format-python`
- [ ] Linting works: `make lint-python`
- [ ] Testing works: `make test-python`, `make test-python-cov`
- [ ] Go server finds Python: Test PDF API endpoint
- [ ] CI passes: GitHub Actions workflow
- [ ] VS Code uses correct Python: Check interpreter status

### Step 4: Documentation review

- [ ] README.md updated with new approach
- [ ] tools/pdf-generator/README.md updated
- [ ] Migration notes added
- [ ] TROUBLESHOOTING.md updated if needed

### Step 5: Merge and communicate

1. Create PR with comprehensive description
2. Tag team members for review
3. Update Discord/Slack with migration instructions
4. Merge to main
5. Consider adding to CHANGELOG

## Benefits After Implementation

✅ **Single source of truth** for Python dependencies
✅ **Simplified setup** - one command: `make install-python`
✅ **Consistent environment** - all tools use same packages
✅ **Reduced disk usage** - no duplicate venvs
✅ **Easier maintenance** - one `requirements.txt` to update
✅ **Better CI/CD** - single environment to cache
✅ **Less confusion** - clear documentation

## Risk Mitigation

### Risk: Breaking existing workflows

**Mitigation**:

- Test thoroughly before merging
- Document migration path clearly
- Keep old structure working during transition (if needed)

### Risk: Missing dependencies

**Mitigation**:

- Carefully merge all requirements files
- Test all Python functionality
- Have rollback plan

### Risk: CI failures

**Mitigation**:

- Test CI changes in PR before merging
- Can temporarily keep both venv setups during transition

## Open Questions

1. **Should we support per-tool additional requirements?**

   - If a tool needs packages others don't, where should they be defined?
   - **Recommendation**: Add to root `requirements.in` - keep it simple

2. **How to handle version conflicts?**

   - What if PDF generator needs pandas==2.0 but plotting needs pandas==2.1?
   - **Current state**: Both use same versions, so likely not an issue
   - **Recommendation**: Use compatible version ranges in requirements.in

3. **Should scripts/install-data-deps.sh be kept?**

   - **Recommendation**: Remove it, use `make install-python` instead

4. **Keep tools/pdf-generator/requirements.txt as documentation?**
   - **Recommendation**: No, delete it. Root requirements.in is documentation.

## Success Criteria

✅ All Python scripts run successfully with root `.venv`
✅ No duplicate venv directories
✅ CI passes
✅ Documentation is clear and accurate
✅ No degradation in functionality
✅ Team members can easily set up environment

## Timeline Estimate

- **Phase 1-2 (Makefile)**: 2-3 hours
- **Phase 3-5 (Scripts/configs)**: 1-2 hours
- **Phase 6 (Documentation)**: 1-2 hours
- **Phase 7 (Cleanup)**: 30 minutes
- **Phase 8 (Testing)**: 2-3 hours
- **PR review/fixes**: 1-2 hours

**Total**: ~8-12 hours of focused work

## References

- Current Makefile: `/Makefile`
- Requirements: `/requirements.in`, `/requirements.txt`
- PDF requirements: `/tools/pdf-generator/requirements.txt`
- Setup scripts: `/scripts/dev-setup.sh`, `/scripts/venv-init.sh`
- Go server: `/internal/api/server.go` (PDF generation path)
- CI workflow: `/.github/workflows/python-tests.yml`
