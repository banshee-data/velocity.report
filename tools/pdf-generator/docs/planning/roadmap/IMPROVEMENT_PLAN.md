# Improvement Plan: velocity.report

**Status**: Draft for Review
**Created**: 2025-01-XX
**Context**: Post-restructure quality improvements and Python/Go integration documentation

## Executive Summary

This plan outlines a multi-phase approach to improve code quality, test coverage, and documentation for the velocity.report project following the successful Python PDF generator restructure. The project now has a clear separation between Go server components and Python tooling, which needs to be better documented and integrated.

**Current State**:
- ✅ Python PDF generator: 451/451 tests passing (100%)
- ✅ Test coverage: 86% overall (2050 statements, 287 uncovered)
- ✅ PDF generation: Working (fonts + map rendering fixed)
- ⚠️ Documentation: Root README needs Python/Go separation
- ❌ Coverage gaps: 2 CLI tools at 0%, several modules <90%

**Goals**:
- Achieve 95%+ test coverage for core modules
- Document Python/Go integration patterns
- Update root documentation for new structure
- Improve developer onboarding experience

---

## Coverage Analysis Results

### Overall Coverage: 86%

```
Name                                       Stmts   Miss  Cover   Missing
------------------------------------------------------------------------
pdf_generator/__init__.py                      3      0   100%
pdf_generator/cli/__init__.py                  0      0   100%
pdf_generator/cli/create_config.py            21     21     0%   ← CRITICAL
pdf_generator/cli/demo.py                    123    123     0%   ← CRITICAL
pdf_generator/cli/main.py                    228     14    94%
pdf_generator/core/__init__.py                 0      0   100%
pdf_generator/core/api_client.py              27      0   100%
pdf_generator/core/chart_builder.py          384     67    83%   ← IMPROVE
pdf_generator/core/chart_saver.py             70      3    96%
pdf_generator/core/config_manager.py         248      2    99%
pdf_generator/core/data_transformers.py       63      1    98%
pdf_generator/core/date_parser.py             53      3    94%
pdf_generator/core/dependency_checker.py     132     10    92%
pdf_generator/core/document_builder.py        76      0   100%
pdf_generator/core/map_utils.py              167     11    93%
pdf_generator/core/pdf_generator.py          120      1    99%
pdf_generator/core/report_sections.py         86      4    95%
pdf_generator/core/stats_utils.py             73      4    95%
pdf_generator/core/table_builders.py         168     22    87%   ← IMPROVE
pdf_generator/tests/conftest.py                8      1    88%
------------------------------------------------------------------------
TOTAL                                       2050    287    86%
```

### Priority Modules for Coverage Improvement

#### Tier 1: Critical (0% Coverage)
1. **create_config.py** (21/21 uncovered) - Config template generation
2. **demo.py** (123/123 uncovered) - Interactive demo

#### Tier 2: High Priority (<90% Coverage)
3. **chart_builder.py** (83% coverage, 67 uncovered lines)
4. **table_builders.py** (87% coverage, 22 uncovered lines)
5. **conftest.py** (88% coverage, 1 uncovered line)

#### Tier 3: Medium Priority (90-95% Coverage)
6. **dependency_checker.py** (92% coverage, 10 uncovered lines)
7. **map_utils.py** (93% coverage, 11 uncovered lines)
8. **date_parser.py** (94% coverage, 3 uncovered lines)

---

## Documentation Review Findings

### Root README (`/README.md`)

**Issues Identified**:
1. ❌ **No mention of Python tooling** - Only discusses Go server
2. ❌ **No project structure overview** - Doesn't explain tools/, web/, internal/
3. ❌ **Missing development setup** - No Python environment setup instructions
4. ❌ **Limited deployment docs** - Only Go service deployment
5. ❌ **No link to subdirectory READMEs** - Doesn't reference tools/pdf-generator/README.md

