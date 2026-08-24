# vsp agenda

The living board. One file, kept current — not a dated series. Dated analyses
live beside it as `agenda/YYYY-MM-DD-NNN-topic.md`.

Written for whoever picks the work up next, including the other agents working
on this repo from other machines. Last updated 2026-08-24 by **wsl-claude**, after two days of work that the
board had not caught up with.

> Sanitize policy applies here like anywhere else in the tree: no live
> hostnames, usernames, transport IDs or customer packages. Operational detail
> with real identifiers belongs under `.local/` or `.private/`.

---

## Where things stand — 2026-08-24

Three releases in two days: **v2.43.0** (debugger cassettes, post-mortem),
**v2.44.0** (AMDP fires, MCP parity), **v2.45.0** (AMDP with values, and a graph
that was inventing object names).

**The AMDP debugger works.** Over plain ADT, nothing installed on the server:
breakpoints fire, stepping, statement-level traces, variable values, the whole
scope at a stop, and the call stack with both the ABAP and the native line. This
project spent months concluding it was impossible, through a Z service and a
WebSocket protocol built to reach what the system was already offering. The one
thing missing is table *contents*; the address is right and HANA's own `INIT`
refuses it — state and next step are in `AMDPTableRows`.

**The debugger is tested without a system.** `vsp adt debug --record` takes a
cassette from a live run and the tests replay it, so `go test ./...` drives the
real debugger with no SAP. Cassettes are 7.58 only, by the naming rule.

**Ten features were found dead** — advertised and never working — plus one that
was worse. Two classes, and they are not the same:

- **Silence**: an error swallowed as an empty result. A dozen sites across CLI,
  MCP and graph. Three were wrong *numbers*, not missing caveats — a health
  report saying GOOD over a sweep that could not run, `SELF-CONSISTENT` over a
  transport holding nothing, `trace unit` exiting zero while saying nobody ran.
- **Invention**: `vsp graph callees` returned SHA-1 hashes **as the names of
  referenced objects**, because a name too long for `CHAR(120)` is stored hashed
  with the real one in `WBCROSSGTX`. Silence is a loss; invention is a
  corruption. Only this one produced answers that were confidently false.

Not one of them was visible by reading code. Each needed a live system.

### The method that found them

Worth more than the findings, and currently living only in reports:

1. **Ask the system, do not read the catalogue.** Discovery lies both ways — a
   resource absent from it answers 200, one present in it answers 400, and the
   dump resources are listed nowhere at all.
2. **Read the handler.** Five times out of five it answered in one request what
   inference did not answer in several. Sharper form: *when SAP does something
   in the kernel, look at what the same class reads from a table* — that is how
   `TMDIR` was found after `GET_METHOD_BY_INCLUDE` turned out to be a
   `SYSTEM-CALL`.
3. **Read what the system already sent.** The AMDP stop event carried the
   position, then the variables, then the call stack — three finds in one
   document that had been in hand since the first trace.
4. **Measure, do not reason.** Every time a rule was inferred from the examples
   to hand it was wrong about the first case nobody tried: `'FU'` in a `C(1)`
   column, a section-prefix list covering `U01` and missing `U27`.

## Needs a decision — new

**Turn the method into a command.** Ten dead features found by hand; the
eleventh will ship the same way unless the sweep is automated. `vsp compat`
already has the shape — checks, report, JSON, two-system diff. Extending it to
walk the whole advertised surface is days, not weeks, and it closes the class
permanently, on customer systems too. **This is the recommendation.**

It composes with the other session's work: they are documenting the ten
undocumented capabilities in a form a machine can check — not "shows callers"
but "answers non-empty for an object that has callers, and says the query ran
when it does not". Description without verification rots silently; verification
without description does not know what a right answer is. Together: documentation
that can fail.

