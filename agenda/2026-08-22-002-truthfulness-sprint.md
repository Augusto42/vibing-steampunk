# What the audit found, and what to do about it

**Author:** wsl-claude
**Date:** 2026-08-22
**Status:** first item done, the rest queued

## What happened

An eleven-agent audit read the README in full, both published articles, the MCP
tool surface, every package under `pkg/`, the debugger, and the standing agenda.
It inventoried 181 promises, verified 134 of them against the code, and found 68
that were overstated, wrong, or unverifiable.

The finding worth internalising is not any single claim. It is that **the project
is more capable than it says in some places and less in others, and a reader
cannot tell which is which**. That is a credibility problem, and credibility is
cheap to repair and expensive to lose. It is best not lost at the moment the
debugger finally gets attention.

## Done

**Tool counts.** Three published numbers, none right: `--mode` help said 100/147,
the long usage said 81/122, the README said 147 in twelve places. Measured:
**hyperfocused 1, focused 101, expert 146**. Corrected everywhere and pinned by
`internal/mcp/tools_parity_test.go`, which asserts each count, requires focused
to be a subset of expert, and requires every whitelisted name to be a tool that
exists.

That last assertion failed on its first run and found a real defect: four gCTS
entries whitelisted tools that no mode registers, because `registerGCTSTools` is
never called. The entries are gone. See the open decision below.

## Also done, same day

**All eight remaining claim corrections**, each checked against the code rather
than taken on the audit's word — and one the audit had wrong: abaplint has 13
rules of which 8 are on by default, so "8 lint rules" was defensible and the
phrasing was not. Statement patterns are 95, not 91 and not 94.

**Released as v2.42.0**, after the corrections rather than before, so the
announcement does not point at numbers already known to be false.

**Field reports closed.** `save_to_file` now creates its output directory. The
git export nests by subpackage again on the system that was running an older
copy of the class. A WebSocket opened after a session refresh used the session
from startup — a real bug, found while chasing a symptom that turned out to be a
stale process holding a deleted inode.

**gCTS probed.** Live on two systems of four (200, empty repository list), 403
where authorization is missing, 404 on 7.50 where it does not exist. So the
service is not dead — the wiring is.

## Queued, in the order they pay off

**1. ~~The rest of the claim corrections~~ — done.** Kept here for the record:

- `README.md:3` — "everywhere ADT is available" contradicts our own
  `pkg/adt/compat.go`, which records that a resource present on S/4 is absent on
  ERP. Replace with "any system with ADT enabled (7.50+); available surface
  varies by release", and point at the compatibility matrix. RAP is S/4-only,
  AMDP needs HANA.
- `README.md:15` — "8 hands-on tasks that need no SAP system"; the guide itself
  has 11 tasks, 6 of them offline.
- `README.md:153` — "45ms in three requests". The harness counts four round
  trips, and the measurement lives in an `integration` test that only logs.
  Either commit the measurement with system, release and date, or reword.
- `README.md:224` — write paths over the RFC-ADT tunnel described as "proven".
  There is no test, no transcript and no date. Downgrade or add the test.
- `README.md:369` — compression ratios 7–30x. No test computes a ratio. Turn one
  live measurement into an offline fixture with an assertion.
- `README.md:440` — "diagnostics on every save". `didSave` is not handled at all;
  diagnostics fire on every change, debounced. **The truth is the stronger
  claim.**
- `README.md:466` — points at branch `feat/wasm-abap`; the work is on main.
- `CLAUDE.md` — "8 lint rules" (13) and "91 statements" (~94).

**2. Typed per-action parameters on the universal tool.** Today
`SAP(action="query", params={"sql":...})` answers "No handler found", and
`RUN_REPORT` accepts a wrong key without complaint and runs with defaults. This
is the strongest argument against converging on hyperfocused, and it is fixable
independently of that decision.