**Current Content**:
- ASCII art branding ✅
- Go server build instructions ✅
- Go deployment (systemd) ✅
- **MISSING**: Python setup, PDF generator, web frontend, project overview

### PDF Generator README (`/tools/pdf-generator/README.md`)

**Strengths**:
- ✅ Comprehensive (613 lines)
- ✅ Clear quick start guide
- ✅ Detailed configuration examples
- ✅ All Makefile commands documented
- ✅ Module structure explained

**Potential Improvements**:
- Could add troubleshooting section
- Could add performance considerations
- Could document Go integration patterns

### Other Documentation Files

Found 126+ .md files across the project:
- `/data/README.md` - Data directory documentation
- `/web/README.md` - Web frontend documentation
- `/docs/` - Various feature documentation
- `/tools/pdf-generator/docs/` - Phase completion docs (restructure history)

**Gap**: No unified index or navigation between these documents.

---

## Multi-Phase Improvement Plan

### Phase A: Test Coverage - CLI Tools (Est: 4-6 hours)

**Objective**: Achieve 100% coverage for CLI tools (create_config.py, demo.py)

**Tasks**:
1. Add tests for `create_config.py`:
   - [ ] Test template generation (minimal vs full)
   - [ ] Test file writing
   - [ ] Test CLI argument parsing
   - [ ] Test error handling (permissions, invalid paths)

2. Add tests for `demo.py`:
   - [ ] Test interactive prompts (mock input)
   - [ ] Test configuration building
   - [ ] Test validation
   - [ ] Test example generation

**Deliverables**:
- `pdf_generator/tests/test_create_config.py`
- `pdf_generator/tests/test_demo.py`
- Coverage: 0% → 100% for both files

**Success Criteria**:
- All new tests pass
- Coverage increases to ~92% overall (from 86%)
- No regressions in existing tests

---

### Phase B: Test Coverage - Core Modules (Est: 8-10 hours)

**Objective**: Improve core module coverage to 95%+

**Tasks**:

1. **chart_builder.py** (83% → 95%):
   - [ ] Add tests for edge cases in lines 250-258, 273-274
   - [ ] Test error handling paths (449-454, 470-471, etc.)
   - [ ] Test histogram generation edge cases
   - [ ] Test chart styling variations

2. **table_builders.py** (87% → 95%):
   - [ ] Test edge cases in histogram table computation
   - [ ] Test error handling for malformed data
   - [ ] Test table formatting variations
   - [ ] Test parameter table edge cases

3. **dependency_checker.py** (92% → 95%):
   - [ ] Test missing dependency scenarios
   - [ ] Test version checking logic
   - [ ] Test import fallbacks

4. **map_utils.py** (93% → 95%):
   - [ ] Test SVG processing errors
   - [ ] Test marker positioning edge cases
   - [ ] Test coordinate transformation edge cases

**Deliverables**:
- Extended test files for each module
- Coverage: 86% → 95%+ overall

**Success Criteria**:
- All modules >95% coverage
- Edge cases documented in test names
- No regressions

---

### Phase C: Root Documentation Restructure (Est: 4-6 hours)

**Objective**: Update root README to reflect Python/Go architecture

**Tasks**:
1. [ ] Rewrite root README.md structure:
   ```markdown
   # velocity.report

   ## Overview
   - Project description
   - Architecture overview (Go server + Python tools + Web UI)

   ## Quick Start
   - Go Server (development)
   - Python PDF Generator (see tools/pdf-generator/)
   - Web Frontend (see web/)

   ## Project Structure
   - /internal/ - Go server internals
   - /cmd/ - Go CLI commands
   - /tools/ - Python tooling (PDF generator, etc.)
   - /web/ - Svelte frontend
   - /docs/ - Documentation
   - /data/ - Data and migrations

   ## Development Setup
   - Go prerequisites
   - Python prerequisites (for PDF generation)
   - Database setup

   ## Deployment
   - Go service deployment
   - Python tooling deployment
   - Web frontend deployment

   ## Documentation Index
   - Link to tools/pdf-generator/README.md
   - Link to web/README.md
   - Link to docs/
   ```

