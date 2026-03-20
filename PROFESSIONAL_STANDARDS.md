# Gorkbot Professional Repository Standards

**Effective Date:** March 20, 2026
**Status:** ENFORCED via .gitignore and code review process

---

## 🎯 Golden Rule

**ONLY PRODUCTION-READY, PROFESSIONAL CODE GOES IN THE PUBLIC REPOSITORY.**

Everything else belongs in:
- Development branches (never merged to main)
- The private development repo (`~/project/gorky`)
- Local `.gitignore` exclusions
- Separate plugin/extension repositories

---

## ✅ APPROVED FOR PUBLIC REPO

### Source Code
- `cmd/` - Executable entry points (production builds only)
- `internal/` - Core engine, TUI, platform abstraction
- `pkg/` - Public packages and tools

### Documentation
- `README.md` - Project overview (comprehensive, professional)
- `GETTING_STARTED.md` - User guide (clear, complete)
- `CLAUDE.md` - Architecture guide (detailed, technical)
- `VERSIONING.md` - Version strategy (clear semantics)
- `docs/` - Reference documentation (professional quality)
- `PROFESSIONAL_STANDARDS.md` - This file

### Configuration & Build
- `Makefile` - Build targets (documented, tested)
- `go.mod`, `go.sum` - Go dependencies
- `.env.example` - Configuration template (sanitized, no real values)
- `.gitignore` - Comprehensive exclusions (enforced)
- `.github/workflows/` - CI/CD pipelines

### Optional (With Approval)
- `ext/llama.cpp/` - Optional C++ submodule (build artifacts in .gitignore)

---

## ❌ NEVER IN PUBLIC REPO

### Development & Debugging
- `bash_demo/` - Test data and demo files
- `blockmosaic/` - Standalone UI modules
- `components/` - UI component playgrounds
- `mcp_servers/` - Python MCP server development
- `orchestrator/` - Internal MCP management code
- `plugins/` - Python plugin development (100+ auto-forged artifacts)
- `server/` - Python server development
- `scripts/` - Build scripts and dev utilities
- `configs/` - Test configurations

### Generated & Dynamic Code
- `pkg/tools/custom/` - Dynamically created tools
- `pkg/vision/captures/` - Screen captures
- `*_generated.go` - Generated source files
- `*_pb2.py` - Protobuf generated code

### Data Files & Caches
- `*.db`, `*.sqlite*` - Database files
- `.cache/` - Caches
- `tmp/`, `temp/` - Temporary files
- `*.jsonl` - Log files (except documentation examples)
- `vector_store.json` - Development data
- `__pycache__/` - Python cache
- `node_modules/` - Node dependencies
- `venv/`, `.venv/` - Python virtual environments

### Configuration & Secrets
- `.env` - API keys (use `.env.example` instead)
- `*.pem`, `*.key`, `*.token` - Cryptographic material
- `*credentials*.json` - Any credential files
- `*secret*.json` - Secret data

### Test Data & Artifacts
- `describe_test_photo.jpg` - Test images
- `front_camera.jpg` - Device capture tests
- `GORKBOT_SYSTEM_REPORT.txt` - Test reports
- `*_report.txt` - Generated reports
- `*audit*.txt`, `*audit*.json` - Audit logs

### IDE & OS Files
- `.vscode/project.json` - IDE project settings
- `.idea/` - IntelliJ IDEA settings
- `*.iml` - IntelliJ project files
- `.DS_Store` - macOS metadata
- `Thumbs.db` - Windows metadata

---

## 🛡️ ENFORCEMENT MECHANISMS

### 1. Comprehensive .gitignore
The `.gitignore` file includes:
- ✅ All development directories
- ✅ Generated/temporary files
- ✅ Cache and data directories
- ✅ Configuration (except templates)
- ✅ IDE and OS files
- ✅ Python artifacts

**Update .gitignore when adding new development directories.**

### 2. Pre-Commit Checks
Before pushing, verify:
```bash
# Check what would be committed
git status

# Verify no sensitive files
git diff --cached | grep -E "secret|password|key|token|credential" && echo "⚠️ FOUND SECRETS" || echo "✓ No secrets detected"

# List staged files
git diff --cached --name-only | head -20
```

### 3. Code Review Standards
**All PRs must pass:**
- [ ] No development artifacts included
- [ ] No test data or demo files
- [ ] No generated code or caches
- [ ] No configuration files (except templates with placeholders)
- [ ] No IDE or OS files
- [ ] No sensitive data (secrets, credentials)
- [ ] Documentation is professional quality