**3. Routes for what the universal tool cannot reach** — i18n (7), revision
history (3), `AnalyzeABAPCode` — or an explicit statement that they are
expert-only. Not a truthfulness fix but a capability decision, which is why it is
here and not above. Note the asymmetry cuts both ways: eight analysis handlers
(`impact`, `health`, `co_change`, `cr_history`, …) are reachable *only* through
`SAP()` and are registered as tools nowhere, so hyperfocused is already richer
for analysis.

Nobody has measured the shortfall mechanically. Three different figures are in
circulation — 21 unreachable, "122 of 146" in a file header, 24 in a draft test.
Measure it before publishing it.

**4. The debugger's test debt.** It has **zero** tests that run by default. The
only cross-transport guarantee is behind the `integration` tag and needs a live
system *and* the `ZADT_DEBUG` facade — so it does not exercise the "no Z code
needed" claim at all. Uncovered: stepping, `SetVariable`, `GoToFrame`, batch
capture, recording, non-line breakpoints, rejection reporting, `SystemDebugging`,
post-mortem, error paths, the MCP handlers, the Lua bindings.

The way in is replaying recorded transcripts: `pkg/saprfc/record.go` already
produces statement-level traces with values, which is an oracle no competitor can
easily assemble. Also: `pkg/adt/integration_test.go:1829` asserts that the
stateless client's debug calls *return errors* — it pins broken behaviour and
reads as debugger coverage. Rename it so that is visible, or delete it.

## Open decisions

**gCTS: connect or delete.** `registerGCTSTools` is called from nowhere, so ten
tools are dead in all three modes. Orphaned behind them:
`internal/mcp/handlers_gcts.go` (241), `pkg/adt/gcts.go` (386), `gcts_test.go`
(257) — 884 lines with no consumer in `cmd/`. There is no `routeGctsAction`
either. Leaving it as is means carrying code that nothing can call.

**The dead and the orphaned.** `internal/mcp/tools_aliases.go` is 59 lines inside
a block comment, called and doing nothing. The session half of
`pkg/adt/websocket_debug.go` has no callers outside its file. `pkg/ts2go` — 608
lines, no tests, no importers, and it is TypeScript-to-Go, not ABAP.
`pkg/jseval` (1 518) and `pkg/cache` (1 188) have zero consumers: adopt, extract
or delete, but do not leave.

**`vsp install abapgit` cannot work.** Both embedded archives are 0 bytes, so the
tool is offered in every mode and always fails; `edition="dev"` names a
dependency that does not exist; the actual deploy is a TODO. Separately,
`make refresh-deps` sources those archives *from a SAP system*, which means a
shipped binary would carry whatever was on one developer's machine — there is no
reproducible build of that artifact. Fetch from upstream by tag with a checksum,
or drop the tool and print a link.

## Strategy, for the record

The audit weighed three directions: (a) best-in-class debugger plus static and
dynamic analysis, (b) open-abap-go as a separate project, (c) polish every
feature to proven parity across systems.

Its recommendation was **(a), with a time-boxed slice of (c) as the entry
ticket** — two to three weeks of making the documentation true, then all effort
into the debugger and dynamic analysis. The reasoning: the debugger is the only
part of the repository that is both new and hard to reproduce; everything else is
either a wrapper over ADT that any competitor can build, or research, or dead.
Full parity is unachievable by definition — RAP does not exist on ECC, AMDP does
not exist without HANA — so only documenting the differences is achievable, and
that is much cheaper work.

(b) is parked. The thinking behind it is kept, with what could be reused and what
would change the verdict; only the `abaplint` grammar is funded, because option
(a) needs it too.

## An article

There is one, and the material is this day's work rather than a feature list: a
sign-on that repairs itself, RFC over a WebSocket with no gateway and no
password, one syntax error in an unrelated class silencing a whole tunnel, a port
nothing on the machine knows, and an audit in which the project found 68 of its
own overstatements. An honest account of one's own inflation reads better than
any capability list — but it publishes *after* the corrections, or it sets them
in print.
