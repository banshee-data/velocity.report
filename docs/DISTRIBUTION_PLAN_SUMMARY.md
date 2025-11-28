# Distribution and Packaging Plan - Executive Summary

**Full Document:** [DISTRIBUTION_AND_PACKAGING_PLAN.md](./DISTRIBUTION_AND_PACKAGING_PLAN.md)  
**Date:** 2025-11-20  
**Status:** Proposed for Review

## The Challenge

velocity.report currently has multiple components scattered across the repository:
- Main server binary (`cmd/radar`)
- Database migrations (subcommand in binary)
- Sweep tool (`cmd/sweep`)
- PDF generator (Python)
- Various utility tools (transit-backfill, ring elevations, grid-heatmap)
- Web frontend (embedded)

**Current problems:**
- No unified distribution strategy
- Python tools require manual PYTHONPATH setup
- Unclear which tools are for end-users vs developers
- No GitHub releases or versioning
- Complex installation process

## Recommended Solution: Hybrid Distribution Model

### Core Design

**Primary Binary: `velocity-report`** (with subcommands)
```bash
velocity-report serve          # Start radar/LIDAR server (default)
velocity-report migrate        # Database migrations (existing)
velocity-report pdf            # Generate PDF report (wrapper for Python)
velocity-report backfill       # Transit backfill
velocity-report version        # Show version info
```

**Secondary Binaries** (for power users)
- `velocity-report-sweep` - LIDAR parameter sweep tool
- `velocity-report-backfill-rings` - Ring elevation backfill utility

**Python Tools** (integrated but separate)
- Installed in `/usr/local/share/velocity-report/python/`
- Invoked via Go wrapper (`velocity-report pdf`)
- Can also be used standalone for development

### Why This Approach?

✅ **For End Users:**
- Single binary handles 90% of use cases
- Simple installation: `curl -sSL install.sh | sudo bash`
- Familiar CLI pattern (like `git`, `docker`)
- PDF generation just works: `velocity-report pdf config.json`

✅ **For Power Users:**
- Advanced tools available as separate binaries
- Full control over Python environment if needed
- All tools discoverable via naming convention

✅ **For Developers:**
- Development workflow unchanged
- All Makefile targets still work
- Clear separation between core and utilities

✅ **Technical Benefits:**
- Follows Go/Python best practices
- No complex embedding or cross-compilation issues
- Each component independently updatable
- GitHub Actions can automate releases

## Four Distribution Approaches Evaluated

| Approach | Pros | Cons | Verdict |
|----------|------|------|---------|
| **A: Monolithic Binary** (embed Python) | Single file | ❌ Complex, brittle, large | ❌ Not Recommended |
| **B: Multi-Binary Suite** (all separate) | Standard Unix convention | Multiple files | ✅ Good Option |
| **C: Subcommand Architecture** (single entry) | Best discoverability | Larger binary | ✅ Strong Candidate |
| **D: Hybrid Model** (core + optional) | Balanced, flexible | Slight complexity | ✅ **RECOMMENDED** |

See full document for detailed analysis of each approach.

## Implementation Phases

### Phase 1: Restructure Go Binaries (1-2 weeks)
- Rename `cmd/radar/` → `cmd/velocity-report/`
- Add subcommand dispatcher
- Integrate backfill into main binary
- Update Makefile and systemd service

### Phase 2: Python Integration (1 week)
- Create Go wrapper for PDF generation
- Make Python tools installable
- Add Python path discovery logic
- Update setup scripts

### Phase 3: GitHub Releases (3-5 days)
- Create release workflow (`.github/workflows/release.yml`)
- Build binaries for all platforms (ARM64, x64, macOS)
- Package Python tools as tarball
- Automated release on git tag

### Phase 4: Installation Script (3-5 days)
- One-line install: `curl -sSL install.sh | sudo bash`
- Detect architecture and OS
- Install binaries + Python tools
- Set up systemd service

### Phase 5: Testing & Rollout (1 week)
- Test on Raspberry Pi 4
- Alpha release (v0.1.0-alpha)
- Beta release (v0.1.0-beta)
- Stable release (v1.0.0)

**Total Timeline: 4-6 weeks to v1.0.0**

## Migration Path for Existing Users

### Backward Compatible
- ✅ Old binary still works (no args = start server)
- ✅ New binary accepts same flags
- ✅ Database unchanged
- ✅ Web UI unchanged

