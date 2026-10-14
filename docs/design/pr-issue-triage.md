# PR / issue triage — what to merge, what to build, what to close

Triage report, 2026-08-20. Covers every open item on `oisee/vibing-steampunk`
at `main` = `0ac7d64`: **19 pull requests** and **47 issues**. Every verdict
below was checked against the code at HEAD, the PR diff, or the issue thread —
where a claim could not be confirmed it is marked ⚠️ rather than asserted.

> **Reading key.** ✅ = confirmed by reading the code or the diff. ⚠️ = plausible
> but unverified, with the check that would settle it. ❌ = checked and false.
> Effort **S** = under a day, **M** = a few days, **L** = a sprint or more.
> 🤖 = an agent could implement this unattended (mechanical, well-tested,
> no live SAP system needed to be confident).

This document is read-only with respect to GitHub. Nothing here has been
commented, labelled, merged or closed.

---

## 0. The one finding that reframes everything else

**No pull request has been merged since 2026-04-12** (`#101`, four months ago).
All 19 open PRs are from outside contributors; none is a self-PR by `oisee`.
`gh pr view --json reviews,comments` shows **zero maintainer review comments,
review decisions or labels on any of the 19**. The community has been reviewing
each other's work in the maintainer's absence — `txape10`, `zooloo303`,
`frd1201` and `Augusto42` have between them independently reproduced, verified,
and in three cases found regressions in each other's patches.

Two structural facts explain the freeze, and both are cheap to fix:

| # | Fact | Evidence |
|---|---|---|
| F1 | **There is no PR CI.** `.github/workflows/` contains exactly one file, `release.yml`, triggered by `workflow_dispatch`. `gh pr checks <n>` returns "no checks reported" for all 19 PRs | ✅ `find .github -type f` → `copilot-instructions.md`, `workflows/release.yml` |
| F2 | **`go test ./...` is red on a clean checkout.** `pkg/ctxcomp/analyzer_test.go` (`TestAnalyzerLive`) and `pkg/ctxcomp/benchmark_live_test.go` (`TestBenchmarkLive`) dial a live SAP system and fail 401. Unlike `pkg/adt/integration_test.go` they carry **no** `//go:build integration` tag | ✅ ran `go test ./...` — 16 packages ok, `pkg/ctxcomp` FAIL; ✅ `head -8` on both files shows no build tag |

F2 is why F1 was never fixed: adding CI today paints the repo red on day one.
Fix F2 first, then F1, and the merge queue becomes a review problem instead of
a "boot a SAP system and check by hand" problem. This is the highest-leverage
hour available anywhere in this document.

---

## 1. What changed since the previous plan

The previous plan is `reports/2026-06-15-001-issue-pr-triage-and-roadmap.md`
(commit `83b9699`, plus a Russian twin `-002-…-ru.md`). It clustered the tracker
into 14 themes (A–N) and proposed a 3-sprint order. Its central judgement was
right and is repeated here:

> "The highest-leverage single action is a **review-and-merge pass** on the open
> PRs — most of the diagnosis work is already done by the community."

**None of it was executed.** `git log 83b9699..HEAD` is ten commits, and all ten
are the classic-RFC integration or its docs. Not one Sprint-1 item landed: no
PR merged, no `corrNr` forward, no activation-error port. The backlog it
described has since grown from 11 open PRs / 31 open issues to 19 / 47.

What genuinely moved:

| Previous plan said | Status now |
|---|---|
| Cluster A — merge #125, reconcile #108, implement corrNr-forward for #132/#135 | ❌ none merged. But **PR #145 now exists** and implements corrNr-forward properly (`Fixes #144`), and **PR #152** narrows the `NoModification` guard. The cluster is now fully covered by unmerged patches |
| Cluster B — pick #121 vs #139 for INCL | Unchanged, both still open. ✅ Confirmed #121 is a strict superset: it contains the same `/includes/` fix as #139 plus `WriteInclude` |
| Cluster C — port #136 (activation errors) | ❌ not ported. But **PR #148 now exists** covering the root-element half of #136 |
| Cluster D — "mandant bug NOT YET FILED" | ✅ **now filed as #146**, and **PR #128** is exactly its fix |
| Cluster D — merge #120 (CSRF) and #107 (proxy) | ❌ neither merged; both still clean and mergeable |
| Cluster G — RunReport/APC is an "architectural limit"; consider docs-closing #55/#113 | **Overtaken by events.** `docs/design/rfc-opportunities.md` §5 shows the APC restriction is bypassable over classic RFC and names the exact RFC-enabled FM chain. Do not docs-close these |
| Cluster I — merge #130 (ENHO read) | **Reverse this.** #130 has since been shown to carry undisclosed regressions (§3) |
| Cluster J — "#138 is important", merge with #106 | **Split the verdict.** #106 is a clean fix and should merge; #138 is the same fix buried in 110 files and should be closed (§3) |
| Cluster M — anonymization/column masking, "file an issue" | ❌ ✅ never filed — `gh issue list --search "mask"` and `"anonym"` both return `[]`. Still the only enterprise-gating item in the tracker, and still invisible |

**The new thing that did not exist in June:** classic RFC shipped in v2.40.0
(`d4e51ea`, `2f79046`, `c2f0fb9`) — `vsp rfc` CLI, `SAP(action="rfc")`, and
`pkg/saprfc`, on the sibling library `open-rfc-go`. Its consequences for the
tracker are in §5.

### 1.1 The RFC design doc is already partly stale

`docs/design/rfc-opportunities.md` (`0ac7d64`, written today) opens with four
structural limits L1–L4. Three of them were fixed hours later by `9528ba1`
("WHERE splitting, wide-table fallback, one shared ReadTable"):

