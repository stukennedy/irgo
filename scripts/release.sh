#!/bin/bash
set -e

# Release script for irgo
# Usage: ./scripts/release.sh [patch|minor|major]
#   patch: 0.2.2 -> 0.2.3 (default)
#   minor: 0.2.2 -> 0.3.0
#   major: 0.2.2 -> 1.0.0

BUMP_TYPE=${1:-patch}
VERSION_FILE="cmd/irgo/main.go"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

info() { echo -e "${GREEN}[INFO]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }

# Check we're in the right directory
if [[ ! -f "$VERSION_FILE" ]]; then
    error "Must run from repository root (can't find $VERSION_FILE)"
fi

# Check for uncommitted changes
if [[ -n $(git status --porcelain) ]]; then
    error "Working directory not clean. Commit or stash changes first."
fi

# Check we're on main branch
BRANCH=$(git branch --show-current)
if [[ "$BRANCH" != "main" ]]; then
    warn "Not on main branch (on $BRANCH). Continue? [y/N]"
    read -r response
    if [[ ! "$response" =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# Get current version from main.go
CURRENT_VERSION=$(grep 'var version' "$VERSION_FILE" | sed 's/.*"\(.*\)".*/\1/')
if [[ -z "$CURRENT_VERSION" ]]; then
    error "Could not parse current version from $VERSION_FILE"
fi
info "Current version: $CURRENT_VERSION"

# Parse version components
IFS='.' read -r MAJOR MINOR PATCH <<< "$CURRENT_VERSION"

# Calculate new version
case $BUMP_TYPE in
    major)
        MAJOR=$((MAJOR + 1))
        MINOR=0
        PATCH=0
        ;;
    minor)
        MINOR=$((MINOR + 1))
        PATCH=0
        ;;
    patch)
        PATCH=$((PATCH + 1))
        ;;
    *)
        error "Invalid bump type: $BUMP_TYPE (use patch, minor, or major)"
        ;;
esac

NEW_VERSION="$MAJOR.$MINOR.$PATCH"
TAG="v$NEW_VERSION"

info "New version: $NEW_VERSION ($BUMP_TYPE bump)"

# Check if tag already exists
if git rev-parse "$TAG" >/dev/null 2>&1; then
    error "Tag $TAG already exists!"
fi

# Confirm
echo ""
echo "This will:"
echo "  1. Update version in $VERSION_FILE to $NEW_VERSION"
echo "  2. Commit the change"
echo "  3. Create and push tag $TAG"
echo "  4. Create GitHub release"
echo ""
read -p "Continue? [y/N] " -r
if [[ ! $REPLY =~ ^[Yy]$ ]]; then
    echo "Aborted."
    exit 1
fi

# Update version in main.go
info "Updating version in $VERSION_FILE..."
sed -i '' "s/var version = \".*\"/var version = \"$NEW_VERSION\"/" "$VERSION_FILE"

# Verify the change
NEW_FILE_VERSION=$(grep 'var version' "$VERSION_FILE" | sed 's/.*"\(.*\)".*/\1/')
if [[ "$NEW_FILE_VERSION" != "$NEW_VERSION" ]]; then
    error "Failed to update version in file"
fi

# Run tests
info "Running tests..."
if ! go test ./... >/dev/null 2>&1; then
    warn "Some tests failed. Continue anyway? [y/N]"
    read -r response
    if [[ ! "$response" =~ ^[Yy]$ ]]; then
        git checkout "$VERSION_FILE"
        exit 1
    fi
fi

# Build to verify
info "Building..."
if ! go build ./... >/dev/null 2>&1; then
    error "Build failed"
fi

# Commit
info "Committing version bump..."
git add "$VERSION_FILE"
git commit -m "chore: release v$NEW_VERSION"

# Create tag
info "Creating tag $TAG..."
git tag -a "$TAG" -m "Release $TAG"

# Push
info "Pushing to origin..."
git push origin "$BRANCH"
git push origin "$TAG"

# Create GitHub release
info "Creating GitHub release..."

# Generate release notes from commits since last tag
LAST_TAG=$(git describe --tags --abbrev=0 HEAD^ 2>/dev/null || echo "")
if [[ -n "$LAST_TAG" ]]; then
    COMMITS=$(git log --oneline "$LAST_TAG"..HEAD~1 | head -20)
    COMPARE_URL="https://github.com/stukennedy/irgo/compare/$LAST_TAG...$TAG"
else
    COMMITS=$(git log --oneline -10 HEAD~1)
    COMPARE_URL=""
fi

RELEASE_NOTES="## What's Changed

$COMMITS

## Installation

\`\`\`bash
go install github.com/stukennedy/irgo/cmd/irgo@$TAG
\`\`\`
"

if [[ -n "$COMPARE_URL" ]]; then
    RELEASE_NOTES="$RELEASE_NOTES
## Full Changelog
$COMPARE_URL"
fi

gh release create "$TAG" \
    --title "Release $TAG" \
    --notes "$RELEASE_NOTES"

# Verify installation works
info "Verifying installation from Go proxy..."
sleep 2

# Trigger proxy to index the new version
curl -s "https://proxy.golang.org/github.com/stukennedy/irgo/@v/$TAG.info" >/dev/null

echo ""
echo -e "${GREEN}========================================${NC}"
echo -e "${GREEN}  Release $TAG complete!${NC}"
echo -e "${GREEN}========================================${NC}"
echo ""
echo "Install with:"
echo "  go install github.com/stukennedy/irgo/cmd/irgo@$TAG"
echo ""
echo "Or:"
echo "  go install github.com/stukennedy/irgo/cmd/irgo@latest"
echo ""