### Changes
- Service file: Change `ExecStart=/usr/local/bin/velocity-report` to `ExecStart=/usr/local/bin/velocity-report serve`
- Python tools: Can now use `velocity-report pdf` instead of complex PYTHONPATH setup
- Sweep tool: Renamed from `app-sweep` to `velocity-report-sweep`

### Migration Steps
1. Stop service: `sudo systemctl stop velocity-report`
2. Backup database
3. Download new binary from GitHub release
4. Test new binary
5. Update systemd service file
6. Install new binary
7. Restart service: `sudo systemctl start velocity-report`

Full migration guide in main document.

## File Structure Changes

### Before
```
cmd/radar/                      # Main server
cmd/sweep/                      # Sweep tool
cmd/transit-backfill/          # Backfill utility
tools/pdf-generator/           # Python PDF
Binary: velocity-report-linux-arm64, app-sweep
```

### After
```
cmd/velocity-report/           # Main binary (was cmd/radar)
  ├── main.go                  # Subcommand dispatcher
  ├── serve.go                 # Server logic
  ├── pdf.go                   # PDF wrapper
  └── backfill.go              # Backfill (moved)
cmd/velocity-report-sweep/     # Sweep (renamed)
cmd/velocity-report-backfill-rings/  # Utility (renamed)
tools/pdf-generator/           # Python PDF (unchanged)
Binary: velocity-report-*, velocity-report-sweep-*
```

### Installed System
```
/usr/local/bin/
  ├── velocity-report
  ├── velocity-report-sweep (optional)
  └── velocity-report-backfill-rings (optional)

/usr/local/share/velocity-report/
  └── python/
      ├── .venv/
      ├── pdf_generator/
      └── requirements.txt

/var/lib/velocity-report/
  └── sensor_data.db

/etc/systemd/system/
  └── velocity-report.service
```

## Command Comparison

### Current (Before)
```bash
velocity-report --db-path /path/to/db          # Start server
velocity-report migrate up --db-path /path     # Migrate
cd tools/pdf-generator && PYTHONPATH=. python -m pdf_generator.cli.main config.json
./app-sweep --mode multi
```

### Proposed (After)
```bash
velocity-report serve --db-path /path/to/db    # Start server (or just: velocity-report)
velocity-report migrate up --db-path /path     # Migrate (unchanged)
velocity-report pdf config.json                # Generate PDF (simplified!)
velocity-report-sweep --mode multi             # Sweep (renamed)
```

## Future Enhancements

Once this foundation is in place, we can add:

1. **Docker Distribution** - Pre-built images
2. **Raspberry Pi Image** - Flash and run
3. **Package Managers** - APT/DEB, Homebrew
4. **Web-Based Configuration** - No CLI required
5. **Plugin System** - Third-party extensions

## Key Benefits

✅ **Simplicity** - Single binary for most users  
✅ **Discoverability** - All features via subcommands  
✅ **Automation** - GitHub releases with CI/CD  
✅ **Professional** - Industry-standard patterns  
✅ **Compatible** - Existing deployments keep working  
✅ **Maintainable** - Clear separation of concerns  

## Next Steps

1. **Review this plan** - Gather feedback from team
2. **Approve approach** - Confirm hybrid model is best fit
3. **Start Phase 1** - Restructure Go binaries
4. **Iterate** - Adjust based on implementation learnings
5. **Release alpha** - Get community feedback
6. **Ship v1.0.0** - Roll out to production

## Questions?

See the full document [DISTRIBUTION_AND_PACKAGING_PLAN.md](./DISTRIBUTION_AND_PACKAGING_PLAN.md) for:
- Detailed analysis of all approaches
- Complete implementation steps
- Migration guide with commands
- Testing checklist
- Release process
- Breaking changes summary

## Document Structure

The full 1,589-line plan includes:

1. **Current State Analysis** - What we have today
2. **User Personas** - Who uses what and why
3. **Approach Tradeoffs** - Detailed comparison of 4 options
4. **Recommended Architecture** - Complete design spec
5. **Implementation Plan** - 5 phases with timelines
6. **Migration Guide** - Step-by-step upgrade path
7. **Testing & Validation** - Quality assurance strategy
8. **Future Enhancements** - Long-term roadmap
9. **Appendices** - Reference materials

---

**Document Prepared By:** Agent Ictinus  
**Role:** Product-Conscious Software Architect  
**Focus:** Feature ideation, capability mapping, evolution paths  
**Output:** Design documents, specifications, architectural proposals
