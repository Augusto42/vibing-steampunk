# Community fork integration matrix (2026-09-25)

This is a review of `oisee/vibing-steampunk:main` at `9886d27` and
`Augusto42/vibing-steampunk:main` at `d0d3681`. At this point the histories
diverge by 502 upstream and 33 community commits. This document is an
integration checklist, not a claim that the branches have been merged.

Only synthetic fixtures and non-production SAP systems may be used for this
work. No customer source, credentials, transport identifiers, or system data
belong in tests, logs, commits, pull requests, or releases.

| Capability | Current upstream | Community fork | Integration decision |
| --- | --- | --- | --- |
| Logical write/activation failures, package probes, deployment read-back | Present, including upstream PR #182 | Present; MCP diagnostic payload refined in fork PR #9 | Use current upstream implementation; retain fork's additional tests only where they still test distinct behavior. |
| SQLite without CGO | Pure-Go driver present | Pure-Go driver and portable cache-path handling | Use upstream driver; port the portable path creation/example fixes that still fail on Windows. |
| ENHO read | Upstream PR #213 adds a current ADT/RFC read path | Older ENHO read plus class/BAdI metadata fallbacks | Use upstream read path as base; compare subtype and classic-system fallbacks before removing fork code. |
| ENHO/XH update | No supported write path found | Classic enhancement write through the SAP framework bridge, with transport-owner check and active read-back | Port as a distinct write capability; keep the mutation fail-closed. |
| ENHO/XH, class enhancement, BAdI creation | No creation path found | Explicit metadata and SAP framework bridge; repository verification after creation | Port without guessing host/anchor/spot; validate each subtype on non-production SAP before release claims. |
| Dynpro read | No general screen reader found | Read-only `GetDynpro` via `RPY_DYNPRO_READ` bridge and `GetSource` routing | Port read-only API, MCP route, bridge, and synthetic tests. Do not imply screen creation support. |
| INCL source operations | Present | Present with additional routing/tests | Prefer upstream behavior; retain only verified fork-specific cases. |
| Transportable object workflow | New upstream transport choice/reuse and source-hash guard | Earlier package/transport fixes and ENHO owner checks | Reconcile contracts; test `$TMP` and a transportable synthetic package with an explicit request. |
| Mock SAP development server | Not present | Local simulator and object-expansion test script | Retain as a development-only test fixture, updating request/response contracts to upstream. |
| CI and community releases | Upstream checks and GoReleaser target `oisee` | Fork checks, reviewed sync policy, community tags | Preserve fork ownership and reviewed synchronization; carry upstream `LICENSE`/`NOTICE` assets with any new RFC binary. |

## Release gates

1. The integrated tree builds and passes `go vet ./...` and unit tests. Record
   pre-existing platform-specific failures separately; do not label them as
   integration regressions without reproducing them on the upstream baseline.
2. Mock tests cover each retained fork-only route, including rejected writes,
   creation verification, and transport mismatch. No test may access a real
   customer system.
3. In a non-production SAP system, verify ENHO/XH update, ENHO/XH creation,
   class enhancement creation, BAdI creation, and package/transport assignment.
   Record the SAP release and any unsupported subtype explicitly.
4. Review the full diff, the embedded ABAP bridge, dependency licences, release
   destination, and a secret/customer-data scan before merging to community
   `main` or publishing a community binary.

Until those gates pass, the latest community binary remains on the older
upstream baseline. A source merge alone is not a verified release.
