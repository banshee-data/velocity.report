# Setup Guide: Thompson's Action Items

**Date**: 2025-11-07  
**Full Review**: See `setup-guide-thompson-review.md`  
**Complements**: Ictinus review (technical/functional focus)

---

## Quick Status

**Publication Ready**: ❌ No - needs copy editing pass  
**Estimated Work**: 16-20 hours for complete treatment  
**Quick Wins**: 2-3 hours for high-impact fixes  
**Priority**: HIGH - flagship user-facing document

---

## Critical Issues (Must Fix)

### 1. 🔴 Brand Voice Violations

**Location**: Throughout document  
**Issues**:
- Line 46: "Here's a weekend project" (too casual)
- Line 339: "Nice work!" (condescending tone)
- Inconsistent formality (fluctuates casual ↔ professional)

**Fix**:
```markdown
❌ "Here's a weekend project that fixes that."
✅ "This weekend project addresses that need."

❌ "Nice work! You've built a working traffic radar"
✅ "You've successfully built a working traffic radar"
```

**Time**: 1 hour to audit and fix all instances

---

### 2. 🔴 Structure: Deployment Split Creates Confusion

**Location**: Lines 17-40  
**Problem**: Dual paths (DIY vs Infrastructure) introduced before readers understand what they're building

**Impact**: Decision paralysis, cognitive overload, forces reading both paths

**Fix**: Default to DIY path, defer infrastructure to appendix

```markdown
## DIY Deployment (~$150-200)

[Full build instructions]

---

## Advanced: Weatherproof Infrastructure (~$350-450)

After validating your approach with DIY deployment, consider permanent 
installation. See [Infrastructure Deployment Guide](infrastructure-setup.md).
```

**Alternative**: Split into two separate documents entirely

**Time**: 3-4 hours to restructure

---

### 3. 🔴 Missing Visual Documentation

**Location**: Entire document (0 images in 748 lines)  
**Impact**: 
- Can't visualize final result
- Ambiguous instructions ("20-45° angle" - what does this look like?)
- Reduces confidence

**Required screenshots**:
1. Completed assembly (both deployments)
2. Radar mounting angle diagram
3. Web dashboard overview
4. Sample PDF report
5. RS232 wiring diagram

**Time**: 4-6 hours to create/capture + integrate

---

### 4. 🔴 Parts List: Information Overload

**Location**: Lines 92-249 (158 lines!)  
**Problem**: Deep dive into product codes before understanding purpose

**Fix**: Recommendations first, details later

```markdown
## Parts List (Quick Start)

| Part | Model | Price |
|------|-------|-------|
| Radar | OPS243-A-CW-RP | ~$100-130 |
| Pi | Zero 2 W | ~$15-20 |
| Power | 5V 2.5A adapter | ~$10-15 |
| **Total** | | **~$150** |

[Start building]

---

## Appendix: Sensor Selection Guide

[Detailed product codes, comparison tables]
```

**Time**: 2-3 hours to reorganize

---

### 5. 🔴 Missing Prerequisites Section

**Location**: Nowhere (jumps straight to build)  
**Problem**: Readers don't know if they have necessary skills/tools

**Add before Step 1**:

```markdown
## Before You Begin

**Skills required**:
- Basic Linux command line (SSH, file editing)
- Basic hardware assembly
- Patience for troubleshooting

**Tools needed**:
- Computer for SD card flashing
- Screwdrivers
- Optional: Multimeter

**No soldering** • **No coding** • **No prior radar experience**

**Time**: 2-4 hours build + data collection period  
**Cost**: ~$150-200 (DIY) or ~$350-450 (weatherproof)
```

**Time**: 30 minutes to write

---

## High Priority (Should Fix)

### 6. 🟡 Passive Voice & Wordiness

**Current**: ~6,500 words with passive constructions  
**Target**: ~5,000 words (25% reduction), active voice

**Examples**:

```markdown
❌ "OmniPreSense radar sensors are available in multiple configurations"
✅ "OmniPreSense offers radar sensors in multiple configurations"

❌ Current intro (109 words)
✅ Tightened (78 words - 28% shorter)
```

**Time**: 3 hours for comprehensive edit

---

### 7. 🟡 Missing Navigation Aids

**Add to document**:
- Table of contents at top
- Section cross-references
- "Back to top" links
- Related docs footer

```markdown
**In this guide**: [Hardware](#hardware) • [Software](#software) • 
[Reports](#reports) • [Troubleshooting](#troubleshooting)
```

**Time**: 1 hour to implement

---

### 8. 🟡 Poor Scannability (Text Walls)

**Problem**: Dense paragraphs, minimal visual hierarchy

**Fixes**:
- Break long paragraphs (max 4-5 lines)
- Add callout boxes for important info
- Use bold/italic strategically
- Add more subheadings