| Limit | Status | Evidence |
|---|---|---|
| L1 — MCP handler opens **and closes** a client per call; no stateful conversation expressible | ❗ **still open** | ✅ `internal/mcp/handlers_rfc.go:55` is still `defer c.Close(ctx)`; `grep -rn 'Session\|Pool\|Callbacks' pkg/saprfc/ internal/mcp/handlers_rfc.go cmd/vsp/rfc.go` returns **nothing** |
| L2 — WHERE truncates silently at 72 chars | ✅ **fixed** | `pkg/saprfc/readtable.go` `splitWhereClause()` splits on token boundaries and errors rather than truncating |
| L3 — only the 512-byte `DATA` table is read | ✅ **fixed** | `ReadTable()` retries with `USE_ET_DATA_4_RETURN='X'` on `DATA_BUFFER_EXCEEDED` |
| L4 — `rfcReadTable` duplicated in CLI and MCP | ✅ **fixed** | both call `saprfc.ReadTable` (`cmd/vsp/rfc.go:156`, `internal/mcp/handlers_rfc.go:115`) |

Still genuinely missing from `pkg/saprfc/readtable.go`: `ROWSKIPS` (server-side
paging) and `NO_DATA` (schema-only) — ✅ `grep -rn 'ROWSKIPS\|NO_DATA' pkg/ cmd/ internal/`
returns nothing.

### 1.2 Upstream gap G1 is closed; vsp has not picked it up

The RFC doc lists G1 — "an exported sticky session" — as "the single most
valuable thing open-rfc-go could add for vsp." **It has since been added**:
`open-rfc-go` shipped `rfc/session.go` with `Session.Call`, `Session.DescribeTool`,
`Session.Ping` and `Session.Close` (`3ba4638`, `4868782`). vsp's `go.mod` already
pins a commit that contains it (`v0.0.0-20260820210256-6b0fd4a541ec` = `6b0fd4a`)
and uses none of it. §6 of the RFC doc — the debugger over RFC — is no longer
library-blocked.

vsp is also **two commits behind** on that dependency, and one of them is a real
correctness fix:

> `7fc4b9e fix(metadata): RFC_FIELDS POSITION is not a dense 1..n sequence` —
> "A structure that includes another one reports the included component's own
> POSITION, so the rows can repeat or skip values; requiring position == index+1
> rejected real DDIC structures."

Before that fix, `DecodeRfcStructureDefinitionResult` returned a hard error for
any FM whose parameters are typed by a structure containing an `.INCLUDE` or an
append — which is a large fraction of real BAPIs. On the pinned version,
`vsp rfc describe`/`call` fails on those FMs. A one-line `go.mod` bump fixes it.

---

## 2. Inventory — the 19 open pull requests

Every PR is from an outside contributor. "CI" is omitted from the table because
the answer is the same for all 19: none, per F1.

| PR | Title | Author | Files / +/− | Merge state | Closes | Verdict |
|---|---|---|---|---|---|---|
| #157 | CGO-free SQLite in release binaries | Augusto42 | 10 / +265 / −55 | ❗ CONFLICTING | — | merge after rebase |
| #156 | Harden write safety, ADT sessions, verification | Augusto42 | 44 / +2520 / −553 | draft | — | split into focused PRs |
| #155 | IAM authorization chain + SSO transport owner | Aagaard89 | 11 / +1572 / −32 | unknown | — | split: fix now, feature later |
| #152 | `NoModification` alone must not fail a MODIFY lock | Edgars-Ralfs-Dunis | 2 / +70 / −3 | clean | — | **merge as-is** |
| #150 | `ActivateMultiple` batch activation | txape10 | 5 / +176 / −10 | clean | #137 | merge after changes |
| #149 | SRVB read unblock + inverted doc fix | lin2qwer1-cloud | 4 / +100 / −6 | clean | — | **merge as-is** |
| #148 | Parse activation response root element | Edgars-Ralfs-Dunis | 2 / +62 / −10 | clean | — | merge after changes |
| #145 | Reuse an object's open transport on write | zooloo303 | 6 / +194 / −7 | clean | #144 | merge after changes |
| #139 | Program includes are not class includes | enricoandreoli | 3 / +20 / −4 | clean | — | **close** (subsumed by #121) |
| #138 | InstallZADTVSP deploys real source | blicksten | **110 / +23815 / −88** | unknown | — | **close** |
| #130 | ENHO (Enhancement Framework) read support | barkow15 | 23 / +2726 / −114 | ❗ CONFLICTING | — | **close** / reimplement |
| #128 | `sap-client` during browser authentication | andreasmuenster | 3 / +18 / −4 | clean | — | **merge as-is** |
| #126 | Server-side search type filter | frd1201 | 5 / +253 / −5 | clean | #119 | **merge as-is** |
| #125 | Skip redundant mutation gate after lock | dme007 | 8 / +361 / −14 | clean | — | merge (2nd in train) |
| #121 | INCL (PROG/I) write support | frd1201 | 10 / +204 / −43 | clean | #116 | merge after changes |
| #120 | CSRF HEAD→GET fallback + cookie + session type | frd1201 | 4 / +82 / −7 | clean | #104 | **merge as-is** |
| #108 | Deploy session ordering + `MODIFICATION_SUPPORT` | dme007 | 7 / +467 / −108 | clean | — | merge after changes (3rd) |
| #107 | Honor `HTTP_PROXY` for WebSocket connections | dme007 | 2 / +99 / −9 | clean | — | **merge as-is** |
| #106 | Install: propagate Description, detect package | dme007 | 3 / +77 / −18 | clean | ⚠️ **#2** | merge after changes |

**Six PRs are clean, tight, self-contained and ready today**: #128, #126, #120,
#107, #149, #152.

---

## 3. Per-PR verdicts

### Merge as-is

