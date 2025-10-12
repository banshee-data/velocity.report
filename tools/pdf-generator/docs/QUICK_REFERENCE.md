# Quick Reference: Improvement Plan Execution

This is a condensed version of `IMPROVEMENT_PLAN.md` for quick reference during execution.

## Current State
- ✅ Tests: 451/451 passing (100%)
- ⚠️ Coverage: 86% (target: 95%+)
- ⚠️ Docs: PDF generator complete, root needs update
- ✅ Functionality: All working (PDF, maps, charts)

## Phase Summary

| Phase | Focus | Hours | Priority | Status |
|-------|-------|-------|----------|--------|
| **A** | CLI Test Coverage | 4-6 | High | 🟡 Ready |
| **B** | Core Test Coverage | 8-10 | High | 🟡 Ready |
| **C** | Root Documentation | 4-6 | Critical | 🟡 Ready |
| **D** | Go Integration Docs | 3-4 | Medium | 🟡 Ready |
| **E** | Developer Experience | 6-8 | High | 🟡 Ready |
| **F** | CI/CD Documentation | 2-3 | Low | 🟡 Ready |

**Total Estimated Effort**: 27-37 hours

## Recommended Execution Order

```
1. Phase A (CLI coverage)    ← Quick wins
2. Phase C (Root docs)       ← Unblocks onboarding  
3. Phase B (Core coverage)   ← Parallel with E
4. Phase E (DevEx)           ← Parallel with B
5. Phase D (Go integration)  ← Prep for Phase 10
6. Phase F (CI/CD)           ← Deferred if needed
```

## Coverage Targets by Phase

### Phase A: CLI Tools (0% → 100%)
- `create_config.py`: 21 lines uncovered
- `demo.py`: 123 lines uncovered
- **Impact**: 86% → ~92% overall coverage

### Phase B: Core Modules
- `chart_builder.py`: 83% → 95% (67 lines)
- `table_builders.py`: 87% → 95% (22 lines)
- `dependency_checker.py`: 92% → 95% (10 lines)
- `map_utils.py`: 93% → 95% (11 lines)
- **Impact**: 92% → 95%+ overall coverage

## Quick Commands

### Run All Tests
```bash
cd /Users/david/code/velocity.report/tools/pdf-generator
make pdf-test
```

### Run Coverage Analysis
```bash
cd /Users/david/code/velocity.report/tools/pdf-generator
PYTHONPATH=. .venv/bin/pytest --cov=pdf_generator --cov-report=term-missing pdf_generator/tests/
```

### Generate HTML Coverage Report
```bash
cd /Users/david/code/velocity.report/tools/pdf-generator
PYTHONPATH=. .venv/bin/pytest --cov=pdf_generator --cov-report=html pdf_generator/tests/
# Open htmlcov/index.html in browser
```

### Run Specific Test File
```bash
cd /Users/david/code/velocity.report/tools/pdf-generator
PYTHONPATH=. .venv/bin/pytest pdf_generator/tests/test_<module>.py -v
```

## Phase A: CLI Test Coverage Checklist

### create_config.py Tests
- [ ] Test minimal template generation
- [ ] Test full template generation
- [ ] Test file writing to specified path
- [ ] Test CLI argument parsing
- [ ] Test error handling (permissions, invalid paths)
- [ ] Test default filename behavior

**New file**: `pdf_generator/tests/test_create_config.py`

### demo.py Tests
- [ ] Test interactive prompt flow (mock input)
- [ ] Test configuration building from prompts
- [ ] Test validation of user inputs
- [ ] Test example generation
- [ ] Test error recovery
- [ ] Test help text display

**New file**: `pdf_generator/tests/test_demo.py`

**Estimated coverage gain**: 86% → 92%

## Phase B: Core Coverage Improvement Checklist

### chart_builder.py (67 uncovered lines)
Focus on lines: 80, 250-258, 273-274, 369, 385-387, 449-454, etc.

- [ ] Error handling in chart generation
- [ ] Histogram bucket edge cases
- [ ] Font fallback scenarios
- [ ] Empty data handling
- [ ] Invalid config handling
- [ ] Chart styling variations

**Update file**: `pdf_generator/tests/test_chart_builder.py`

### table_builders.py (22 uncovered lines)
Focus on lines: 63, 180, 284, 327, 389, 439, 448-449, 473-474, 517-544