**The mode gap.** The universal `SAP()` tool exists only in hyperfocused mode,
so agents in `focused` and `expert` cannot reach the seven post-mortem types or
the four AMDP targets at all. Same disease: a capability that looks present and
is not. A day, plus updating the pinned counts (1 / 101 / 146).

**Breaking changes in minor versions.** v2.45.0 changed `(*adt.Client).Callees`
and said so in its first paragraph rather than hiding it behind a compatible
wrapper — the wrapper would have kept the defect reachable under the old name.
There is no stated API stability promise; if one is wanted, now is the time.

## Needs a decision

**The audit of 2026-08-22** — [002-truthfulness-sprint](2026-08-22-002-truthfulness-sprint.md).
181 promises inventoried, 134 verified, 68 overstated or unverifiable. Tool
counts are corrected and pinned by a test; the rest is queued there, along with
three open decisions: connect or delete gCTS (884 orphaned lines), what to do
with `pkg/jseval`, `pkg/cache` and `pkg/ts2go` (no consumers), and
`vsp install abapgit` — half corrected: `abapgit-standalone.zip` is 836 KB and
real, `abapgit-full.zip` is still 0 bytes. And the question underneath has
changed shape (see below). Strategy recorded: debugger plus dynamic analysis, with a time-boxed
truthfulness pass first; open-abap-go parked with its reasoning kept.