**#128 — `sap-client` during browser authentication.** ✅ Threads a `sapClient`
parameter into `BrowserLogin` and appends `sap-client=<client>` to the ADT
discovery URL before the browser opens, so the SSO cookie scopes to the right
client instead of the default. 3 files, +18/−4. This is literally the fix for
issue **#146**, and it is also the "mandant bug" the June plan flagged as
unfiled. ✅ Confirmed the PR touches only `cmd/vsp/main.go`,
`pkg/adt/browser_auth.go`, `pkg/adt/browser_auth_test.go`. Ask the author to add
`Closes #146` so the issue closes with it.

**#126 — server-side search type filter.** ✅ Adds
`Client.SearchObjectByType` so `--type` is applied by the ADT
`informationsystem/search` endpoint *before* `maxResults` truncates, instead of
client-side afterwards. Adds a `CanonicalObjectType` mapping (`CLAS`→`CLAS/OC`,
`INCL`→`PROG/I`, …). Author already closed an MCP-path gap that `txape10` found
in review. `Closes #119`.

**#120 — CSRF HEAD→GET fallback.** ✅ I read the diff directly:
`fetchCSRFToken` keeps HEAD, and falls back to a GET with `X-CSRF-Token: fetch`
only when the HEAD response carries no usable token **and** the status is not
401/403. That 401/403 guard is commit `59b401b2`, added in response to a
regression `txape10` found in review (the unguarded fallback broke
`TestTransport_Request_RetryOn401_ReauthFails`). Also strips the `Secure` cookie
flag over plain HTTP and adds `SAP_SESSION_TYPE`. `Closes #104`.

> Correction for the record: an earlier reading of this cluster attributed the
> CSRF fallback to #128 and claimed it had been reverted. ✅ False — #128 does
> not touch `pkg/adt/http.go` at all, and #120's fallback is present and
> refined, not reverted.

**#107 — `HTTP_PROXY` for WebSocket.** ✅ 2 files. Adds `newZADTVSPDialer` and
`newPreAuthHTTPClient`, both setting `Proxy: http.ProxyFromEnvironment`, and
routes `BaseWebSocketClient.Connect` through them. Mirrors the already-accepted
#13 pattern for the HTTP path. Zero scope creep, two new tests.

**#149 — SRVB read unblock.** ✅ Adds `"SRVB"` to the `routeSourceAction` read
allow-list so the *already implemented* `GetSource` path becomes reachable from
MCP, and corrects `binding_category` docs that were inverted (SAP domain
`SRVB_BND_CATEGORY` is 0=UI, 1=A2X/Web API — the code said the opposite).
Docs half is behaviour-free.

**#152 — `NoModification` must not fail a lock that has a handle.** ✅ The
sharpest patch in the tracker: `pkg/adt/crud.go` `LockObject` currently rejects
any MODIFY lock whose response says `MODIFICATION_SUPPORT="NoModification"`.
That guard came from `22517d4` and is correct on BTP, where class roots really
are read-only. On NetWeaver on-prem a CLAS lock reports `NoModification` **and
returns a working handle**, so the guard broke all on-prem class edits. The fix
adds `&& result.LockHandle == ""` to the reject condition. 2 files, +70/−3, with
a test pinning the on-prem case and a live verification table in the body. This
is the single highest value-per-line patch open.

### Merge after specific changes

**#106 — install fixes.** ⚠️ **Do not merge as-is: it will close the wrong
issue.** ✅ `gh pr view 106 --json closingIssuesReferences` returns issue **#2**
— "gui debugger". Merging auto-closes the project's flagship strategic issue.
Required change: get the linkage out of the body (or reopen #2 immediately
after). The code itself is good and correctly scoped — passes `obj.Description`
so `writeSourceCreate` stops silently failing, checks `res.Success` instead of
only `err`, and adds `Client.PackageExists` so a rerun stops falling into
`reconcileFailedCreate`'s LOCK+DELETE path against a pre-existing package. The
body honestly scopes out the deeper `reconcileFailedCreate` bug as follow-up.

**#145 — reuse an object's open transport.** Good design: one helper,
`resolveWriteTransport(supplied, lockCorrNr, opName)`, which prefers the
caller's transport, else falls back to `LockResult.CorrNr`, and critically
**re-runs `checkTransportableEdit` on the fallback** so it cannot bypass
`--allow-transportable-edits`. This is the corrNr-forward the June plan asked
for. `Fixes #144`, and ✅ #135 is a duplicate of #144 so it closes too. Required
change: the author states plainly that the commit-2 paths
(`writeClassMethodUpdate`, `writeSourceUpdate`, `UpdateFromFile`) are **not**
live-verified — ask for a verification run, or merge and label those paths
experimental.

**#148 — activation response root element.** ✅ Real and well-diagnosed:
`parseActivationResult` declared `Messages`/`Inactive` as child fields, but SAP
returns `<chkl:messages>` / `<ioc:inactiveObjects>` as the **root**, so unmarshal
matched nothing and `Success` stayed `true` through real activation failures —
across ~20 call sites. Required change: `txape10` notes in review that this is a
partial fix — it does not handle namespace-prefixed attributes (`adtcomp:type="E"`)
or the legacy `<activationLog>` root, which is the other half of issue **#136**.
Ask for `Closes #136` plus those two cases, or merge and keep #136 open for them.

**#150 — `ActivateMultiple`.** ✅ Adds `Client.ActivateMultiple` posting one
batch `<adtcore:objectReferences>` to `/sap/bc/adt/activation`, rewrites
`ActivatePackage` on top of it, adds `ResolveObjectRef`. `Closes #137`.
Deliberately avoids overlapping #126. Required change: it is built on the
activation-result parser that #148/#136 prove is broken — batch activation that
cannot detect failure is worse than single activation that cannot. **Sequence it
after #148.**