- [ ] Edge cases in range computation
- [ ] Empty histogram data
- [ ] Maximum bucket handling
- [ ] Table formatting variations
- [ ] Error scenarios

**Update file**: `pdf_generator/tests/test_table_builders.py`

**Estimated coverage gain**: 92% → 95%+

## Phase C: Root Documentation Checklist

### Files to Create/Update

1. [ ] Update `/README.md`:
   - [ ] Add project overview section
   - [ ] Add architecture summary (Go + Python + Web)
   - [ ] Add quick start for each component
   - [ ] Add project structure section
   - [ ] Add links to component READMEs

2. [ ] Create `/ARCHITECTURE.md`:
   - [ ] System diagram
   - [ ] Component relationships
   - [ ] Data flow (sensor → Go → DB → API → Python/Web)

3. [ ] Create `/CONTRIBUTING.md`:
   - [ ] Go development workflow
   - [ ] Python development workflow
   - [ ] Testing requirements
   - [ ] PR process

4. [ ] Create/Update `/docs/README.md`:
   - [ ] Documentation index
   - [ ] Links to all component docs
   - [ ] Navigation guide

## Phase D: Go Integration Checklist

### Files to Create

1. [ ] `/tools/pdf-generator/GO_INTEGRATION.md`:
   - [ ] How Go calls Python PDF generator
   - [ ] Path resolution patterns
   - [ ] Error handling
   - [ ] Example Go code snippets

2. [ ] `/docs/PHASE_10_MIGRATION_GUIDE.md`:
   - [ ] Files to update in Go codebase
   - [ ] Path changes required
   - [ ] Testing strategy
   - [ ] Rollback plan

## Phase E: Developer Experience Checklist

1. [ ] Create `/scripts/dev-setup.sh`:
   - [ ] Check prerequisites (Go, Python versions)
   - [ ] Setup Go dependencies
   - [ ] Setup Python venv
   - [ ] Initialize database
   - [ ] Run health checks

2. [ ] Create `/tools/pdf-generator/TROUBLESHOOTING.md`:
   - [ ] Font rendering issues
   - [ ] Map generation issues
   - [ ] LaTeX compilation errors
   - [ ] Common test failures

3. [ ] Improve error messages:
   - [ ] Add context to exceptions
   - [ ] Add hints for config errors
   - [ ] Translate LaTeX errors

4. [ ] Add `.pre-commit-config.yaml`:
   - [ ] Go formatting check
   - [ ] Python formatting (black/ruff)
   - [ ] Test execution
   - [ ] Doc link validation

## Phase F: CI/CD Documentation Checklist

1. [ ] Create `/docs/CI_CD.md`:
   - [ ] Current CI setup
   - [ ] Proposed GitHub Actions
   - [ ] Testing strategy
   - [ ] Deployment automation

2. [ ] Create `/docs/RELEASE_PROCESS.md`:
   - [ ] Versioning strategy
   - [ ] Changelog management
   - [ ] Release artifacts
   - [ ] Deployment checklist

3. [ ] Create example workflow:
   - [ ] `.github/workflows/test.yml.example`
   - [ ] Test Go + Python
   - [ ] Build artifacts

## Success Criteria

Each phase complete when:
- [ ] All tasks checked off
- [ ] Tests still pass (451+)
- [ ] Coverage target met
- [ ] Documentation updated
- [ ] No regressions

## Overall Success Metrics

- [ ] Coverage: 86% → 95%+
- [ ] All components documented
- [ ] Developer onboarding: <15 minutes
- [ ] No broken functionality

## Tips for Execution

1. **Start Small**: Begin with Phase A (quick wins)
2. **Run Tests Often**: After every change
3. **Check Coverage**: Use HTML report to identify gaps
4. **Document As You Go**: Update docs while code is fresh
5. **Ask Questions**: Clarify before implementing
6. **Take Breaks**: Don't rush, quality over speed

## Files to Reference

- **Main Plan**: `/IMPROVEMENT_PLAN.md` (detailed)
- **Current State**: `/tools/pdf-generator/CURRENT_STATE.md`
- **Coverage Report**: Run `make pdf-test` with coverage flag

## Need Help?

1. Check `/tools/pdf-generator/README.md` for Python docs
2. Check existing tests for patterns
3. Ask questions before making changes
4. Review improvement plan for context

---

**Start Here**: Phase A (CLI Test Coverage)  
**Quick Win**: Get create_config.py and demo.py to 100% coverage  
**Next**: Phase C (Root Documentation) for immediate developer value
