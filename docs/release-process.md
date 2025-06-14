# Release Process

This document describes the automated release process for SSAnime GUI using `commit-and-tag-version`.

## Quick Release (Recommended)

### Using the Interactive Script

```bash
# Run the interactive release script
./scripts/release.sh
```

### Using NPM Scripts

```bash
# Patch release (bug fixes): 1.0.0 → 1.0.1
pnpm run release:patch

# Minor release (new features): 1.0.0 → 1.1.0
pnpm run release:minor

# Major release (breaking changes): 1.0.0 → 2.0.0
pnpm run release:major
```

### Using Node.js Scripts

```bash
# Simple version bump with custom script
pnpm run version:patch
pnpm run version:minor
pnpm run version:major
```

## Advanced Usage

### Using commit-and-tag-version Directly

```bash
# Automatic version bump based on conventional commits
pnpm release

# Force specific version type
pnpm commit-and-tag-version --release-as patch
pnpm commit-and-tag-version --release-as minor
pnpm commit-and-tag-version --release-as major

# Custom version
pnpm commit-and-tag-version --release-as 1.2.3

# Dry run (preview changes)
pnpm release:dry
```

### Manual Process

```bash
# 1. Run tests
pnpm test

# 2. Bump version
npm version patch  # or minor, major

# 3. Push with tags
git push --follow-tags origin main
```

## GitHub Actions Workflow

You can also trigger releases via GitHub Actions:

1. Go to **Actions** tab in your repository
2. Select **"Version Bump and Release"** workflow
3. Click **"Run workflow"**
4. Choose version type (patch/minor/major)
5. Optionally enable "dry run" to preview changes

## What Happens During Release

1. **Pre-release Checks**

   - Verifies working directory is clean
   - Runs full test suite (lint + format + type-check)
   - Ensures all tests pass

2. **Version Management**

   - Updates version in `package.json`
   - Generates/updates `CHANGELOG.md` based on conventional commits
   - Creates a release commit

3. **Git Operations**

   - Creates a git tag (e.g., `v1.2.3`)
   - Pushes commit and tag to remote repository

4. **Automated Build & Release**
   - GitHub Actions automatically detects the new tag
   - Builds executables for multiple platforms/architectures
   - Creates GitHub release with downloadable assets

## Conventional Commits

The changelog generation works best with conventional commit messages:

```bash
feat: add new encoding profile system
fix: resolve slider alignment issues
chore: update dependencies
docs: improve installation guide
style: format code with prettier
refactor: restructure component hierarchy
perf: optimize video encoding pipeline
test: add unit tests for profile manager
```

## Supported Platforms & Architectures

The automated build process creates executables for:

- **Windows**: x64, ia32 (.exe, .msi, .zip)
- **macOS**: Universal (Intel + Apple Silicon) (.dmg, .zip)
- **Linux**: x64, ARM64 (.AppImage, .deb, .rpm, .tar.gz)

## Pre-release Versions

For alpha, beta, or release candidate versions:

```bash
# Create pre-release
pnpm commit-and-tag-version --prerelease alpha
pnpm commit-and-tag-version --prerelease beta
pnpm commit-and-tag-version --prerelease rc

# Example output: 1.0.0-alpha.1, 1.0.0-beta.1, 1.0.0-rc.1
```

## Configuration

The release process is configured via `.versionrc.json`:

```json
{
  "types": [
    { "type": "feat", "section": "✨ Features" },
    { "type": "fix", "section": "🐛 Bug Fixes" },
    { "type": "chore", "section": "🔧 Maintenance" },
    { "type": "docs", "section": "📝 Documentation" },
    { "type": "style", "section": "💄 Styling" },
    { "type": "refactor", "section": "♻️ Code Refactoring" },
    { "type": "perf", "section": "⚡ Performance Improvements" },
    { "type": "test", "section": "✅ Tests" }
  ],
  "releaseCommitMessageFormat": "chore(release): {{currentTag}}",
  "tagPrefix": "v"
}
```

## Troubleshooting

### Release Failed

- Ensure working directory is clean
- Verify all tests pass locally
- Check network connectivity
- Ensure you have push permissions

### Version Already Exists

```bash
# Delete local tag
git tag -d v1.2.3

# Delete remote tag
git push origin :refs/tags/v1.2.3

# Try release again
```

### Rollback Release

```bash
# Reset to previous commit (before version bump)
git reset --hard HEAD~1

# Force push (⚠️ use with caution)
git push --force-with-lease origin main

# Delete the tag
git tag -d v1.2.3
git push origin :refs/tags/v1.2.3
```

## Examples

### Patch Release (Bug Fixes)

```bash
# Option 1: Interactive script
./scripts/release.sh

# Option 2: Direct command
pnpm run release:patch

# Option 3: Custom script
pnpm run version:patch
```

### Minor Release (New Features)

```bash
# Option 1: Interactive script
./scripts/release.sh

# Option 2: Direct command
pnpm run release:minor

# Option 3: Custom script
pnpm run version:minor
```

### Major Release (Breaking Changes)

```bash
# Option 1: Interactive script
./scripts/release.sh

# Option 2: Direct command
pnpm run release:major

# Option 3: Custom script
pnpm run version:major
```

### Dry Run (Preview Changes)

```bash
# See what would change without making changes
pnpm run release:dry
```

## Migration from standard-version

This project has been migrated from the deprecated `standard-version` to `commit-and-tag-version`. The API is identical, but the package is actively maintained and more secure.

Key differences:

- Package name: `commit-and-tag-version` instead of `standard-version`
- Same CLI options and configuration
- Better security and maintenance
- Continued development and bug fixes
