#!/bin/bash

# Retry Release Script
# Usage: ./scripts/retry-release.sh <version> [force]
# Example: ./scripts/retry-release.sh v1.2.3
# Example: ./scripts/retry-release.sh v1.2.3 force

set -e

VERSION="$1"
FORCE="$2"

if [ -z "$VERSION" ]; then
    echo "❌ Error: Version is required"
    echo "Usage: $0 <version> [force]"
    echo "Example: $0 v1.2.3"
    exit 1
fi

# Validate version format
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+.*$ ]]; then
    echo "❌ Error: Version must start with 'v' and follow semantic versioning (e.g., v1.2.3)"
    exit 1
fi

echo "🔄 Retrying release for version: $VERSION"

# Check if tag exists
if git rev-parse "$VERSION" >/dev/null 2>&1; then
    echo "⚠️  Tag $VERSION already exists"
    
    if [ "$FORCE" = "force" ]; then
        echo "🔨 Force flag detected, deleting existing tag..."
        git tag -d "$VERSION" 2>/dev/null || true
        git push origin --delete "$VERSION" 2>/dev/null || true
        echo "✅ Existing tag deleted"
    else
        echo "❌ Tag already exists. Use 'force' argument to recreate it."
        echo "Command: $0 $VERSION force"
        exit 1
    fi
fi

# Get current commit
CURRENT_COMMIT=$(git rev-parse HEAD)
echo "📍 Current commit: $CURRENT_COMMIT"

# Create new tag on current commit
echo "🏷️  Creating tag $VERSION on latest commit..."
git tag -a "$VERSION" -m "Release $VERSION"

# Push the tag
echo "📤 Pushing tag to remote..."
git push origin "$VERSION"

echo "✅ Tag $VERSION created and pushed successfully!"
echo "🚀 GitHub Actions will now automatically start the release build."
echo "📊 Monitor progress at: https://github.com/$(git config --get remote.origin.url | sed 's/.*github.com[:/]\([^.]*\).*/\1/')/actions"