**Before** (24 lines dense text):
```markdown
1. **Prepare weatherproof enclosure**:
   - Drill mounting holes...
   - Install cable glands...
   [20 more lines]
```

**After** (grouped and scannable):
```markdown
1. **Prepare enclosure**:

   **Mounting prep**:
   - Drill holes for clamps
   - Install cable glands
   
   **Sensor position**:
   - Clear view through front
   - Use acrylic window if needed
```

**Time**: 2 hours to refactor

---

### 9. 🟡 Missing Success Criteria

**Problem**: No "how do I know this worked?" markers

**Add to each step**:

```markdown
### Step 4: Verify Data Stream

```bash
cat /dev/ttyUSB0
```

**Success looks like**:
```json
{ "magnitude": 1.2, "speed": 3.4 }
```

**Troubleshooting**:
- Garbled text → Check baud rate (19200)
- Nothing → Verify power and connection
- Permission denied → Add user to dialout group
```

**Time**: 1.5 hours for all steps

---

### 10. 🟡 Terminology Inconsistencies

**Issues found**:
- "OPS243A" vs "OPS243-A-CW-RP" vs "OPS243"
- "USB interface" vs "RP interface" vs "USB (RP)"
- Inconsistent code formatting for paths

**Standardize**:
```markdown
First reference: "OmniPreSense OPS243-A-CW-RP radar sensor"
Subsequent: "OPS243 sensor" or "radar sensor"
Interfaces: "USB interface (designated RP in product codes)"
Paths: Always use code format: `/var/lib/velocity-report/`
```

**Time**: 1 hour audit + fixes

---

## Medium Priority (Nice to Have)

### 11. 🟢 Add Audience Segmentation

```markdown
## Who This Guide Is For

**Community advocates**: Professional data for traffic calming proposals  
**Parents**: Prove speeding near schools with evidence  
**Data enthusiasts**: Build useful civic tech  
**Local officials**: Validate commercial traffic studies

**Not sure?** 2-4 hours, ~$150. Worth it if you care about street safety.
```

**Time**: 15 minutes

---

### 12. 🟢 Improve Legal Guidance Position

**Current**: Lines 303-329 (buried at end)  
**Better**: Move to "Before You Begin" section

**Restructure**:
- What's collected (speed, time) vs not (plates, photos)
- Generally allowed uses
- May need permission (utility poles, school zones)
- Clear disclaimer

**Time**: 30 minutes to relocate + revise

---

### 13. 🟢 Add Advocacy Guide

**After data collection, what next?**

```markdown
## Using Your Data for Advocacy

### Presenting to City Council

**Do**: Print PDF reports, compare to speed limits, propose solutions  
**Don't**: Share raw data, attack drivers, make emotional appeals

### Example Talking Points

❌ "Cars go way too fast!"
✅ "85% of drivers exceed the 25 mph limit, with p85 at 38 mph"
```

**Time**: 1 hour to write

---

### 14. 🟢 Add Context to Code Blocks

**Current**: Commands without explanation  
**Better**: Inline comments

```bash
# Clone the repository
git clone https://github.com/banshee-data/velocity.report.git

# Build the Go server binary
make build-radar-linux

# Install as system service (creates user, database, systemd)
sudo ./scripts/setup-radar-host.sh
```

**Time**: 30 minutes for all blocks

---

### 15. 🟢 Add Uninstall Instructions

```markdown
## Uninstalling

To completely remove velocity.report:

```bash
sudo systemctl stop velocity-report
sudo systemctl disable velocity-report
sudo rm /usr/local/bin/velocity-report
sudo rm /etc/systemd/system/velocity-report.service
sudo rm -rf /var/lib/velocity-report/
```

**Warning**: This deletes all data. Export PDFs first.
```

**Time**: 15 minutes

---

### 16. 🟢 Reduce DRY Violations

**Issue**: Repeats content from README.md

**Fix**: Link instead of repeat

```markdown
## What is velocity.report?

**New to this project?** Read the [project overview](../../README.md).

**Ready to build?** Continue below.
```

**Time**: 30 minutes to identify and fix

---

### 17. 🟢 Strengthen Competitor Comparison

**Current**: "Built hardware equivalent to $10k+ professional traffic counters"

**Better**: 
```markdown
"Professional traffic counters from Jamar or Eco-Counter cost $3,000-10,000 
to purchase or $500-1,500 to rent. Your $150 DIY radar provides the same 
core data for a fraction of the cost."
```

**Time**: 15 minutes (research + write)

---

## Priority Tiers

### Tier 1: Must Fix Before Publication (8-10 hours)

1. ✅ Fix brand voice violations (1 hour)
2. ✅ Add prerequisites section (30 min)
3. ✅ Reduce wordiness - comprehensive edit (3 hours)
4. ✅ Add navigation (TOC, cross-refs) (1 hour)
5. ✅ Fix scannability (text walls) (2 hours)
6. ✅ Add success criteria to steps (1.5 hours)

