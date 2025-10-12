# Implementation Guide: PDF Generator Organization

**For**: velocity.report Go monorepo
**Component**: Python PDF generator utility
**Goal**: Respect Go project structure, improve Python tooling usability

---

## 🎯 Quick Decision Matrix

| If you want... | Use this approach | Time needed |
|---|---|---|
| **Quick wins today** | Add Makefile commands only | 15 minutes |
| **Proper organization** | Move to `tools/pdf-generator/` | 2-3 hours |
| **Best practice** | Move + restructure + pyproject.toml | 1 day |

---

## ⚡ Fastest Path (15 minutes)

### What you get
- Simple commands: `make pdf-report CONFIG=config.json`
- No file moves, minimal risk
- Better than typing long paths

### Steps

1. **Copy Makefile commands**
   ```bash
   cat Makefile.python-example >> Makefile
   ```

2. **Test it**
   ```bash
   make pdf-setup
   make pdf-config
   make pdf-report CONFIG=config.example.json
   ```

3. **Done!** You now have npm-style commands for Python.

---

## 🏗️ Recommended Path (2-3 hours)

### What you get
- Clean separation: Go code vs Python tools
- Respects monorepo structure (`tools/` for utilities)
- Sets foundation for future improvements
- Organized output directory

### Steps

1. **Run migration script**
   ```bash
   chmod +x migrate-pdf-generator-to-tools.sh
   ./migrate-pdf-generator-to-tools.sh
   ```

2. **Update Go integration code**

   Find and replace in your Go files:
   ```go
   // OLD
   cmd := exec.Command("python3", "internal/report/query_data/get_stats.py", configPath)

   // NEW
   cmd := exec.Command("python3", "tools/pdf-generator/get_stats.py", configPath)
   cmd.Dir = "tools/pdf-generator"
   ```

3. **Test the migration**
   ```bash
   make pdf-setup
   make pdf-test
   ```

4. **Verify Go integration**
   ```bash
   # Run your Go app that calls the PDF generator
   # Ensure it still works
   ```

5. **Clean up old location** (after verification)
   ```bash
   rm -rf internal/report/query_data
   ```

### New structure
```
velocity.report/
├── cmd/                    # Go binaries
├── internal/               # Go packages ONLY
│   ├── api/
│   ├── db/
│   ├── lidar/
│   └── radar/
├── tools/                  # ✨ NEW: Non-Go utilities
│   └── pdf-generator/      # Python PDF generator
│       ├── get_stats.py
│       ├── *.py           # Python modules
│       ├── tests/         # Test files
│       ├── output/        # Generated PDFs
│       └── requirements.txt
├── web/                    # Frontend
└── Makefile
```

---

## 🚀 Best Practice Path (1 day)

### What you get
- Everything from Recommended path
- Proper Python package structure
- Clear separation of CLI vs internal code
- Can install as standalone tool
- Professional Python conventions

### Additional steps after Recommended path

1. **Reorganize into package structure**
   ```
   tools/pdf-generator/
   ├── pyproject.toml
   ├── pdf_generator/          # Python package
   │   ├── __init__.py
   │   ├── cli/               # Entry points
   │   │   ├── main.py        # Was: get_stats.py
   │   │   ├── create_config.py
   │   │   └── api_server.py  # Was: generate_report_api.py
   │   ├── core/              # Internal modules
   │   │   ├── api_client.py
   │   │   ├── chart_builder.py
   │   │   └── ...
   │   └── tests/
   └── output/
   ```

2. **Add pyproject.toml** (see `pyproject.toml.example`)

3. **Update imports** throughout Python code

4. **Install as package**
   ```bash
   cd tools/pdf-generator
   pip install -e .
   ```

5. **Update Go to use module syntax**
   ```go
   cmd := exec.Command("python3", "-m", "pdf_generator.cli.main", configPath)
   cmd.Dir = "tools/pdf-generator"
   ```

---

## 📋 Files Created for You

| File | Purpose | When to use |
|---|---|---|
| `PROPOSAL_USABILITY_IMPROVEMENTS.md` | Full detailed proposal | Read for context |
| `Makefile.python-example` | Ready-to-use Makefile commands | Copy to Makefile |
| `migrate-pdf-generator-to-tools.sh` | Automated migration script | Run for Recommended path |
| `pyproject.toml.example` | Python package config | Use for Best Practice path |

---

## 🔧 Go Integration Examples

### Current (before changes)
```go
cmd := exec.Command(
    "python3",
    "internal/report/query_data/get_stats.py",
    configPath,
)
```

### After moving to tools/
```go
// Option 1: Direct script call
pythonBin := filepath.Join(rootDir, "tools", "pdf-generator", ".venv", "bin", "python3")
script := filepath.Join(rootDir, "tools", "pdf-generator", "get_stats.py")
cmd := exec.Command(pythonBin, script, configPath)

// Option 2: Let system find python, set working dir
cmd := exec.Command("python3", "get_stats.py", configPath)
cmd.Dir = filepath.Join(rootDir, "tools", "pdf-generator")
```

### After restructuring as package
```go
cmd := exec.Command("python3", "-m", "pdf_generator.cli.main", configPath)
cmd.Dir = filepath.Join(rootDir, "tools", "pdf-generator")
```

---

## ✅ Verification Checklist

After migration, verify:

- [ ] Makefile commands work
  ```bash
  make pdf-setup
  make pdf-test
  make pdf-report CONFIG=config.example.json
  ```

- [ ] Go integration still works
  ```bash
  # Run your Go app that generates PDFs
  # Verify PDFs are created correctly
  ```

- [ ] Tests pass
  ```bash
  make pdf-test
  ```

- [ ] PDFs go to correct output directory
  ```bash
  ls tools/pdf-generator/output/
  ```

- [ ] CI/CD updated (if applicable)

---

## 🤔 Common Questions

**Q: Will this break existing functionality?**
A: Not if you update Go code to point to new paths. The migration script copies (doesn't move) so old location remains as backup.

**Q: Do I have to do the full restructure?**
A: No! Start with just adding Makefile commands. Move to `tools/` when convenient. Package restructure is optional.

**Q: What if I want to keep it in internal/ for now?**
A: Fine! Just add the Makefile commands. The `PDF_DIR` variable makes it easy to move later.

**Q: Can I name it something other than pdf-generator?**
A: Yes! Common options: `pdf-generator`, `report-generator`, `velocity-pdf-gen`

**Q: What about the .venv directory?**
A: It stays with the tool in `tools/pdf-generator/.venv` - keeps it isolated from Go project.

---

## 📞 Next Steps

1. **Read**: `PROPOSAL_USABILITY_IMPROVEMENTS.md` for full context
2. **Choose**: Pick your path (Fastest/Recommended/Best Practice)
3. **Execute**: Follow the steps for your chosen path
4. **Verify**: Run the verification checklist
5. **Iterate**: Can always upgrade to next level later

Good luck! The fastest path takes 15 minutes and gives you immediate benefits. 🚀