2. [ ] Create `/ARCHITECTURE.md`:
   - System architecture diagram
   - Component relationships
   - Data flow (sensor → Go → SQLite → API → Python/Web)

3. [ ] Create `/CONTRIBUTING.md`:
   - Go development workflow
   - Python development workflow
   - Testing requirements
   - PR process

**Deliverables**:
- Updated `/README.md`
- New `/ARCHITECTURE.md`
- New `/CONTRIBUTING.md`
- Documentation index at `/docs/README.md`

**Success Criteria**:
- Clear separation of Go/Python concerns
- Easy navigation to component-specific docs
- Developer can onboard in <15 minutes

---

### Phase D: Go Integration Documentation (Est: 3-4 hours)

**Objective**: Document Go→Python integration patterns (Phase 10 prep)

**Tasks**:
1. [ ] Create `/tools/pdf-generator/GO_INTEGRATION.md`:
   - How Go calls Python PDF generator
   - Path resolution patterns
   - Error handling between Go/Python
   - Example Go code snippets

2. [ ] Document current Go→Python integration points:
   - Where in Go codebase PDF generation is triggered
   - What data is passed
   - How errors are surfaced

3. [ ] Create migration guide for Phase 10:
   - Files to update in Go codebase
   - Path changes required
   - Testing strategy
   - Rollback plan

**Deliverables**:
- `/tools/pdf-generator/GO_INTEGRATION.md`
- `/docs/PHASE_10_MIGRATION_GUIDE.md`

**Success Criteria**:
- Go developers can understand integration without Python expertise
- Clear migration path for Phase 10
- Examples are executable

---

### Phase E: Developer Experience Improvements (Est: 6-8 hours)

**Objective**: Streamline developer onboarding and daily workflows

**Tasks**:
1. [x] Create unified setup script:
   ```bash
   # /scripts/dev-setup.sh
   # - Check Go version
   # - Check Python version
   # - Setup Go dependencies
   # - Setup Python venv for PDF generator
   # - Initialize database
   # - Run health checks
   ```

2. [x] Add troubleshooting documentation:
   - `/tools/pdf-generator/TROUBLESHOOTING.md`:
     - Font rendering issues
     - Map generation issues
     - LaTeX compilation errors
     - Common test failures

3. [ ] Improve error messages:
   - Add context to Python exceptions
   - Add hints for common configuration errors
   - Add LaTeX error translation

4. [x] Add pre-commit hooks:
   - Go formatting + linting (gofmt, goimports, golangci-lint)
   - Python formatting (ruff, black)
   - Web lint pipeline (`pnpm lint`)
   - Repo hygiene checks (trailing whitespace, large files)

**Deliverables**:
- `/scripts/dev-setup.sh`
- `/tools/pdf-generator/TROUBLESHOOTING.md`
- Improved error messages in Python code
- `.pre-commit-config.yaml`

**Success Criteria**:
- New developer can run `make dev-setup` and have working environment
- Common errors have helpful messages
- Pre-commit prevents broken commits

---

### Phase F: CI/CD Documentation (Est: 2-3 hours)

**Objective**: Document CI/CD strategy for Python/Go monorepo

**Tasks**:
1. [ ] Create `/docs/CI_CD.md`:
   - Current CI setup (if any)
   - Proposed GitHub Actions workflows
   - Testing strategy (Go + Python)
   - Deployment automation

2. [ ] Document release process:
   - Versioning strategy
   - Changelog management
   - Release artifact generation
   - Deployment checklist

3. [ ] Create example GitHub Actions workflow:
   ```yaml
   # .github/workflows/test.yml
   # - Test Go code
   # - Test Python PDF generator
   # - Build artifacts
   # - Run integration tests
   ```