**Result**: Readable, professional documentation

---

### Tier 2: High Value Improvements (6-8 hours)

7. ✅ Restructure deployment paths (3-4 hours)
8. ✅ Reorganize parts list (2-3 hours)
9. ✅ Standardize terminology (1 hour)

**Result**: Clear information architecture

---

### Tier 3: Complete Thompson Treatment (4-6 hours)

10. ✅ Create visual documentation (4-6 hours)

**Result**: Professional, publication-ready guide

---

### Tier 4: Nice-to-Have Enhancements (3-4 hours)

11. Add audience segmentation (15 min)
12. Relocate legal guidance (30 min)
13. Add advocacy guide (1 hour)
14. Add code block context (30 min)
15. Add uninstall instructions (15 min)
16. Fix DRY violations (30 min)
17. Strengthen competitor claims (15 min)

**Result**: Comprehensive, user-centered documentation

---

## Implementation Approach

### Option A: Quick Wins (2-3 hours)

Focus on Tier 1 critical items only:
- Brand voice fixes
- Prerequisites section  
- Navigation aids
- Basic copy editing (cut 10-15%)

**Good for**: Fast improvement, minimal time investment  
**Result**: 70% better than current

---

### Option B: Professional Polish (10-12 hours)

Tier 1 + Tier 2:
- All critical fixes
- Structural reorganization
- Comprehensive copy edit

**Good for**: Publication-ready quality  
**Result**: 90% better than current

---

### Option C: Complete Thompson Treatment (18-22 hours)

Tier 1 + Tier 2 + Tier 3 + Tier 4:
- Everything in Option B
- Visual documentation
- All nice-to-have features

**Good for**: Flagship quality, long-term reference  
**Result**: 100% - exceeds DIY magazine standards

---

## Success Metrics

### Before Thompson Edit

- Word count: ~6,500
- Passive voice: 15+ instances
- Visual aids: 0
- Scannable callouts: 2
- Avg paragraph: 6 lines
- Flesch Reading Ease: ~55 (Fairly Difficult)

### After Thompson Edit (Option C)

- Word count: ~5,000 (23% reduction)
- Passive voice: <5 instances (67% reduction)
- Visual aids: 6-8 images
- Scannable callouts: 15+
- Avg paragraph: 4 lines
- Flesch Reading Ease: ~60-65 (Standard)

---

## Files to Update

**Primary**:
1. `docs/src/guides/setup.md` - Main edits

**Supporting** (for consistency):
2. `README.md` - Align terminology, reduce duplication
3. `tools/pdf-generator/README.md` - Match workflow descriptions
4. Create new images in `docs/src/images/`

---

## Testing Checklist

After Thompson edits:

- [ ] Read aloud test (natural flow?)
- [ ] Scan test (find info in 10 seconds?)
- [ ] Jargon check (all terms explained?)
- [ ] Fresh-eyes review (someone unfamiliar with project)
- [ ] Consistency check (matches other docs?)
- [ ] Link validation (all links work?)
- [ ] Screenshot verification (images match current UI?)

---

## Coordination with Ictinus Review

**Ictinus focuses on**: Technical accuracy, functionality, implementation  
**Thompson focuses on**: Writing quality, UX, brand voice, marketing

**Both reviews required for**:
- ✅ Content is correct (Ictinus)
- ✅ Content is clear (Thompson)
- ✅ Content is compelling (Thompson)
- ✅ Content is complete (Ictinus)

**Non-overlapping fixes**:
- Ictinus: Python venv path, PDF workflow verification, cost placeholder
- Thompson: Voice/tone, structure, scannability, visuals

**Overlapping fixes** (both noted):
- Troubleshooting section (Ictinus: content, Thompson: placement/format)
- Legal guidance (Ictinus: completeness, Thompson: positioning)

---

## Recommended Sequence

1. **Start with Ictinus critical fixes** (technical correctness)
   - Python venv path
   - PDF workflow verification
   - Cost estimate

2. **Then apply Thompson Tier 1** (writing quality)
   - Brand voice
   - Prerequisites
   - Navigation
   - Copy editing

3. **Then Tier 2** (structure)
   - Deployment paths
   - Parts list organization

4. **Then Tier 3** (visuals)
   - Screenshots
   - Diagrams

5. **Finally Tier 4** (enhancements)
   - Nice-to-have features

**Total time**: ~20-25 hours (Ictinus + Thompson combined)

---

## Next Steps

1. Review both analyses (Ictinus + Thompson)
2. Choose implementation approach (Quick Wins vs Complete Treatment)
3. Prioritize fixes based on publication timeline
4. Begin with Tier 1 critical items
5. Iterate based on feedback

---

**Thompson's note**: This document represents velocity.report to the DIY community. 
Worth investing in excellence. The content is solid—let's make the presentation 
match the quality of the engineering. ✨