**#121 — INCL write support.** ✅ Confirmed a strict superset of #139: it carries
the same `/includes/` fix in `normalizeObjectURLForPackageCheck` and
`SyntaxCheck`, plus `Client.WriteInclude`, `.incl.abap` parsing, and CLI/MCP
wiring. `Closes #116`. Required change: it also reorders `UpdateFromFile` to run
SyntaxCheck before Lock — the same change #108 makes. Whichever lands second
needs a rebase; pick one owner for that hunk.

**#157 — CGO-free SQLite.** Coherent single theme: swap `mattn/go-sqlite3` for
`modernc.org/sqlite`, add a `ci.yml`, bump actions and CVE-driven deps.
Required change: ✅ it is `CONFLICTING`/`DIRTY` and needs a real rebase. Note
its `ci.yml` overlaps the CI work in §6 item 1 — decide which lands first so
they do not fight. Worth doing either way: a CGO-free build is what makes the
9-platform `make build-all` honest.

### Split, don't merge

**#155 — IAM authorization chain.** 11 files, +1572. Two genuinely different
things in one PR: (a) a small, clearly correct bug fix — resolve `CurrentUser`
from `GET /sap/bc/adt/cts/transportrequests` instead of `config.Username`, which
fixes the empty transport owner under browser SSO; and (b) a sizeable new
feature — three new ADT object types (SIA1/SIA6/SIA7), a `vsp adt-get` raw-read
command, async publish-job handling. The work is well-researched with live
verification notes, not slop. Ask for (a) as its own PR — it pairs naturally
with #128 in the SSO cluster — and review (b) on its own merits afterwards.

**#156 — harden write safety (draft).** 44 files, +2520, 15 commits, ten
self-described "defect groups", and the body itself says it "overlaps with the
intent of #106, #108, #120, #125, and #138". Sampled code is real and specific —
`resolveCLISafety()` merging per-system `.vsp.json` policy with flags and env is
a genuine improvement — but as one PR it is unreviewable and collides with five
others. It also predates the RFC merge and needs a rebase. Reply kindly: pick
the two or three groups that do **not** overlap the merge train and open them
separately.

### Close

