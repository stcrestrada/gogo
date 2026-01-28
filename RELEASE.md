# Release Process

This document describes how to create a new release of gogo.

## Prerequisites

1. Install GoReleaser:
   ```bash
   brew install goreleaser/tap/goreleaser
   # or
   go install github.com/goreleaser/goreleaser@latest
   ```

2. Ensure you have push access to the repository

3. Make sure all changes are committed and pushed to `master`

## Automated Release (Recommended)

We use GoReleaser with GitHub Actions for automated releases.

### Quick Release

Use the helper script:

```bash
chmod +x scripts/release.sh
./scripts/release.sh
```

The script will:
1. Prompt you for the new version
2. Validate you're on the master branch
3. Run tests
4. Create and push a git tag
5. GitHub Actions takes over and:
   - Runs GoReleaser
   - Creates a GitHub release
   - Generates changelog
   - Updates Homebrew tap (if configured)

### Manual Release Steps

If you prefer to do it manually:

```bash
# 1. Ensure all tests pass
make test

# 2. Create a new tag
git tag -a v3.0.0 -m "Release v3.0.0"

# 3. Push the tag
git push origin v3.0.0

# 4. GitHub Actions will automatically run GoReleaser
```

## Local Testing

Test the release process locally before pushing:

```bash
# Dry run (creates snapshot, doesn't publish)
goreleaser release --snapshot --clean

# Check what would be released
goreleaser check
```

## Version Numbering

Follow [Semantic Versioning](https://semver.org/):

- **MAJOR** (v3.0.0): Breaking API changes
  - Example: Changing function signatures, removing functions
- **MINOR** (v2.1.0): New features, backwards compatible
  - Example: Adding new functions, new convenience methods
- **PATCH** (v2.0.1): Bug fixes, backwards compatible
  - Example: Fixing bugs, updating docs

### Current Version History

- **v3.0.0**: Flattened API (breaking), added Map/ForEach/Collect
- **v2.0.0**: Added context support (breaking)
- **v1.0.1**: Bug fixes
- **v1.0.0**: Initial release

## What Gets Released

When you push a tag, GoReleaser will:

1. **Run Tests**: Ensures code quality
2. **Generate Changelog**: Automatically from git commits
3. **Create GitHub Release**: With release notes
4. **Build Artifacts**: (Currently skipped for libraries)
5. **Update Homebrew**: Formula in homebrew-tap repo (if configured)

## Changelog Format

Use conventional commits for better changelogs:

- `feat:` - New features (shows in "New Features")
- `fix:` - Bug fixes (shows in "Bug Fixes")
- `perf:` - Performance improvements
- `breaking:` - Breaking changes
- `docs:` - Documentation (excluded from changelog)
- `test:` - Tests (excluded from changelog)
- `chore:` - Maintenance (excluded from changelog)

Example:
```bash
git commit -m "feat: add Collect() method for easier result gathering"
git commit -m "breaking: flatten Pool API to remove nested functions"
git commit -m "fix: correct context propagation in ForEach"
```

## Homebrew Tap (Optional)

The ruby file is the Homebrew formula, generated automatically by GoReleaser.

### Setup Homebrew Tap

1. Create a homebrew-tap repository:
   ```bash
   gh repo create homebrew-tap --public
   ```

2. Update `.goreleaser.yaml` if needed (already configured)

3. (Optional) Create a `TAP_GITHUB_TOKEN` secret in GitHub:
   - Settings → Secrets → Actions
   - Add `TAP_GITHUB_TOKEN` with a personal access token

Users can then install with:
```bash
brew tap stcrestrada/tap
brew install gogo
```

## Troubleshooting

### Release Failed

1. Check GitHub Actions logs: https://github.com/stcrestrada/gogo/actions
2. Common issues:
   - Tests failed: Fix tests before releasing
   - Permission denied: Check `GITHUB_TOKEN` permissions
   - Tag already exists: Delete tag and re-create

### Delete a Tag

If you need to redo a release:

```bash
# Delete local tag
git tag -d v3.0.0

# Delete remote tag
git push --delete origin v3.0.0

# Recreate and push
git tag -a v3.0.0 -m "Release v3.0.0"
git push origin v3.0.0
```

## Post-Release

After a successful release:

1. Check the GitHub release page: https://github.com/stcrestrada/gogo/releases
2. Verify changelog is correct
3. Update any external documentation if needed
4. Announce the release (Twitter, Reddit, etc.)

## Release Checklist

Before releasing:

- [ ] All tests passing (`make test`)
- [ ] Documentation updated (README.md)
- [ ] Examples updated
- [ ] Breaking changes documented
- [ ] Version number follows semver
- [ ] On master/main branch
- [ ] Working directory clean
- [ ] Pulled latest changes
