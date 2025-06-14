#!/bin/bash

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 SSAnime GUI Release Script${NC}"
echo ""

# Check if we're in a git repository
if ! git rev-parse --git-dir > /dev/null 2>&1; then
    echo -e "${RED}❌ Not in a git repository${NC}"
    exit 1
fi

# Check if working directory is clean
if [[ -n $(git status --porcelain) ]]; then
    echo -e "${RED}❌ Working directory is not clean. Please commit or stash changes first.${NC}"
    git status --short
    exit 1
fi

# Get current version
CURRENT_VERSION=$(node -p "require('./package.json').version")
echo -e "📦 Current version: ${YELLOW}${CURRENT_VERSION}${NC}"

# Ask for version type
echo ""
echo "Select version bump type:"
echo "1) Patch (bug fixes) - e.g., 1.0.0 → 1.0.1"
echo "2) Minor (new features) - e.g., 1.0.0 → 1.1.0"  
echo "3) Major (breaking changes) - e.g., 1.0.0 → 2.0.0"
echo "4) Custom version"
echo "5) Cancel"

read -p "Enter your choice (1-5): " choice

case $choice in
    1)
        VERSION_TYPE="patch"
        ;;
    2)
        VERSION_TYPE="minor"
        ;;
    3)
        VERSION_TYPE="major"
        ;;
    4)
        read -p "Enter custom version (e.g., 1.2.3): " CUSTOM_VERSION
        if [[ ! $CUSTOM_VERSION =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            echo -e "${RED}❌ Invalid version format${NC}"
            exit 1
        fi
        VERSION_TYPE="$CUSTOM_VERSION"
        ;;
    5)
        echo -e "${YELLOW}Release cancelled${NC}"
        exit 0
        ;;
    *)
        echo -e "${RED}❌ Invalid choice${NC}"
        exit 1
        ;;
esac

# Confirm the action
echo ""
echo -e "${YELLOW}⚠️  This will:${NC}"
echo "   • Run tests"
echo "   • Bump version in package.json"
echo "   • Generate/update CHANGELOG.md"
echo "   • Create a git commit"
echo "   • Create a git tag"
echo "   • Push to remote repository"
echo ""
read -p "Continue? (y/N): " confirm

if [[ $confirm != [yY] ]]; then
    echo -e "${YELLOW}Release cancelled${NC}"
    exit 0
fi

# Run tests first
echo ""
echo -e "${BLUE}🧪 Running tests...${NC}"
if ! pnpm test; then
    echo -e "${RED}❌ Tests failed. Release cancelled.${NC}"
    exit 1
fi

# Run commit-and-tag-version
echo ""
echo -e "${BLUE}📝 Generating changelog and bumping version...${NC}"
if [[ $VERSION_TYPE =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    # Custom version
    pnpm commit-and-tag-version --release-as "$VERSION_TYPE"
else
    # Standard version type
    pnpm commit-and-tag-version --release-as "$VERSION_TYPE"
fi

if [ $? -ne 0 ]; then
    echo -e "${RED}❌ Version bump failed${NC}"
    exit 1
fi

# Get new version
NEW_VERSION=$(node -p "require('./package.json').version")

echo ""
echo -e "${GREEN}✅ Version bumped: ${CURRENT_VERSION} → ${NEW_VERSION}${NC}"

# Push to remote
echo ""
echo -e "${BLUE}🚀 Pushing to remote repository...${NC}"
git push --follow-tags origin main

if [ $? -eq 0 ]; then
    echo ""
    echo -e "${GREEN}🎉 Release completed successfully!${NC}"
    echo -e "🏷️  Tag: ${YELLOW}v${NEW_VERSION}${NC}"
    echo -e "🚀 Pushed to: ${YELLOW}origin/main${NC}"
    echo ""
    echo -e "${BLUE}ℹ️  The GitHub Actions workflow will automatically build and attach executables to the release.${NC}"
else
    echo -e "${RED}❌ Failed to push to remote repository${NC}"
    exit 1
fi