**Deliverables**:
- `/docs/CI_CD.md`
- `/docs/RELEASE_PROCESS.md`
- Example workflow file (not activated unless user wants)

**Success Criteria**:
- Clear understanding of CI/CD strategy
- Reusable workflow examples
- Documentation for future automation

---

## Execution Strategy

### Recommended Order

1. **Phase A** (CLI coverage) - Quick wins, high impact
2. **Phase C** (Root docs) - Unblocks developer onboarding
3. **Phase B** (Core coverage) - Longer, can be incremental
4. **Phase E** (DevEx) - Parallel with Phase B
5. **Phase D** (Go integration) - Prep for future work
6. **Phase F** (CI/CD) - Lower priority, can be deferred

### Parallel Execution Opportunities

- Phase C + Phase A can run in parallel (different files)
- Phase E + Phase B can run in parallel (different focus)
- Phase D + Phase F can run in parallel

### Effort Estimation

| Phase | Estimated Hours | Priority | Dependencies |
|-------|----------------|----------|--------------|
| A     | 4-6            | High     | None         |
| B     | 8-10           | High     | Phase A      |
| C     | 4-6            | Critical | None         |
| D     | 3-4            | Medium   | Phase C      |
| E     | 6-8            | High     | Phase C      |
| F     | 2-3            | Low      | None         |
| **Total** | **27-37 hours** | | |

### Milestones

1. **M1**: Complete Phase A + C (Root docs updated, CLI fully tested)
2. **M2**: Complete Phase B + E (Core coverage >95%, DevEx improved)
3. **M3**: Complete Phase D + F (Go integration documented, CI/CD ready)

---

## Risk Assessment

### Low Risk
- ✅ **Phase A (CLI tests)**: Isolated, no dependencies
- ✅ **Phase C (Root docs)**: Documentation only, no code changes

### Medium Risk
- ⚠️ **Phase B (Core coverage)**: Touching complex modules, regression risk
  - **Mitigation**: Run full test suite after each change

- ⚠️ **Phase E (DevEx)**: Setup scripts can break environments
  - **Mitigation**: Test on clean VM/container first

### High Risk
- 🔴 **Phase D (Go integration)**: Requires Go codebase changes (deferred to separate PR)
  - **Mitigation**: Documentation only for now, actual code changes in Phase 10

---

## Success Metrics

### Quantitative
- [ ] Test coverage: 86% → 95%+
- [ ] Documentation coverage: All major components documented
- [ ] Setup time: >60min → <15min (new developer)
- [ ] Build success rate: Track in CI (baseline TBD)

### Qualitative
- [ ] Developer feedback: Onboarding difficulty rated "easy"
- [ ] Documentation clarity: Developers can find answers without asking
- [ ] Error messages: Developers can self-resolve issues

---

## Phase Completion Criteria

Each phase is considered complete when:

1. **All tasks checked off**
2. **Tests pass**: 451/451 (or more) passing
3. **Coverage target met**: Specific to phase
4. **Documentation updated**: Changes reflected in relevant docs
5. **Peer reviewed**: At least one other developer reviews
6. **No regressions**: Existing functionality unchanged

---

## Deferred Items (Future Work)

These items are out of scope for this improvement plan but should be tracked:

1. **Phase 10**: Go codebase integration (separate PR)
2. **Performance optimization**: PDF generation speed improvements
3. **Web UI integration**: Connect web frontend to PDF generator
4. **Automated testing**: Full integration test suite
5. **Docker containerization**: Development environment in Docker
6. **API documentation**: OpenAPI/Swagger for Go server

---

## Appendix A: Coverage Gaps Detail

### create_config.py (0% coverage, 21 lines)
**Missing tests**:
- Template generation logic
- File I/O operations
- CLI argument parsing
- Error handling