**#139 — program includes vs class includes.** ✅ Correct, and the smallest diff
in the tracker (3 files, +20/−4). Close it anyway — **#121 contains the same fix**
and also delivers the `WriteInclude` feature the reporters actually want, and
`txape10` already flagged the duplication on both threads. Close kindly, credit
the independent diagnosis, and if #121 stalls, revive #139 as the minimal fix
instead. (If speed matters more than features, invert this: merge #139 today and
strip the duplicate hunk from #121.)

**#138 — InstallZADTVSP deploys real source.** ❗ **110 files, +23815/−88, 63
commits.** `gh pr diff 138` refuses to render it ("HTTP 406: diff exceeded max
20000 lines"). The described fix is roughly ten lines. Around it the branch
carries an entire `.claude/` agent framework (41 files), a full SAML SSO
subsystem, an impact/refactoring/regression analysis engine, an "Intelligence
Layer" MCP handler set, and eight planning documents. This is the same author
and the same shape as #82/#84/#85/#86/#89, all previously closed. Close with the
standard note — and point at **#106, which fixes the same install defect
correctly in 3 files**. Nothing of value is lost.

**#130 — ENHO read support.** Close, or reimplement the good part. The
`enhancements.go` additions are fine in isolation, but ✅ the diff also makes two
undisclosed regressions, independently confirmed by `Augusto42` running the full
suite downstream:

1. `zcl_vsp_apc_handler.clas.abap`'s `class_constructor` **unconditionally
   comments out** the debug, AMDP, git and report service registrations from
   `gt_services` — justified only by "not available on this classic-ECC system",
   with no capability gate. Every ZADT_VSP user loses four service domains.
2. `zcl_vsp_rfc_service.clas.abap`'s `handle_move_to_package` — a working
   implementation — is deleted and replaced with a hardcoded `NOT_AVAILABLE`
   stub, for the same reason.

It is also `CONFLICTING`. **Reimplement ourselves:** take `pkg/adt/enhancements.go`
+ its test and the POSIX-regex fixes for old ECC, drop all seven embedded ABAP
class edits, and add a proper runtime capability probe instead of commenting
registrations out — if `ZADT_CL_TADIR_MOVE` is absent, the service should report
unavailable at call time, not be deleted at compile time. Credit `barkow15` for
the ENHO work and `Augusto42` for catching the regressions. ~M.

---

## 4. Issue verdicts

47 open issues. Grouped; every "still present" was confirmed by reading the
named file at HEAD.

### 4.1 Already fixed — close with a note (4)

| # | Verdict |
|---|---|
| **#88**, **#92**, **#98** | ✅ Fixed by `22517d4` (missing `Stateful:true` on `CreateTestInclude`/`UpdateClassInclude`). Ask the reporters to confirm on a current build, then close |
| **#105** | ✅ Fixed at HEAD but **not** in the v2.38.1 the reporter tested: `packageExists` (`pkg/adt/crud.go:537-552`) was rewritten in `81416d37` (2026-04-09) to be permissive on API errors; `git merge-base --is-ancestor 81416d37 v2.38.1` → false. Close with an upgrade note |

### 4.2 Not applicable (2)

**#45**, **#46** — both ask to enhance `scripts/sync-upstream.sh`. ✅ That script
does not exist in this repo and never has; a commenter already said so. Close
kindly. The June plan listed these as "low effort, batch when convenient" — the
real effort is zero because there is nothing to change.

### 4.3 Fixed by a PR that is sitting unmerged (7)

These need no new code — only the merge train in §6.

| # | Fixed by | Confirmed root cause |
|---|---|---|
| **#104** CSRF 403, HEAD unsupported | PR **#120** | ✅ `pkg/adt/http.go` `fetchCSRFToken` hardcodes HEAD |
| **#116** WriteSource has no INCL | PR **#121** | ✅ `pkg/adt/workflows_source.go:218-224` switch excludes INCL; the error string in the issue is a byte-for-byte match. Read side already supports it |
| **#119** `vsp search --type` drops results | PR **#126** | ⚠️ not independently traced; #126's server-side-filter analysis is convincing and the maintainer's own `--verbose` request was answered |
| **#137** `ActivateMultiple` | PR **#150** | ✅ no `ActivateMultiple` anywhere in the tree |
| **#144**, **#135** transport 409 / `corrNr not found` | PR **#145** | ✅ `LockResult.CorrNr` (`pkg/adt/crud.go:16-24`) is never threaded into `UpdateSource` (`crud.go:137,151-152`) or six sibling call sites. #135 is a duplicate of #144 |
| **#146** `sap-client` missing under SSO | PR **#128** | ✅ `sap-client` is set only in the normal request builder (`pkg/adt/http.go:357`); `pkg/adt/browser_auth.go` never sets it |

### 4.4 The lock-handle cluster — one merge order settles it (4)

**#91** (partial — BTP half fixed by `22517d4`, on-prem half re-broken by the
same commit), **#110**, **#132**, **#141**. ✅ Two distinct mechanisms remain
live at HEAD, and each has its own patch:

- the `NoModification` guard at `pkg/adt/crud.go:67-75` rejecting locks that
  carry a working handle → **PR #152**;
- the mutation gate's `getObjectPackage` making a **stateless** `SearchObject`
  hop between the stateful Lock and the stateful write, letting ICM retire the
  session → **PR #125** (`pkg/adt/mutation_gate.go:94-118`);
- SyntaxCheck ordered after Lock in `CreateFromFile`/`UpdateFromFile`
  (`pkg/adt/workflows_deploy.go:81,103,228,251`) → **PR #108**.

**#141** is worth flagging separately: it reports the guard firing "over
ADT-over-RFC on NW 75x". That is the *old* ZADT_VSP bridge, not the new classic
RFC leg — #152 fixes it either way, but do not let the word "RFC" route it to
the wrong subsystem. **#110** ⚠️ is the one genuine unknown: it was filed *after*
`22517d4`, so it is not that bug; it most likely matches #125 or #108 but the
reporter's config is needed to say which. Ask before closing.

### 4.5 Message classes — a four-link chain, all still present (4)

All four filed by `erovneiko` on 2026-08-19. I verified every one directly.
They are a dependency chain, not duplicates, and together they are the single
best-defined small work item in the tracker.

| # | Confirmed defect |
|---|---|
| **#162** | ✅ `internal/mcp/handlers_source.go` — the read switch lists `"MSAG"`; the edit switch immediately below is `"CLAS", "PROG", "INTF", "DDLS", "BDEF", "SRVD"`. MSAG is rejected before reaching any handler. **Gates the other three** |
| **#159** | ✅ `internal/mcp/tools_register.go` registers `WriteMessageClassTexts` with `name`, `language`, `lock_handle`, `transport` — and no `texts`. The handler (`internal/mcp/handlers_i18n.go:87,105-107`) reads `["texts"]` and errors "texts is required", because the MCP SDK drops undeclared arguments. The tool is literally uncallable |
| **#160** | ✅ `MessageClass` (`pkg/adt/client.go:775-784`) has no `XMLName`, so `xml.Marshal(mc)` in `pkg/adt/i18n.go:143` emits a bare `<MessageClass>` root — the Go type name — instead of ADT's namespaced element. The same literal leaves `Description` unset, exactly as reported |
| **#161** | ✅ Genuine capability gap: no delete collection on the struct, no delete path in `WriteMessageClassTexts` |

Fix order is forced: #159 + #162 make the tool reachable, #160 makes it correct,
#161 is a feature on top. RFC is irrelevant here — MSAG is a source object and
ADT locking must stay authoritative (`rfc-opportunities.md` §12).

### 4.6 Small confirmed defects, each a contained fix (9)

| # | Confirmed at HEAD | Effort |
|---|---|---|
| **#117** `--allow-transportable-edits` rejected by subcommands | ✅ `cmd/vsp/main.go:134` uses `rootCmd.Flags()`, not `PersistentFlags()`, so subcommands with their own FlagSet never inherit it. ✅ **Generalizes** — `--allowed-packages`, `--allowed-ops`, `--allowed-transports`, `--enable-transports` are all registered the same way, so the whole safety-flag family is CLI-invisible on write subcommands | S 🤖 |
| **#111** `get_user_transports` empty | ✅ `GetUserTransports` (`pkg/adt/transport.go:69-86`) lacks the `listTransportsViaSQL` fallback that `ListTransports` (`transport.go:459`) has | S 🤖 |
| **#140** `ListTransports` wildcard `*` | ✅ `transport.go:470` builds `WHERE e070~AS4USER = '<user>'` with exact equality; `*` is never mapped to `LIKE '%'` or dropped | S 🤖 |
| **#123** `GetRevisions` 404 for INTF/DDLS | ✅ `pkg/adt/revisions.go:138,151` build exactly the URLs the issue shows 404-ing. ⚠️ the correct alternative path shape is unknown — needs a live probe | S–M |
| **#142** `GetCallGraph` 415 | ✅ `pkg/adt/client.go:1324,1355` sends `application/xml`; SAP's own error names `application/vnd.sap.adt.cai.callgraphconfig.v1+xml`. Also affects `GetCallersOf`/`GetCalleesOf`/`AnalyzeCallGraph` | S 🤖 |
| **#147** object explorer 404 on NW 750 | ✅ `GetObjectStructureCAI` (`pkg/adt/client.go:1717`) is called with no fallback to the older `GetClassObjectStructure` (`client.go:346`) that the issue itself names | S 🤖 |
| **#153** `GetTypeInfo` 406 for DTEL | ✅ `pkg/adt/client.go:1136-1141` sends only `Accept: application/xml`; `GetFunctionGroup` got a 3-tier fallback in `edd94bc2` and this call site never did | S 🤖 |
| **#118** (part 1) `EDITSOURCE` needs lowercase `object_url` | ✅ `internal/mcp/handlers_fileio.go:198` passes `objectURL` through with no normalization. ⚠️ part 2 (oversized-JSON silent failure) not verified | S 🤖 |
| **#131** global warnings opt-out | ✅ Only the per-call `ignore_warnings` exists (`handlers_fileio.go:235,247`); no flag/env/config equivalent. `txape10` has a fork implementation | S 🤖 |

### 4.7 Confirmed but larger (4)

- **#136** activation/syntax errors silently swallowed. ✅ `parseActivationResult`
  (`pkg/adt/devtools.go:182-236`) never strips namespace prefixes, so
  `adtcomp:type="E"` never matches `xml:"type,attr"`, `m.Type` stays `""`, and
  `strings.ContainsAny("", "EAX")` is false — `Success` stays `true` on real
  failures. `parseSyntaxCheckResults` (`devtools.go:70-74`) has a
  `strings.ReplaceAll(xmlStr, "chkrun:", "")` hack that the issue correctly calls
  broken (it turns `xmlns:chkrun=` into a default-namespace declaration); the same
  hack recurs at `pkg/adt/transport.go:332` and `pkg/adt/testing.go:230`.
  **This is the worst correctness bug in the tracker** — the server reports
  success on objects SAP rejected. PR #148 covers the root-element half; a proper
  `stripXMLNamespaces` helper covers the rest. M.
- **#143** `WriteSource` demands `package` on updates. ✅ The top-level mutation
  gate (`pkg/adt/workflows_source.go:184,200`) runs before the existence check
  (~`:226`) that could self-resolve the package, so it fails closed. M.
- **#154** `GetFunctionGroup` 406 for namespaced groups. ⚠️ Partially fixed —
  the 3-tier Accept fallback exists (`client.go:430-441`); the reporter's
  namespaced case still fails, so something about the leading-slash form is
  unsatisfied. Needs a live probe against a namespaced group. M.
- **#114** HANA detection on S/4HANA 758. ❌ **The reported causal chain is
  wrong** — `probeHANA` (`pkg/adt/features.go:231-242`) → `GetSystemInfo`
  (`client.go:1213-1239`) does its S4CORE fallback via `RunQuery` on `CVERS`, not
  via `/sap/bc/adt/system/components`. ✅ But a real adjacent bug exists:
  `GetInstalledComponents` (`client.go:1263-1268`) sends `application/xml` where
  `application/atom+xml;type=feed` is required. Reply explaining the split, fix
  the Accept header, and ask for a fresh trace on the detection half. S–M.

### 4.8 Feature requests, no defect (7)

**#27** AFF/NROB object types (label `enhancement`) · **#74** CDS metadata
extensions DDLX/EX — ✅ absent from `workflows_source.go`'s switch and from
`CreatableObjectType` · **#94** ZADT_VSP custom API for IMG config · **#99**
OAuth2 for BTP — ✅ `rg -i oauth` across the tree is empty · **#109** create
Domains/Data Elements — ✅ `pkg/adt/crud.go:180-193` `CreatableObjectType` has no
`DOMA`/`DTEL` · **#112** reload cookies after external refresh — ✅ loaded once
at `pkg/adt/config.go:97-101` from a one-time read at `cmd/vsp/main.go:645`, no
watch or reload-on-401 · **#158** read-only DYNP support (a well-formed proposal
from `Augusto42`).

