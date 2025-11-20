# Distribution Plan - Quick Reference Card

**Full Plan:** [DISTRIBUTION_AND_PACKAGING_PLAN.md](./DISTRIBUTION_AND_PACKAGING_PLAN.md) (1,589 lines)  
**Summary:** [DISTRIBUTION_PLAN_SUMMARY.md](./DISTRIBUTION_PLAN_SUMMARY.md) (258 lines)

## 30-Second Pitch

**Problem:** Multiple scattered tools, no release process, complex Python setup  
**Solution:** Single `velocity-report` binary with subcommands + optional power-user tools  
**Result:** Professional distribution with one-line install and GitHub releases

## Recommended Architecture (Hybrid Model)

```
velocity-report                        # Main binary (all users)
  ├── serve      (default)            # Start server
  ├── migrate    (existing)           # DB migrations
  ├── pdf        (new)                # Generate PDF
  ├── backfill   (moved)              # Transit backfill
  └── version    (new)                # Version info

velocity-report-sweep                  # Power user tool
velocity-report-backfill-rings         # Developer tool
```

## Key Changes

| What | Before | After |
|------|--------|-------|
| **Main binary** | `cmd/radar/` | `cmd/velocity-report/` |
| **Start server** | `velocity-report` | `velocity-report serve` (or just `velocity-report`) |
| **PDF generation** | `PYTHONPATH=... python -m ...` | `velocity-report pdf config.json` |
| **Sweep tool** | `./app-sweep` | `velocity-report-sweep` |
| **Installation** | Manual build + scp + script | `curl install.sh \| sudo bash` |
| **Releases** | None | GitHub Releases with CI/CD |

## Timeline

- **Phase 1:** Go restructure (1-2 weeks)
- **Phase 2:** Python integration (1 week)
- **Phase 3:** GitHub Actions (3-5 days)
- **Phase 4:** Install script (3-5 days)
- **Phase 5:** Testing (1 week)
- **Total: 4-6 weeks to v1.0.0**

## User Impact

### End Users ✅
- **Before:** Clone repo, build, configure PYTHONPATH, run scripts
- **After:** `curl install.sh | bash` → done

### Power Users ✅
- **Before:** Find and build various tools manually
- **After:** All tools named `velocity-report-*` and in PATH

### Developers ✅
- **Before:** Complex Makefile, unclear tool organization
- **After:** Same Makefile, clearer structure, all tests pass

## Migration Checklist

```bash
# 1. Stop service
sudo systemctl stop velocity-report

# 2. Backup
sudo cp /var/lib/velocity-report/sensor_data.db{,.backup}

# 3. Download new binary
curl -LO https://github.com/.../releases/download/v1.0.0/velocity-report-linux-arm64

# 4. Test
./velocity-report-linux-arm64 version
./velocity-report-linux-arm64 migrate status

# 5. Update service file
# Change: ExecStart=/usr/local/bin/velocity-report ...
# To:     ExecStart=/usr/local/bin/velocity-report serve ...

# 6. Install
sudo mv velocity-report-linux-arm64 /usr/local/bin/velocity-report
sudo systemctl daemon-reload
sudo systemctl start velocity-report

# 7. Verify
velocity-report version
curl http://localhost:8080
```

## Breaking Changes

### For End Users: ✅ NONE
- Old binary still works (no args = serve)
- All flags preserved

### For Developers: ⚠️ MINOR
- `cmd/radar/` moved to `cmd/velocity-report/`
- Import paths unchanged

### For Advanced Users: ✨ IMPROVED
- Better tool naming and discoverability

## Commands Cheatsheet

### Current
```bash
velocity-report                                    # Start server
velocity-report migrate up                         # Migrate
cd tools/pdf-generator && PYTHONPATH=... python... # PDF (ugly!)
./app-sweep --mode multi                          # Sweep
```

### Proposed
```bash
velocity-report                                    # Start server (default)
velocity-report serve                              # Start server (explicit)
velocity-report migrate up                         # Migrate (same)
velocity-report pdf config.json                    # PDF (clean!)
velocity-report-sweep --mode multi                 # Sweep (renamed)
velocity-report version                            # Version (new)
```

## File Structure Summary

### Current
```
cmd/radar/           → Main server
cmd/sweep/           → Sweep tool
cmd/transit-backfill/ → Backfill
tools/pdf-generator/ → Python PDF
```

### Proposed
```
cmd/velocity-report/ → Main (was radar) with subcommands
cmd/velocity-report-sweep/ → Sweep (renamed)
cmd/velocity-report-backfill-rings/ → Utility (renamed)
tools/pdf-generator/ → Python (unchanged, but integrated)
```

## Decision Matrix

| Criterion | Score | Notes |
|-----------|-------|-------|
| **Ease of Use** | ⭐⭐⭐⭐⭐ | One-line install |
| **Discoverability** | ⭐⭐⭐⭐⭐ | All via subcommands |
| **Maintainability** | ⭐⭐⭐⭐⭐ | Clear separation |
| **Best Practices** | ⭐⭐⭐⭐⭐ | Go/Python standards |
| **Complexity** | ⭐⭐⭐⭐☆ | Slightly more build steps |
| **Migration Effort** | ⭐⭐⭐⭐☆ | Mostly backward compatible |

## Approval Checklist

- [ ] Review full plan (DISTRIBUTION_AND_PACKAGING_PLAN.md)
- [ ] Approve hybrid architecture
- [ ] Confirm timeline acceptable
- [ ] Assign developer for Phase 1
- [ ] Set alpha release date
- [ ] Announce to community

## Questions to Answer

1. **Go binary naming:** `velocity-report` vs `vr` vs other?
   - **Recommendation:** `velocity-report` (explicit, searchable)

2. **Python installation:** System Python + pip vs venv?
   - **Recommendation:** venv in `/usr/local/share/` (isolated)

3. **Release frequency:** Major.Minor.Patch vs date-based?
   - **Recommendation:** SemVer (1.0.0, 1.1.0, 2.0.0)

4. **Backward compatibility window:** How long support old patterns?
   - **Recommendation:** 2 major versions (1.x supports old, 3.x removes)

## Success Metrics

**v1.0.0 Launch:**
- [ ] One-line install works on fresh Pi
- [ ] All core subcommands functional
- [ ] GitHub Release with binaries
- [ ] Documentation complete
- [ ] <5 GitHub issues from migration

**6 Months Post-Launch:**
- [ ] 90% of users on v1.x
- [ ] Positive community feedback
- [ ] <10 distribution-related bugs
- [ ] Release process takes <1 hour

## Resources

- **Full Plan:** [DISTRIBUTION_AND_PACKAGING_PLAN.md](./DISTRIBUTION_AND_PACKAGING_PLAN.md)
- **Summary:** [DISTRIBUTION_PLAN_SUMMARY.md](./DISTRIBUTION_PLAN_SUMMARY.md)
- **Architecture:** [ARCHITECTURE.md](../ARCHITECTURE.md)
- **Current Makefile:** [Makefile](../Makefile)

## Contact

- **Plan Author:** Agent Ictinus
- **Role:** Product-Conscious Software Architect
- **Focus:** Feature ideation, capability mapping, evolution paths

---

**Status:** ✅ Plan Complete - Ready for Review  
**Next Step:** Team review and approval to proceed with Phase 1
