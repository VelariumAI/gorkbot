# Release Automation Guide

This document explains how automatic release automation works in Gorkbot.

## Overview

Gorkbot uses **Google Release Please** with GitHub Actions to automatically:
1. Detect commits merged to `main` branch
2. Analyze commit messages using [Conventional Commits](https://www.conventionalcommits.org/)
3. Determine semantic version bump (patch/minor/major)
4. Create/update release PR with changelog
5. Publish GitHub Release with binaries

## Automatic Workflow

### How It Triggers

When you merge commits to `main` branch, GitHub Actions automatically:

1. **Release Please Action** runs
   - Parses all commits since last release
   - Analyzes conventional commit types
   - Creates a "release PR" with version bump and changelog
   - Commits version updates

2. **Release PR Gets Merged**
   - Once merged, GitHub automatically creates a tag
   - Pushes trigger the second phase

3. **Binary Publishing**
   - Builds gorkbot and grokster for 5 platforms
   - Creates SHA256 checksums
   - Publishes GitHub Release with files and changelog

### Version Bumping Rules

Based on **Conventional Commits** format: `type(scope): description`

| Commit Type | Version Impact | Example |
|-------------|---|---------|
| `fix:` | **Patch** bump | `v5.3.0 → v5.3.1` |
| `feat:` | **Minor** bump | `v5.3.0 → v5.4.0` |
| `BREAKING CHANGE:` (in footer) | **Major** bump | `v5.3.0 → v6.0.0` |
| `chore:`, `docs:`, `refactor:` | **No bump** (if no fix/feat) | No release |

### Example Commit Messages

```bash
# Will trigger patch release (v5.3.0 → v5.3.1)
git commit -m "fix(tui): resolve viewport scrolling issue"

# Will trigger minor release (v5.3.0 → v5.4.0)
git commit -m "feat(tools): add new bash execution tool"

# Will trigger major release (v5.3.0 → v6.0.0)
git commit -m "feat(api): redesign tool system

BREAKING CHANGE: Old tool interface no longer supported"

# Will NOT trigger release
git commit -m "chore: cleanup debug code"
git commit -m "docs: update README"
```

## Release PR Process

### Step 1: Automatic PR Creation

When commits are pushed to `main`:

1. Release Please action runs
2. Creates a pull request titled: `chore(release): Automatic Version Bump v{VERSION}`
3. PR includes:
   - Updated CHANGELOG.md with all commits
   - Updated version constants in:
     - `pkg/version/version.go` (InternalVersion)
     - `internal/platform/env.go` (Version)
     - `.release-please-manifest.json` (current version tracking)
   - Full changelog of changes since last release

### Step 2: Review & Merge

1. **Review the PR**
   - Check version bump is correct
   - Review changelog entries
   - Verify no unintended changes

2. **Merge to main**
   - Use GitHub's "Merge pull request" button
   - Release Please detects merge
   - Automatically creates git tag (e.g., `v5.3.1`)
   - Triggers release publication workflow

### Step 3: Automatic Release Publishing

Once the release PR is merged:

1. **Build Phase**
   - Compiles gorkbot and grokster for 5 platforms:
     - Linux x86_64
     - macOS x86_64
     - macOS ARM64 (Apple Silicon)
     - Windows x86_64
   - Generates SHA256 checksums

2. **Release Phase**
   - Creates GitHub Release with:
     - Tag: v5.3.1 (or appropriate version)
     - Title: Version 5.3.1
     - Description: Auto-generated from PR body (changelog)
     - Attachments: All compiled binaries + SHA256SUMS

3. **Notification**
   - GitHub sends release notifications
   - Binaries available for download
   - Users can update via GitHub Releases page

## Manual Release (If Needed)

For emergency releases or if automation needs override:

```bash
# Create annotated tag
git tag -a v5.3.2 -m "Release v5.3.2 - critical fix"

# Push tag to trigger release workflow
git push origin v5.3.2
```

Note: This still requires manually updating version constants in code.

## Files Modified by Release Automation

### Automatically Updated
- `CHANGELOG.md` - New section added per release
- `pkg/version/version.go` - InternalVersion constant bumped
- `internal/platform/env.go` - Version constant bumped
- `.release-please-manifest.json` - Current version tracked

### Configuration Files (Don't Edit)
- `release-please-config.json` - Release Please settings
- `.github/workflows/release.yml` - Automation workflow
- `.release-please-manifest.json` - Version tracking

## Changelog Management

### Automatic Sections

Release Please automatically generates changelog sections based on commit types:

```markdown
## [5.4.0] - 2026-03-21

### 🎯 Features
- New tool system implementation
- Theme switching runtime support

### 🐛 Bug Fixes
- Fix viewport scrolling bug
- Resolve HITL notification spam

### 🔧 Refactoring
- Simplify internal structures

### ⚡ Performance
- Optimize memory usage in streaming
```

### Manual Edits

You can manually edit CHANGELOG.md for:
- Adding context to release notes
- Grouping related changes
- Removing insignificant changes
- Adding migration guides

Changes in unreleased section will be preserved.

## Troubleshooting

### Release PR Not Creating

**Symptoms**: No PR created after push to main

**Solutions**:
1. Check that `release-please-config.json` exists
2. Verify `.release-please-manifest.json` has correct version
3. Ensure commits use conventional commit format
4. Check GitHub Actions logs for errors

### Wrong Version Bumped

**Symptoms**: Release PR shows incorrect version

**Solutions**:
1. Check commit messages use correct conventional commit format
2. Verify `BREAKING CHANGE:` is in commit footer for major bump
3. Check `.release-please-manifest.json` for current version
4. Can manually edit PR before merging if needed

### Binaries Not Building

**Symptoms**: Release published but no binaries attached

**Solutions**:
1. Check GitHub Actions logs for build errors
2. Verify Go 1.25.0 is installed
3. Check for missing dependencies: `go mod download`
4. Verify build works locally: `go build ./cmd/gorkbot`

### Tag Already Exists

**Symptoms**: "tag already exists" error during release

**Solutions**:
1. Delete local tag: `git tag -d v5.3.1`
2. Delete remote tag: `git push origin --delete v5.3.1`
3. Let Release Please retry

## GitHub Actions Permissions

The release workflow requires these GitHub permissions:

- `contents: write` - Create releases and tags
- `pull-requests: write` - Create and update release PRs

These are configured in `.github/workflows/release.yml`.

## Best Practices

### Commit Messages
✅ DO:
- Use conventional commit format consistently
- Be specific about what changed
- Include issue numbers if applicable
- Use `BREAKING CHANGE:` footer for incompatible changes

❌ DON'T:
- Mix unrelated changes in one commit
- Use vague messages like "fix stuff"
- Forget to include `fix:`, `feat:`, etc. prefixes

### Release Cadence
- Patch releases (v.x.y) - As needed for bug fixes
- Minor releases (v.x) - Every 2-4 weeks for features
- Major releases - Plan ahead, announce in advance

### Testing
Before merging to main:
1. Test locally: `go build -o ./bin/grokster ./cmd/grokster`
2. Run tests: `go test ./...`
3. Ensure no regressions

## Configuration

### To Change Version Bumping Rules

Edit `release-please-config.json`:

```json
{
  "commit-parser": {
    "conventional": {
      "types": {
        "your-type": {
          "type": "feature|fix|refactor",
          "section": "Display Name"
        }
      }
    }
  }
}
```

### To Skip Files from Updates

Edit `release-please-config.json`:

```json
{
  "extra-files": [
    {
      "type": "json",
      "file": "package.json"
    }
  ]
}
```

## Release Checklist

Before final release:

- [ ] All tests pass
- [ ] CHANGELOG.md reviewed
- [ ] Version bump is correct
- [ ] Commit messages are conventional format
- [ ] No secrets in commits
- [ ] Documentation updated
- [ ] Binaries compile for all platforms

## Support

For issues with release automation:

1. Check GitHub Actions logs: **Actions** tab → select workflow → check logs
2. Verify commit format: `git log --oneline -10`
3. Check configuration files exist and are valid JSON
4. Review this guide for common issues

## References

- [Conventional Commits](https://www.conventionalcommits.org/)
- [Semantic Versioning](https://semver.org/)
- [Release Please Documentation](https://github.com/googleapis/release-please)
- [GitHub Actions Documentation](https://docs.github.com/en/actions)
