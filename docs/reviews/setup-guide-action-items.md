# Setup Guide: Action Items for DIY Magazine Publication

**Date**: 2025-11-06  
**Full Review**: See `setup-guide-diy-magazine-review.md`

---

## Quick Status

**Overall Readiness**: 85% ready for publication  
**Estimated Work**: 4-8 hours to address critical issues  
**Recommendation**: ✅ Approve with revisions - Fix critical issues first

---

## Critical Issues (Must Fix)

### 1. 🔴 Python Virtual Environment Path - INCORRECT CLAIM

**Location**: Line 291-292  
**Current Claim**: "The Python environment is created at the repository root (`.venv/`)"  
**Reality**: Makefile creates `tools/pdf-generator/.venv` (OLD location)

**Fix Options**:
- **Option A** (Preferred): Update Makefile to create `.venv/` at repo root (complete migration)
- **Option B**: Update setup guide to say `tools/pdf-generator/.venv` (document current reality)

**Code to fix** (if Option A):
```makefile
# In Makefile, change install-python target:
install-python:
	python3 -m venv .venv  # Create at repo root instead of tools/pdf-generator/
	.venv/bin/pip install --upgrade pip
	.venv/bin/pip install -r tools/pdf-generator/requirements.txt
```

---

### 2. 🔴 PDF Generation Web Workflow - NEEDS VERIFICATION

**Location**: Lines 274-278  
**Current Claim**: "Configure a **Site** tab → Enable **PDF Report Generation** → Click **Generate Report**"

**Action Required**:
1. Start the web server and verify this exact workflow exists
2. If it exists, add screenshots showing each step
3. If it doesn't exist, update documentation to match actual UI

**Testing steps**:
```bash
make build-local
./app-local -dev
# Open http://localhost:8080
# Follow steps described in guide
# Screenshot each screen
```

---

### 3. 🟡 Cost Placeholder

**Location**: Line 13  
**Current**: `**Cost**: ~$XXX-YYY`  
**Fix**: `**Cost**: ~$200-300`

Simple one-line fix.

---

### 4. 🟡 PDF Generator Command Inconsistency

**Setup Guide** says:
```bash
make pdf-config          # Creates config.json
make pdf-report CONFIG=config.json
```

**PDF README** says:
```bash
cd tools/pdf-generator
make pdf-config         # Creates config.example.json
make pdf-report CONFIG=config.example.json
```

**Fix**: Standardize both documents to same workflow (recommend: repo root, config.example.json)

---

## Medium Priority Issues (Should Fix)

### 5. Add Troubleshooting Section

Add new section after Step 7 with common issues:

```markdown
## Troubleshooting

### No data from sensor
- Check baud rate: 19200
- Verify port: `ls /dev/tty*` before/after plugging in
- Check power: 12V stable
- View USB logs: `dmesg | tail`

### Service won't start
- Check binary: `ls -l /usr/local/bin/velocity-report`
- Check permissions: `ls -l /var/lib/velocity-report/`
- View logs: `sudo journalctl -u velocity-report -n 50`

### Web dashboard not accessible
- Check service: `sudo systemctl status velocity-report`
- Check port: `sudo netstat -tlnp | grep 8080`
- Try locally: `curl http://localhost:8080/`

### PDF generation fails
- Check Python: `which python` (should be in venv)
- Check LaTeX: `xelatex --version`
- Check logs in: `tools/pdf-generator/output/`
```

---

### 6. Add Time Estimates

Add duration to each step header:

```markdown
### **Step 1: Mount the Radar Sensor** (15-30 minutes)
### **Step 2: Connect the Sensor** (10-15 minutes)
### **Step 3: Flash Firmware** (20-40 minutes)
### **Step 4: Verify Data Stream** (10 minutes)
### **Step 5: Install Software** (30-60 minutes)
### **Step 6: Access Dashboard** (5 minutes)
### **Step 7: Generate Reports** (varies - need data collection period)
```

---

### 7. Add Platform Compatibility Note

After line 136, add:

```markdown
**Platform notes**: 
- Linux: Use `stty -F /dev/ttyUSB0` as shown
- macOS: Use `stty -f /dev/ttyUSB0` (lowercase `-f`)
- If `screen` not installed: `sudo apt install screen` or use `minicom`
```

---

## Nice to Have (Optional)

### 8. Add Visual Aids
- System architecture flowchart
- Photos of radar mounting angles
- Screenshot of web dashboard

### 9. Add Security Notes

```markdown
### Security Considerations

**Network**: Dashboard runs on port 8080 without authentication
- Home networks: Generally safe (router blocks external access)
- Public deployments: Use reverse proxy with HTTPS + password

**Pi hardening**: Change default password, enable SSH keys, disable password login
```

---

### 10. Add Storage Planning

```markdown
### Storage Planning

**Database growth**: ~1 MB per 10,000 vehicle detections

**Typical scenarios**:
- Quiet street (50 cars/day): ~5 MB/month
- Moderate (500 cars/day): ~50 MB/month
- Busy (2000 cars/day): ~200 MB/month

**Data collection time**:
- Minimum: 1 week for patterns
- Optimal: 1 month for analysis
- Ideal: 3+ months for trends
```

---

## Testing Checklist

Before publication, test on **fresh Raspberry Pi**:

- [ ] Follow Steps 1-4 (hardware setup)
- [ ] Follow Step 5 (software installation) - time each substep
- [ ] Follow Step 6 (web dashboard) - screenshot UI
- [ ] Follow Step 7 (PDF generation) - verify EXACT workflow works
- [ ] Test troubleshooting steps actually solve problems
- [ ] Verify all commands execute without errors
- [ ] Check all links are valid

**Estimated testing time**: 3-4 hours + data collection period

---

## Files to Update

1. **docs/src/guides/setup.md** - Main setup guide
   - Fix Python venv path (line 291)
   - Fill cost placeholder (line 13)
   - Verify/update PDF workflow (lines 274-278)
   - Add troubleshooting section
   - Add time estimates
   - Add platform notes

2. **Makefile** - Build system
   - Update `install-python` to create `.venv/` at repo root
   - OR document that it creates `tools/pdf-generator/.venv`

3. **tools/pdf-generator/README.md** - PDF docs
   - Align with setup guide workflow
   - Standardize config filename

4. **README.md** - Optional
   - Align PDF quickstart with setup guide

---

## Priority Order

1. **Fix Python venv path** (30 min) - Highest impact
2. **Verify PDF web workflow** (60 min) - Critical for users
3. **Fill cost placeholder** (5 min) - Quick win
4. **Add troubleshooting** (60 min) - High value
5. **Standardize PDF commands** (30 min) - Reduces confusion
6. **Add time estimates** (15 min) - Nice UX improvement
7. **Add platform notes** (15 min) - Prevents common issues
8. **Test on fresh Pi** (4 hours) - Validation
9. **Optional enhancements** (varies) - As time permits

**Total critical path**: ~6-7 hours

---

## Success Criteria

Guide is ready when:

- ✅ All commands execute successfully on fresh Raspberry Pi
- ✅ Python venv path matches documentation and reality
- ✅ PDF generation workflow verified and documented
- ✅ Cost estimate filled in
- ✅ Troubleshooting section helps solve common issues
- ✅ No placeholder values remain (XXX, YYY, TBD)
- ✅ Cross-references between docs are consistent

---

## Contact

Questions about this review? See full analysis in `setup-guide-diy-magazine-review.md`