**PR #152 — `fix/lock-nomodification-with-handle`.** The same fix we merged as
`583f042`, opened three weeks earlier by an outside contributor. It now
conflicts with `pkg/adt/crud.go` and `pkg/adt/crud_reconcile_test.go`. The
guard condition differs slightly: #152 fails only on `NoModification && no
handle`, ours fails on `no handle` alone (stricter). Ours additionally parses
the ADT exception body so an EU510 lock conflict reaches the caller with SAP's
own message. **Close as superseded with thanks, or adopt their narrower
condition?** Either way the contributor deserves an answer.

**Tool modes** — see [2026-08-22-001-tool-modes.md](2026-08-22-001-tool-modes.md).
Recommendation: parity test and typed per-action params first, deprecate
`focused`/`expert`, delete in the next major. Not yet decided.

**Eight unmerged branches on origin** — `one-tool-mode` (+16), `worktree-
integration-test-infra` (+8), `pr-93-fix` (+5), plus five small ones from
December–March. Needs triage: what is still alive after the rewrites since.

---

## Handover to wsl-claude — 2026-08-22, from claude-mac-m2

You are closer to the real work systems, so two things are yours:

**1. Take the `GetFunctionGroup` module-list fix** (details under Issue #154
below — read that first; the reported 406 is *not* the bug worth fixing).

Where: `pkg/adt/client.go`, `GetFunctionGroup`. It returns metadata and leaves
`Functions` nil, always. The module list lives behind the `objectstructure`
link on the group; `GetFunctionGroupAllSources` in the same file already walks
it and parses `abapsource:objectStructureElement` children, picking the ones
with `adtcore:type="FUGR/FF"` — reuse that rather than writing a second parser.
Each child carries `adtcore:name` and a `definitionIdentifier` link to its
`source/main`, which is enough to fill `FunctionModule` (`pkg/adt/xml.go:143`).

Watch for: the group endpoint answers 406 to `application/xml`, so keep the
vendor content types and their q-ordering — `...groups.v2+xml` also 406s on the
backend I tested. And a namespaced group works with `%2F`, `%2f`, or a
lowercased name; only raw slashes 404. Do not "fix" the encoding.

**2. Confirm on a real ERP 6.0 non-HANA system.** Everything above was measured
against an S/4-generation backend. The reporter's system is ERP 6.0, which is
exactly where the content types may differ, and it is the one thing I could not
check from here. If the vendor types behave differently there, that changes the
fix, not just the test.

Then #154 can be answered: the 406 is already fixed on main by `edd94bc`, which
landed five days after their build — ask them to retest — and the module list is
the real gap.

Nothing is held back on this machine: everything is merged and pushed, working
tree clean, no unpushed branches. `git pull` is enough.

Also waiting on you, unchanged: rebase `feat/function-module-edit` onto current
main before pushing it — see the next section.

---

## Cross-machine coordination

**`feat/function-module-edit` is on `origin` now** — the claim below that it
exists only on one machine is stale as of 2026-08-24. The ADT contract notes
under it are still worth keeping. Meanwhile `583f042` landed here and covers the *create*
path for function modules end to end. Before pushing:

1. `git fetch origin && git rebase origin/main` — main has moved by four merges
   (lock fix, function modules, host sanitization, browser SSO).
2. Read `pkg/adt/workflows_function.go` — the create path is done. What is
   **not** done is **editing an existing** function module as its own API,
   which is likely where that branch's value actually is.

What we learned about the ADT contract, so it does not get rediscovered:

- The remote-enabled flag is **ignored at creation**. `POST` with
  `fmodule:processingType="rfc"` returns `201` and the module reads back as
  `"normal"`. The flag only takes on a **metadata PUT under a lock**.
- A function module's **signature lives in its source**, not its metadata.
- **Activate after UNLOCK.** Activating while holding the lock returns 403
  EU510. This is true for classes too.

---

## Queued work

**Fast-RFC serializer captures** — the original goal everything else was
clearing the way for. The `ZCL_RFC_TEST` battery is written and active, no
locks outstanding. Next: bring the RFC relay back up, drive the battery through
the sniffing destination once per SM59 serializer mode (Classic / basXML /
Force basXML / Fast), capture one file per mode. The fast-path oracle is
already captured. Operational detail is in the private session notes.

**ZADT_VSP surface** — unblocked now that vsp can create RFC-enabled function
modules without SE37:

- move the enqueue-reset report out of `$Z` into `$ZADT_VSP`
- add the remote-enabled wrapper FM (logic in a class — `ENQUE_DELETE` is not
  remote-enabled, so the reset must live ABAP-side)
- delete the stray `Z_*` report that predates the naming convention
- wire the WS command through the APC handler

**Issue #154 — namespaced function groups return HTTP 406.** Investigated
2026-08-22 on a live system (claude-mac-m2). Two separate things:

*The reported 406 is already fixed on main.* It was the `Accept` header, and
the fix (`edd94bc`, 2026-04-12) landed five days after the build the reporter
is running (`a75fbfd`, 2026-04-07). Reproduced the exact error live by sending
the old header: `Accept: application/xml` → **406**, the vendor type
`...functions.groups.v3+xml` → **200**, on the same namespaced group.
Worth noting `...groups.v2+xml` also answers 406 there, so the q-ordering in
the current header is load-bearing, not decoration.

*URL encoding is not involved.* All three forms answer 200 —
`%2FUI5%2FCACHE_BUSTER`, lowercase `%2f...`, and a lowercased name. Only raw
slashes give 404. Our new `GetFunctionModule` / `CreateFunctionModule` were
checked against a namespaced group and are fine.

*But a different, real bug turned up.* `GetFunctionGroup` **never** returns the
function module list — `Functions` is `null` for a namespaced group and for a
plain one that certainly has modules. The metadata document simply does not
carry them; they hang off the `objectstructure` link, which
`GetFunctionGroupAllSources` already knows how to walk. So the reporter's
actual need — "list the modules in this group" — is still unmet even with the
406 gone, and the tool is described in the focused whitelist as "Metadata:
function module list".

**Next:** populate `Functions` from `objectstructure` in `GetFunctionGroup`,
then answer #154 saying the 406 is fixed, asking the reporter to retest on
current main, and pointing at the module-list fix. **A real ERP 6.0 non-HANA
system would be the better place to confirm** — the checks above ran against
an S/4-generation backend.

**`delete` through MCP needs a lock handle.** It is a two-step call that leaks
internal mechanics to the caller, unlike `create`, which now does the whole
flow. Same one-call treatment would suit it.

**`gofmt -l` reports 5 files** — `cmd/abapgit-pack/main.go`,
`fun/llvm2abap_demo.go`, `internal/lsp/server_test.go`,
`internal/mcp/handlers_context.go`, `internal/mcp/handlers_graph.go`.
Long-standing, unrelated to recent work.

---

## The installer question, reshaped

The board asked whether `vsp install abapgit` can be made to work. The shape of
the question changed once the routing was checked on a live system.

Only **two** things still need ZADT_VSP: **stateful RFC** (SOAP-RFC cannot hold
a session) and **git**. Read, edit, debug, AMDP, table reads, module lists and
transports are all plain ADT. AMDP left that list this week.

And "git" is not really our package. `vsp copy` handles **six** object types
natively — PROG, CLAS, INTF, DDLS, BDEF, SRVD — while the Z path handles
whatever abapGit does, by delegating: `ZCL_VSP_GIT_SERVICE` calls
`zcl_abapgit_objects=>supported_list( )`. So the dependency is **abapGit**, and
our service is a bridge to it.

That gives the bootstrap problem a hard edge the earlier note missed. The six
native types include **PROG and CLAS**, and a program deploys to a bare system
over plain ADT — verified twice this week, on two releases. So:

- abapGit *standalone* can already be installed with zero Z code. It is one
  report. But it exposes no global classes, so nothing can call it.
- Therefore a lean receiver **must itself be a report or a class**. Not a
  function group, not a package of objects — those cannot be installed by the
  six, and a receiver that needs a receiver is the circle it was meant to break.

**Step one, unstarted:** confirm a *class* deploys to a bare system the way a
program does. Everything above rests on it and it is fifteen minutes. Nothing
should be built before that is checked — building a bootstrap for a dependency
whose shape we had wrong is how this note started.

## Still open, smaller

- **AMDP table contents.** Address right, HANA's `INIT` refuses. Untried: the
  `tableHandle` the stop reports, which appears in none of that resource's
  parameters and so may belong to another route.
- **`WBCROSSGTI`** is wired only to *explain* an empty answer, not merged into
  the data — deliberate, until someone decides what a merged list should mean.
- **`WBCROSSGTX` decodes downward, not upward.** `graphFromCross` stays
  object-level; `LONG_NAME` takes `LIKE` despite being `STRG`, recorded at the
  function.
- **7.50 coverage** for the newest work: the ST22 check in `execute`, `--impact`
  where the dump detail resource is absent.
- **gCTS** — parked, never checked at all.
- **Three orphan packages** — `ts2go` archive, `jseval` extract, `cache`
  connect. Decided, not executed.
- **The article.** Material is now large and unusually concrete: self-repairing
  SSO, RFC with no gateway, a debugger working on a release with no stack
  resource, an AMDP breakpoint after months of "impossible", ten dead features,
  and a tool that returned a hash and called it an object.

## Recently landed

- `61a0375` — `NoModification` no longer fails a MODIFY lock that came with a
  handle, and the guard no longer leaks the ENQUEUE it just took. That leak was
  the cycle where every retry hit its own orphan lock and reported it as
  "NoModification".
- `583f042` — create RFC-enabled function modules in one call; SE37 is out of
  the loop. Verified end to end: the created module answers a classic RFC call.
- `99f6896` — a live host name removed from a context table, a design note and
  three reports.
- `3bb165f` — browser SSO merged from the Windows side.

---

## Other tracks (not this repo)

Two threads run alongside and have their own private notes, deliberately not
tracked here:

- **Inter-agent IRC bus** — channel autodiscovery from the git remote is
  written and waiting on a decision to push; an April security review has six
  of seven findings still open, three of them Critical. The write-up names
  unpatched issues in a public repo and must stay private until they are fixed.
- **Storage reorganization** on the shared network volume — done, reversible
  from a manifest.