**#90** BTP Basic Auth 401 — ⚠️ plausible (a redirect stripping `Authorization`;
`CheckRedirect` is set at `pkg/adt/config.go:251`) but not confirmed. Needs a
direct read of the redirect handler before it can be called a defect.

### 4.9 Strategic / architectural (3)

**#2** GUI debugger · **#55** and **#113** RunReport in APC · **#103** SAProuter.
All four are covered in §5.

---

## 5. What the RFC leg changes

### Obsolete — close and redirect (1)

**#151 — `CallRFC` fails HTTP 400 for FMs with non-elementary table parameters.**
✅ Root cause confirmed: the old bridge's signature is
`func (c *DebugWebSocketClient) CallRFC(ctx, function string, params map[string]string)`
(`pkg/adt/websocket_rfc.go:20-58`). Every parameter is forced through a flat
string map before reaching the ABAP-side dynamic `CALL FUNCTION`, so a table
whose line type is a structure cannot be represented at all — which is exactly
the `FIXED_VALUES` type error the reporter sees on `DDIF_FIELDINFO_GET`. It is a
structural limit of that bridge, not a patchable bug.

The new leg has no such constraint: `open-rfc-go` does typed, metadata-driven
decode of arbitrary parameter shapes including deep structures and nested tables.
Reply on #151 with the replacement call and close:

```
SAP(action="rfc", params={"op":"call","fm":"DDIF_FIELDINFO_GET",
                          "args":{"TABNAME":"BSEG"}})
```

Then deprecate the legacy `CallRFC` tool rather than leaving two RFC paths that
behave differently. **Caveat:** ✅ do the `go.mod` bump from §1.2 first —
`DDIF_FIELDINFO_GET` returns `DFIES` rows, and structures with `.INCLUDE` are
precisely what `7fc4b9e` fixes. Verify before closing.

