const fs = require('fs');
const { execSync } = require('child_process');
const path = require('path');

// Get the version type from command line argument
const versionType = process.argv[2] || 'patch';

if (!['major', 'minor', 'patch'].includes(versionType)) {
  console.error('❌ Invalid version type. Use: major, minor, or patch');
  process.exit(1);
}

try {
  console.log(`🚀 Starting ${versionType} version bump...`);

  // Read current package.json
  const packageJsonPath = path.join(__dirname, '..', 'package.json');
  const packageJson = JSON.parse(fs.readFileSync(packageJsonPath, 'utf8'));
  const currentVersion = packageJson.version;

  console.log(`📦 Current version: ${currentVersion}`);

  // Check if working directory is clean
  try {
    const gitStatus = execSync('git status --porcelain', { encoding: 'utf8' });
    if (gitStatus.trim()) {
      console.error(
        '❌ Working directory is not clean. Please commit or stash changes first.'
      );
      console.log(gitStatus);
      process.exit(1);
    }
  } catch (error) {
    console.error('❌ Not in a git repository or git not available');
    process.exit(1);
  }

  // Run tests first
  console.log('🧪 Running tests...');
  execSync('pnpm test', { stdio: 'inherit' });
  console.log('✅ Tests passed');

  // Bump version using npm version
  const newVersion = execSync(
    `npm version ${versionType} --no-git-tag-version`,
    {
      encoding: 'utf8',
    }
  )
    .trim()
    .replace('v', '');

  console.log(`✨ New version: ${newVersion}`);

  // Stage the package.json changes
  execSync('git add package.json pnpm-lock.yaml');

  // Commit the version bump
  const commitMessage = `chore(release): bump version to ${newVersion}`;
  execSync(`git commit -m "${commitMessage}"`);
  console.log(`✅ Committed: ${commitMessage}`);

  // Create and push the tag
  const tagName = `v${newVersion}`;
  execSync(`git tag ${tagName}`);
  console.log(`🏷️  Created tag: ${tagName}`);

  // Push commits and tags
  execSync('git push origin main');
  execSync(`git push origin ${tagName}`);
  console.log('🚀 Pushed commits and tags to remote');

  console.log(`
🎉 Version bump complete!
📦 Version: ${currentVersion} → ${newVersion}
🏷️  Tag: ${tagName}
🚀 Pushed to: origin/main

The GitHub Actions workflow will automatically build and create a release with executables.
  `);
} catch (error) {
  console.error('❌ Version bump failed:', error.message);
  process.exit(1);
}