**Recommended tests**:
```python
def test_create_config_minimal():
    """Test minimal config template generation"""

def test_create_config_full():
    """Test full config template with all fields"""

def test_create_config_file_write():
    """Test config written to correct path"""

def test_create_config_permission_error():
    """Test handling of write permission errors"""
```

### demo.py (0% coverage, 123 lines)
**Missing tests**:
- Interactive prompt handling
- Input validation
- Configuration building
- Demo mode execution

**Recommended tests**:
```python
@patch('builtins.input')
def test_demo_interactive_flow(mock_input):
    """Test full interactive demo flow"""

def test_demo_config_generation():
    """Test demo generates valid config"""

def test_demo_input_validation():
    """Test demo validates user inputs"""
```

### chart_builder.py (83% coverage, 67 uncovered lines)
**Uncovered lines**: 80, 250-258, 273-274, 369, 385-387, 449-454, ...

**Missing scenarios**:
- Error handling in chart generation
- Edge cases in histogram buckets
- Font fallback scenarios
- Empty data handling
- Invalid configuration handling

### table_builders.py (87% coverage, 22 uncovered lines)
**Uncovered lines**: 63, 180, 284, 327, 389, 439, 448-449, 473-474, 517-544

**Missing scenarios**:
- Edge cases in range computation
- Empty histogram data
- Maximum bucket handling
- Table formatting edge cases

---

## Appendix B: Documentation File Inventory

### Root Level
- `/README.md` - Main project README (needs update)
- `/CODE_OF_CONDUCT.md` - Community guidelines ✅
- `/LICENSE` - MIT license ✅
- `/devlog.md` - Development log

### Tools
- `/tools/pdf-generator/README.md` - PDF generator docs ✅
- `/tools/pdf-generator/REQUIRED_FIELDS.md` - Config field reference
- `/tools/pdf-generator/MAP_GENERATION_FIX.md` - Bug fix history
- `/tools/pdf-generator/docs/` - Phase completion docs

### Components
- `/web/README.md` - Web frontend docs
- `/cmd/radar/README.md` - Radar CLI docs
- `/data/README.md` - Data directory docs
- `/data/align/README.md` - Data alignment docs
- `/data/migrations/README.md` - Migration docs

### Documentation Hub
- `/docs/README.md` - Documentation index (needs creation)
- `/docs/FRONTEND_UNITS_FEATURE.md` - Feature documentation
- `/internal/lidar/docs/` - LIDAR documentation

**Gaps**:
- No `/ARCHITECTURE.md`
- No `/CONTRIBUTING.md`
- No `/docs/README.md` (index)
- No `/tools/pdf-generator/TROUBLESHOOTING.md`
- No `/tools/pdf-generator/GO_INTEGRATION.md`

---

## Appendix C: Execution Checklist

### Before Starting
- [ ] Review and approve this plan
- [ ] Create tracking issues for each phase
- [ ] Set up project board (if desired)
- [ ] Assign phases to developers

### During Execution
- [ ] Create feature branch for each phase
- [ ] Update this document with actual progress
- [ ] Track time spent vs estimates
- [ ] Document blockers/issues

### After Completion
- [ ] Generate final coverage report
- [ ] Update all documentation links
- [ ] Create summary document
- [ ] Celebrate! 🎉

---

## Questions for Review

Please review and provide feedback on:

1. **Phase Priority**: Do you agree with the recommended order (A→C→B→E→D→F)?
2. **Scope**: Is anything missing? Should anything be removed/deferred?
3. **Effort**: Are the time estimates reasonable based on your experience?
4. **Coverage Targets**: Is 95% the right target, or should we aim for 90% or 98%?
5. **Go Integration (Phase D)**: Should this be prioritized higher to unblock Phase 10?
6. **Documentation Structure**: Any preferences for how docs should be organized?

---

**Next Steps**:
1. Review this plan
2. Provide feedback/alterations
3. Approve phases to execute
4. Begin with Phase A (or your preferred starting point)