### Newly solvable, not yet solved

**#55 and #113 — RunReport in APC.** ✅ Not duplicates: #113 is the sync `SUBMIT`
timing out, #55 is spool reading being blocked; both trace to
`APC_ILLEGAL_STATEMENT` forbidding `SUBMIT`, `COMMIT WORK AND WAIT` and
`CALL FUNCTION … DESTINATION` inside an APC handler. `rfc-opportunities.md` §5
establishes that none of that applies over RFC and names the whole chain, all
verified `FMODE='R'` on the test system: `BAPI_XMI_LOGON` →
`SUBST_START_REPORT_IN_BATCH` → poll `BAPI_XBP_JOBLIST_STATUS_GET` →
`BAPI_XBP_JOB_SPOOLLIST_READ` → `BAPI_XMI_LOGOFF`. ✅ Nothing is implemented:
`pkg/saprfc/` contains only `saprfc.go` and `readtable.go`, and
`rg "SUBST_START_REPORT|BAPI_XBP|BAPI_XMI"` across `pkg/`, `internal/`, `cmd/`
returns nothing. **This retires the "architectural limit" label these two issues
have carried since March, and un-disables an entire experimental tool group.** M.

**#2 — GUI debugger.** The ADR-documented blocker
(`docs/adr/001-websocket-stateful-debugging.md`) is session affinity, which a
classic RFC connection provides for free. Two things changed since the RFC doc
was written: ✅ gap G1 is closed upstream (`rfc/session.go`), and ✅ vsp already
pins a version containing it. What remains is vsp-side (L1: stop closing the
client per call) plus the `Z_VSP_DBG_CALL` ABAP facade and the
`Z_VSP_PROBE_STATE` stickiness experiment from §6.1. Still **L**, but no longer
blocked on anyone else. Restoring even breakpoints alone brings back part of the
16 tools in `DefaultDisabledTools()` (`pkg/config/systems.go:271-285`).

