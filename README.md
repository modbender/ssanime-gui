# SSAnime GUI

<p align="center">
  <img width="140" src="public/logo.svg" >
</p>

A modern video encoding GUI built with Nuxt 3 and Electron, designed for efficient anime video processing.

## Features

- 🎬 Multi-format video encoding (MP4, MKV, AVI, MOV)
- ⚙️ Customizable encoding profiles
- 🔄 Batch processing with queue management
- 🎨 Modern dark/light theme interface
- 📊 Real-time encoding progress tracking
- 💾 Profile management with custom settings

## Development

### Prerequisites

- Node.js 18+
- pnpm (recommended package manager)

### Setup

```bash
# Install dependencies
pnpm install

# Start development server
pnpm dev
```

### Building

```bash
# Build for production
pnpm build

# Build only Nuxt app
pnpm build:nuxt

# Build only Electron app
pnpm build:electron
```

## Releases

This project uses automated versioning and releases with `commit-and-tag-version`. See [Release Process Documentation](docs/release-process.md) for details.

### Quick Release

```bash
# Interactive release script (Unix/macOS)
./scripts/release.sh

# Or use NPM scripts (cross-platform)
pnpm run release:patch  # Bug fixes
pnpm run release:minor  # New features
pnpm run release:major  # Breaking changes

# Or use Node.js scripts
pnpm run version:patch
pnpm run version:minor
pnpm run version:major
```

### Automated Builds

When you push a version tag, GitHub Actions automatically:

- Builds executables for Windows, macOS, and Linux
- Creates a GitHub release with downloadable assets
- Generates changelog from conventional commits

This project uses GitHub Actions to automatically build multiarch executables when you push a version tag.

### Supported Platforms & Architectures

**Windows:**

- x64 (64-bit Intel/AMD)
- ia32 (32-bit Intel/AMD)

**macOS:**

- Universal Binary (Intel + Apple Silicon)

**Linux:**

- x64 (64-bit Intel/AMD)
- arm64 (ARM64/AArch64)

### Package Formats

- **Windows**: `.exe` (NSIS installer), `.msi` (Windows Installer), `.zip` (portable)
- **macOS**: `.dmg` (disk image), `.zip` (portable)
- **Linux**: `.AppImage` (portable), `.deb` (Debian/Ubuntu), `.rpm` (RHEL/SUSE), `.tar.gz` (archive)

### Creating a Release

1. Update the version in `package.json`
2. Create and push a version tag:
   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```
3. The GitHub Actions workflow will automatically:
   - Build for all supported platforms and architectures in parallel
   - Create a GitHub release with generated release notes
   - Attach all built executables as downloadable assets

### Manual Testing

You can manually trigger test builds without creating a release using the "Test Build" workflow in the GitHub Actions tab.

## Template

This template is based on the official template of Nuxt. You can find it in the clues below.

👉 https://github.com/nuxt/cli/blob/v3.11.1/src/commands/init.ts#L11-L13

## How to work

This quick-start is only a combination of [nuxt](https://github.com/nuxt) and [electron-vite](https://github.com/electron-vite) . You can refer to their official docs separately to learn more.
