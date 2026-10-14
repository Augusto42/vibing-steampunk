# Celebrate Milestone Release

You are celebrating a project milestone! Follow these steps to create a new release:

## 1. Gather Release Information

Ask the user for:
- **Version number** (e.g., v1.2.0) - suggest based on changes (major/minor/patch)
- **Release title** (e.g., "CLI & Cookie Authentication")
- **Key highlights** to include in release notes

## 2. Pre-Release Checklist

Run these checks and report results:

```bash
# Run all unit tests
go test ./...

# Check for uncommitted changes
git status

# Verify the sensitive files are not tracked.
# (Grepping all of history for the word "password" was the old check here. On a
# repo this size it takes minutes and returns mostly comments and field names,
# so it got skimmed — a check nobody reads is not a check.)
for f in .env cookies.txt .mcp.json; do
  git ls-files --error-unmatch "$f" >/dev/null 2>&1 \
    && echo "DANGER: $f is tracked" || echo "ok: $f not in the index"
done

# And scan the staged diff for the identifier families the sanitize policy names
git diff --cached | grep -nE \
  '\b[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\.[0-9]{1,3}\b|\b[A-Z][0-9]{2}K[0-9]{6}\b|\bDEVK[0-9]{6,}\b'
```

## 3. Update Documentation

If needed, update:
- README.md - version references, feature list
- CLAUDE.md - project status metrics
- reports/vsp-status.md - if exists

## 4. Commit & Push

```bash
# Stage and commit any pending changes
git add -A
git commit -m "Prepare release vX.Y.Z"

# Push to origin
git push origin main
```

## 5. Create Git Tag — BEFORE building

The tag has to exist first. `LDFLAGS` takes the version from `git describe`, so
a binary built before the tag reports the PREVIOUS release: someone downloading
v2.58.0 runs `vsp --version` and is told `v2.57.0-25-g977f968`. Tag, then build.

```bash
git tag -a vX.Y.Z -m "Release vX.Y.Z: <title>"
git push origin vX.Y.Z
```

## 6. Build All Platforms

**`build-all-all`, not `build-all`.** `build-all` builds only `PLATFORMS_COMMON`
— linux-amd64, darwin-arm64, windows-amd64. The other six are left at whatever
they were, so a release assembled after it ships stale binaries under a new
version number, and nothing about them looks wrong from the outside.

```bash
rm -f build/vsp-* build/checksums.txt   # so a stale one cannot survive
make build-all-all
```

Then verify, rather than trusting the timestamps:

```bash
ls build/vsp-*                                    # expect 9 + the local alias
./build/vsp-linux-amd64 --version                 # must print vX.Y.Z exactly
./build/vsp-linux-amd64 <a command added this release> --help   # the feature is really in there

cd build && sha256sum vsp-linux-* vsp-darwin-* vsp-windows-* > checksums.txt
```

## 7. Create GitHub Release

Use `gh release create` with:
- All binaries from `build/`
- Release notes highlighting key features
- Mark as latest release

```bash
gh release create vX.Y.Z \
  build/vsp-linux-amd64 \
  build/vsp-linux-arm64 \
  build/vsp-linux-386 \
  build/vsp-linux-arm \
  build/vsp-darwin-amd64 \
  build/vsp-darwin-arm64 \
  build/vsp-windows-amd64.exe \
  build/vsp-windows-arm64.exe \
  build/vsp-windows-386.exe \
  --title "vX.Y.Z: <title>" \
  --notes "$(cat <<'NOTES'
## What's New

- Feature 1
- Feature 2

## Downloads

| Platform | Architecture | File |
|----------|--------------|------|
| Linux | x64 | vsp-linux-amd64 |
| Linux | ARM64 | vsp-linux-arm64 |
| macOS | x64 | vsp-darwin-amd64 |
| macOS | Apple Silicon | vsp-darwin-arm64 |
| Windows | x64 | vsp-windows-amd64.exe |

## Installation

Download the appropriate binary, make it executable, and add to your PATH.

## Full Changelog

See commits since last release.
NOTES
)"
```

## 8. Celebrate!

Report the release URL and summary of what was shipped!