**#158 — read-only DYNP.** The proposal reads screens via `RPY_DYNPRO_READ`
through the *old* ZADT_VSP bridge. ✅ It would work as written (that FM's outputs
are elementary, so #151's defect does not bite), but implementing it over
`pkg/saprfc` instead drops the ZADT_VSP and WebSocket dependencies entirely —
consistent with §9's RFC-only read list. Redirect the author before they build
on the deprecated path. S–M.

**#94** (read half) and **#140** (read half) could both be answered over
`RFC_READ_TABLE`. For #140 that is not worth it — the ADT wildcard bug is a
two-line fix. For #94's IMG/customizing tables it genuinely is the better path.

### Explicitly *not* solved by RFC

**#103 — SAProuter.** ✅ No SAProuter/SNC code in vsp, and none in open-rfc-go
either. SAProuter is an NI-protocol concern orthogonal to the gateway dial, and a
pure-Go SAProuter is a separate unstarted track. Say so on the thread rather than
leaving it looking adjacent to the RFC news.

**#109 — create Domains/Data Elements.** `RPY_DOMAIN_INSERT` and friends exist
and are tempting. `rfc-opportunities.md` §12 rejects them, correctly: they bypass
the ADT lock protocol, the syntax check and the activation queue. Implement as a
proper ADT `CreateObject` addition. Same reasoning for #94's write half.

---

## 6. The ranked plan

### Do next — small, high value

| # | Item | Effort | Agent-safe |
|---|---|---|---|
| 1 | **Make the repo reviewable**: tag the two live `pkg/ctxcomp` tests `//go:build integration`, then add `.github/workflows/ci.yml` (build + `go vet` + `go test ./...` on `pull_request`). Fixes F2 then F1 | S | 🤖 |
| 2 | **Bump `open-rfc-go`** to pick up `7fc4b9e` (§1.2) — a `go.mod`/`go.sum` one-liner that unbreaks `vsp rfc describe`/`call` for any FM using `.INCLUDE`d structures | S | 🤖 |
| 3 | **The merge train** (§6.1) — six clean PRs as-is, four after named changes. Closes 7 issues without writing code | M | ✗ |
| 4 | **Message-class chain** #162 → #159 → #160 | S | 🤖 |
| 5 | **#117** `Flags()` → `PersistentFlags()` for the whole safety-flag family | S | 🤖 |
| 6 | **Close the free ones**: #88/#92/#98 (fixed), #105 (fixed since v2.38.1), #45/#46 (N/A), #151 (redirect to `action="rfc"`), #135 (dup of #144) | S | ✗ |
| 7 | **#111** + **#140** transport read gaps — one small pass over `pkg/adt/transport.go` | S | 🤖 |
| 8 | **#131** global `--ignore-warnings` / `SAP_IGNORE_WARNINGS` | S | 🤖 |

### Worth doing — medium

| # | Item | Effort | Agent-safe |
|---|---|---|---|
| 9 | **#136** — a real `stripXMLNamespaces` helper; retire the three `ReplaceAll("chkrun:")` hacks. The worst correctness bug open. Sequence **before** #150 | M | 🤖 |
| 10 | **Fix L1** — cache a per-system `*rfc.Client` instead of `defer c.Close(ctx)` at `internal/mcp/handlers_rfc.go:55`, and expose `Session` for multi-call flows. Every RFC item below depends on it | S–M | 🤖 |
| 11 | **Reports and jobs over RFC** (§5) → closes #55 and #113, un-disables the report tool group | M | ✗ |
| 12 | **abapGit export over RFC** — `rfc-opportunities.md` §2, live-proven, removes the ZADT_VSP + WebSocket dependency from `vsp export` | S | ✗ |
| 13 | **`vsp rfc probe`** + the ADT-down fallback hint (§3/§11); gives an alternate answer for #114 and #153 | S | 🤖 |
| 14 | **Accept-header pass**: #153, #114's `GetInstalledComponents`, #142's content type, #147's CAI fallback | S–M | 🤖 |
| 15 | **Reimplement #130's ENHO read support** without the service-registration regressions | M | ✗ |
| 16 | **#143** self-resolve package on update; **#118** normalize `object_url` case | M | 🤖 |
| 17 | **ROWSKIPS / NO_DATA** in `pkg/saprfc/readtable.go` (server-side paging, schema-only) | S | 🤖 |

### Later

| # | Item | Effort |
|---|---|---|
| 18 | **#155 split** — take the SSO transport-owner fix now, review IAM object types separately | M |
| 19 | **#157** CGO-free SQLite, after rebase and after item 1 settles who owns `ci.yml` | S |
| 20 | **#156** — ask for 2–3 non-overlapping slices | M |
| 21 | **#123** revisions, **#154** namespaced function groups — both need a live probe first | M |
| 22 | **#99** OAuth2 for BTP, **#90** BTP Basic Auth, **#112** cookie reload | M–L |
| 23 | **#109** DOMA/DTEL, **#74** DDLX, **#27** AFF/NROB — one "DDIC object types" push over ADT | L |
| 24 | **#158** DYNP over the new RFC leg | S–M |
| 25 | **#2** debugger over RFC — no longer library-blocked (§5) | L |
| 26 | **Column/schema masking** — still unfiled, still the only enterprise-gating requirement in the tracker. File the issue even if the design waits | L |

### Won't do

| Item | Why |
|---|---|
| **#138** merge | 110 files / +23815 for a ten-line fix; #106 does it correctly in 3 files |
| **#130** merge as-is | Silently disables four working ZADT_VSP service domains and stubs out a working handler; `CONFLICTING` |
| **#139** merge | Correct, but wholly contained in #121 |
| **#103** SAProuter | Orthogonal to RFC; belongs to a separate pure-Go SAProuter track |
| **#45**, **#46** | Target a script that is not in this repo |
| `RPY_*` writes over RFC (#109, #94-write) | Bypasses the ADT lock/syntax/activation protocol — `rfc-opportunities.md` §12 |
| AMDP debugger over RFC, `SXPG_COMMAND_EXECUTE`, per-FM MCP tools | §12, unchanged |

### 6.1 Merge order

The train is ordered by conflict, not by value — three pairs collide.

1. **#128, #126, #120, #107, #149** — independent, clean, no conflicts. Any order.
2. **#152** — the `NoModification` narrowing. Alone it recovers on-prem class edits.
3. **#125** — mutation-gate skip. ⚠️ `zooloo303` flagged it collides with #108 in
   `pkg/adt/workflows_deploy.go`; whichever is second needs a small rebase.
4. **#108** — rebased onto #125 and #152. ✅ Drop its own `NoModification` hunk,
   which #152 already covers more narrowly.
5. **#148**, then **#150** — batch activation must not land before activation
   failures are detectable.
6. **#106** — only after the accidental `#2` linkage is removed.
7. **#145**, then **#121** — #121 duplicates #108's SyntaxCheck-before-Lock
   reorder; assign that hunk one owner.

After this train: ✅ #104, #116, #119, #135, #137, #144, #146 close by merge, and
#88/#91/#92/#98/#110/#132/#141 can be re-tested and mostly closed.

---

## 7. The three things I would do first

### 1. Unfreeze review — gate the live tests, then add CI

`go test ./...` must be green before CI can exist, and CI must exist before 19
PRs can be judged on anything but reading.

- `pkg/ctxcomp/analyzer_test.go` — add `//go:build integration` (it contains
  `TestAnalyzerLive`, which calls `GetClassSource` against a real system)
- `pkg/ctxcomp/benchmark_live_test.go` — same (`TestBenchmarkLive` → `Search`)
- `.github/workflows/ci.yml` — **new**: `on: [pull_request, push]`, `go build ./...`,
  `go vet ./...`, `go test ./...`, and `golangci-lint` using the existing
  `.golangci.yml`
- `go.mod` / `go.sum` — fold in the `open-rfc-go` bump from §1.2 while here

Effort S, fully mechanical, 🤖. Everything else in this document gets cheaper
once it is done.

### 2. Run the merge train — six PRs as-is, today

No new code. In order: **#128, #126, #120, #107, #149** (independent), then
**#152**. That closes **#104**, **#119**, **#146** outright, fixes on-prem class
editing for every NetWeaver user, and — more valuable than any single patch —
tells nineteen waiting contributors that the queue moves. Then work the rest of
§6.1 as review time allows.

Requires judgement and a live system for spot checks; not agent-safe.

### 3. Make message classes writable — the best-defined small feature open

Four issues, one afternoon, all four root causes confirmed by reading:

- `internal/mcp/handlers_source.go` — add `"MSAG"` to the `action == "edit"`
  switch (it is already in the `action == "read"` switch one block above) → #162
- `internal/mcp/tools_register.go` — add the missing `texts` argument to the
  `WriteMessageClassTexts` registration; without it the MCP SDK drops the
  argument and the handler's "texts is required" fires every time → #159
- `pkg/adt/client.go` — give `MessageClass` an `XMLName` with the ADT namespace
  so `xml.Marshal` stops emitting a bare `<MessageClass>` root → #160
- `pkg/adt/i18n.go` — populate `Description` in the `MessageClass` literal at
  `:143` and send the corrected body → #160
- optional follow-up: a delete collection on the same struct and a delete path in
  `WriteMessageClassTexts` → #161

Effort S, 🤖 for the first four. ⚠️ The exact ADT namespace and root element name
for `PUT /sap/bc/adt/messageclass/<name>` should be confirmed against a live
system (or a captured Eclipse ADT request) before the #160 half is called done.
