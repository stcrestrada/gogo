#!/bin/bash
set -e

# Release script for gogo
# This script helps create a new release with GoReleaser

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Check if goreleaser is installed
if ! command -v goreleaser &> /dev/null; then
    echo -e "${RED}GoReleaser is not installed!${NC}"
    echo "Install it with:"
    echo "  brew install goreleaser/tap/goreleaser"
    echo "  or"
    echo "  go install github.com/goreleaser/goreleaser@latest"
    exit 1
fi

# Get current version
CURRENT_VERSION=$(git describe --tags --abbrev=0 2>/dev/null || echo "v0.0.0")
echo -e "${GREEN}Current version: ${CURRENT_VERSION}${NC}"

# Ask for new version
echo -e "${YELLOW}Enter new version (e.g., v3.0.0):${NC}"
read -r NEW_VERSION

# Validate version format
if [[ ! $NEW_VERSION =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo -e "${RED}Invalid version format. Use vX.Y.Z (e.g., v3.0.0)${NC}"
    exit 1
fi

echo -e "${YELLOW}Creating release ${NEW_VERSION}...${NC}"

# Make sure we're on master/main
BRANCH=$(git branch --show-current)
if [[ "$BRANCH" != "master" && "$BRANCH" != "main" ]]; then
    echo -e "${RED}You must be on master/main branch to release${NC}"
    exit 1
fi

# Make sure working directory is clean
if [[ -n $(git status -s) ]]; then
    echo -e "${RED}Working directory is not clean. Commit or stash your changes.${NC}"
    exit 1
fi

# Pull latest
echo "Pulling latest changes..."
git pull origin "$BRANCH"

# Run tests
echo "Running tests..."
make test

# Create and push tag
echo "Creating tag ${NEW_VERSION}..."
git tag -a "$NEW_VERSION" -m "Release ${NEW_VERSION}"

echo -e "${YELLOW}Pushing tag to GitHub...${NC}"
git push origin "$NEW_VERSION"

echo -e "${GREEN}✅ Tag pushed! GitHub Actions will now:${NC}"
echo "  1. Run tests"
echo "  2. Run GoReleaser"
echo "  3. Create a GitHub release with changelog"
echo "  4. Update Homebrew tap (if configured)"
echo ""
echo "Check progress at: https://github.com/stcrestrada/gogo/actions"
echo ""
echo -e "${YELLOW}To test the release locally before pushing, run:${NC}"
echo "  goreleaser release --snapshot --clean"
