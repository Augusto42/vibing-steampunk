# Upstream synchronization policy

This repository is an independently maintained public distribution derived from
[oisee/vibing-steampunk](https://github.com/oisee/vibing-steampunk).

## Repository roles

- `Augusto42/vibing-steampunk:main` is the canonical branch for this distribution.
- `oisee/vibing-steampunk:main` is a read-only upstream source.
- Community releases use tags such as `v2.38.1-augusto.1` to avoid ambiguity.

## Synchronization

The `Propose upstream sync` workflow fetches upstream and creates or updates an
`automation/upstream-sync` pull request. It never pushes directly to `main` and never enables
automatic merging. Conflicts fail closed and require manual review.

Before merging an upstream synchronization proposal:

1. Review the complete diff and release notes.
2. Confirm that local safety controls and community governance remain intact.
3. Run the full relevant test suite.
4. Resolve conflicts explicitly; never discard local changes wholesale.
5. Verify that no workflow permission or secret boundary was weakened.

Generic local fixes may also be proposed upstream, but this distribution does not depend on
upstream approval to ship them.