### 4. CI/CD Integration
GitHub Actions should verify:
- ✅ Python syntax (no `py_compile` errors)
- ✅ Go builds cleanly (`go build ./...`)
- ✅ Tests pass (`go test ./...`)
- ✅ No large binary files committed
- ✅ Secret scanning passes

---

## 📋 Directory Classification

### TIER 1: Core Production Code
Must always be in public repo:
- `cmd/` - Entry points
- `internal/` - Private implementation
- `pkg/` - Public packages
- `docs/` - Professional documentation
- `Makefile`, `go.mod`, `go.sum`

**Allowed Changes:** Bug fixes, features, tests, refactoring

### TIER 2: Configuration & Build
Allowed if professional:
- `.env.example` - Template with placeholder values ONLY
- `.github/workflows/` - CI/CD pipelines
- `.gitignore` - Comprehensive exclusions
- `PROFESSIONAL_STANDARDS.md` - This file

**Allowed Changes:** CI/CD improvements, build configuration

### TIER 3: Development Only
NEVER in public repo:
- Everything else
- Any test utilities, demo code, development tools
- Generated files, caches, compiled artifacts

**Where to put them:**
- Private dev repo: `~/project/gorky`
- Dev branches (never merge to main)
- `.gitignore` local exclusions
- Separate plugin repositories

---

## 🚀 Workflow for Development

### When Working on Features
1. **Make changes locally** (in private dev repo or branch)
2. **Test thoroughly** (with test files in .gitignore)
3. **When ready for public:**
   - Clean up all development artifacts
   - Ensure documentation is professional
   - Verify no sensitive data
   - Create minimal, focused PR
   - One feature per PR

### When Adding Tools or Plugins
1. **Develop in private repo** (`~/project/gorky`)
2. **Generate final production code** in `pkg/tools/` or `pkg/plugins/`
3. **Delete development/auto-generated files**
4. **Document with professional quality**
5. **PR contains ONLY final code**

### When Syncing from Dev Repo
1. **Filter out** all development artifacts
2. **Keep ONLY** production-ready code
3. **Update VERSIONING.md** and changelog
4. **Run cleanup** (verify git status)
5. **Comprehensive PR** with production code only

---

## 📊 Current State (Post-Cleanup)

**Total files cleaned:** 234
**Directories removed:** 8 (bash_demo, blockmosaic, components, configs, mcp_servers, orchestrator, plugins, server, scripts)
**Reduction:** ~60% of non-essential files removed

**Public Repo Now Contains:**
- ✅ 261 Go source files (production code)
- ✅ Comprehensive documentation (4900+ lines)
- ✅ Professional build system
- ✅ CI/CD pipelines
- ✅ Clear .gitignore enforcement

---

## ✋ How to Maintain Standards

### For Developers
1. **Check .gitignore** before committing
2. **Never commit** test data, demo files, or generated code
3. **Run `git status`** to verify what you're committing
4. **Keep PRs focused** on production code only
5. **Document professionally** at publication time

### For Maintainers
1. **Review PR file lists** - watch for suspicious directories
2. **Verify .gitignore** updates if new dev directories created
3. **Reject PRs** with non-professional content
4. **Enforce standards** consistently
5. **Update PROFESSIONAL_STANDARDS.md** as rules evolve

### For CI/CD
1. **Lint checks** for code quality
2. **Syntax validation** for all code
3. **Build verification** to ensure nothing breaks
4. **Size checks** to catch large files
5. **Secret scanning** to prevent credential leaks

---

## 🎓 Examples

### ❌ WRONG: PR includes development artifacts
```
modified:   cmd/gorkbot/main.go        ✓ OK
new file:   cmd/gorkbot/test_main.go   ✓ OK (test file, if needed)
new file:   plugins/python/auto_forged/new_tool.py  ❌ REJECT
deleted:    some_old_file              ✓ OK
```

### ✅ RIGHT: PR contains only production code
```
modified:   pkg/tools/registry.go      ✓ Production code
modified:   internal/engine/orchestrator.go  ✓ Production code
modified:   pkg/ai/grok.go             ✓ Production code
modified:   docs/TOOL_SYSTEM_DESIGN.md ✓ Professional docs
modified:   README.md                  ✓ Professional docs
```

---

## 📞 Questions?

Refer to:
- `CLAUDE.md` - Architecture and development guide
- `GETTING_STARTED.md` - User guide
- `README.md` - Project overview
- This file (PROFESSIONAL_STANDARDS.md)

**When in doubt: Production code only, everything else private.**

---

**Last Updated:** March 20, 2026
**Enforced By:** .gitignore, Code Review, CI/CD
**Violations:** PR rejection, forced cleanup
