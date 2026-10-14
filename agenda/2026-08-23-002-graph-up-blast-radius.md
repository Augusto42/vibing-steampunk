# Graph up: blast radius, and why it is not a rung

Status: built and verified live on a 7.58 developer edition.
Follows [001-post-mortem](2026-08-23-001-post-mortem.md), which listed graph-up
as rung 4 of the correlation ladder. That placement was wrong, and this is the
argument for moving it out.

## The conclusion first

**Graph up does not belong in the correlation ladder.** Three reasons, in
descending order of how much they matter:

1. **A caller that took part in the failure is already on the stack.** Rung 2
   (`scoreOnStack`, 80) has it. So a caller that graph-up finds and the stack
   does not is, by construction, a caller that *did not run* during this dump.
   Nothing it ever wrote to the application log can be evidence about this
   failure. Scoring it at 30 would place a coincidence above "same user, shortly
   before" — which at least implies the same session.

2. **Blast radius has no timestamp.** The ladder ranks log entries inside a
   window on both sides of the dump. Callers are static repository facts, true
   yesterday and true next month. There is nothing to rank them against the
   clock with, and giving them a rung would manufacture a structural-looking
   score for something that cannot be structural evidence *for this dump*.

3. **They answer different questions and a reader would merge them.** The
   ladder's rows are all arguable and the tool says so. Impact rows are not
   arguable at all. Printed in one list, the certainty of the second leaks into
   how the first is read — the exact failure the ladder was designed against.

So: separate flag, separate output, shared plumbing. `vsp dumps --explain`
argues about cause; `vsp dumps --impact` states who else is exposed.

## What was built

`vsp dumps --impact <id|latest>` — from a dump, who else reaches the code that
failed, nearest the failing statement first.

- `pkg/adt/dumpimpact.go` — the mapping, the filtering, the ranking. Pure
  functions with the client method as a thin shell over them.
- `pkg/adt/dumpimpact_test.go` — no SAP needed.
- `cmd/vsp/dumps.go` — `--impact`, `--impact-frames`, `--impact-top`.

Units are walked outward: the dump's own program first, then stack frames
innermost to outermost, deduplicated, capped at `--impact-frames` (default 3).
Callers already on the stack are split off under "not additional exposure" —
they are the route this dump took, and seeing them is what confirms the query
aimed at the right object.

## What was deliberately not built

**`vsp graph callers <TYPE> <NAME>`.** It already exists as
`vsp graph <TYPE> <NAME> --direction callers`. A second spelling of a command
that is already there is not a deliverable.

**A second where-used engine.** `examples` and `rename-preview` each carry an
inline CROSS/WBCROSSGT query. Neither was extended and no third copy was added;
the ADT where-used list is a better source than both and needed no new engine.

**Method-level radius.** Checked live: a URI of the form
`.../classes/x/source/main#type=CLAS/OM;name=M;start=1` is *ignored* by the
where-used resource — it resolves to the class and returns the class's list,
with `objectIdentifier` naming a `\TY:` type reference rather than the method.
So the answer is object level and says so in its own footer. Method-level call
sites are what `vsp examples --method` already does by reading caller source.

**Reviving the graph-down rung.** See below. It is documented where it breaks,
not fixed here.

## Four things the live system said

Everything below was checked against 7.58, some of it with raw requests holding
a CSRF token, so a 403 could not be mistaken for a missing resource.

**1. `/sap/bc/adt/cai/callgraph` does not exist.** 404, "No suitable resource
found", in both directions. `GetCallersOf` and `GetCalleesOf` are wrappers
around it, so both are dead on this release. `vsp graph` already knew — it has
a WBCROSSGT fallback and prints "ADT call graph not available" every time.

This has a consequence nobody had noticed: **rung 3 of the correlation ladder
(`scoreCalledByStack`, 60) can never fire.** `calleesOfStack` calls
`GetCalleesOf` per frame and treats an error as "this frame contributes
nothing", so the failure is silent and the rung is simply unfed. Documented at
the function; reviving it means asking CROSS the other way round
(`SELECT NAME, TYPE FROM CROSS WHERE INCLUDE = <include>`), which is a separate
change.

**2. `usageReferences` answers, and answers better.**
`/sap/bc/adt/repository/informationsystem/usageReferences` — the list behind
SE84, already spoken by `FindReferences` and until now used only from MCP —
returns the caller's package, its object type, and a grade per reference. It is
the right source and it needed no new code.

**3. Two filters carry the whole result, and both are easy to omit.**
- The list is flat but two-level: a row with no `usageInformation` is a
  container and only the rows beneath it are references.
- `gradeComponent` rows are the target describing its own parts — every method
  of the class you asked about. Only `gradeDirect` is somebody else reaching in.
- Found by running it: packages come back as containers too, with their package
  interfaces listed underneath as `gradeDirect`. A package cannot call anything.
  Without that filter the first live run reported `$ZADT_VSP`, `SAPC_RUNTIME`
  and `SABAP_CHANNELS` as callers, and `SAPMSSY1` — a kernel dispatcher — as
  having exactly one caller, which was a `PINF/KI`.

**4. A function group cannot be asked at all.** Asking about
`/sap/bc/adt/functions/groups/<g>` returns 200, zero results, and a description
reading "<G> - SAPL<G> (Include)": the group URI resolves to the group's *main
include*, which nothing references, so the list is empty whatever the truth is.
Callers live on the modules. The known-good comparison: the same query against
one module returned 48 references.

This is the dangerous shape — a successful, plausible, empty answer — so it is
reported as "not asked" with the reason, and `DumpImpactResult.Answerable()`
lets the caller tell "nothing calls this" apart from "nothing here could be
asked". A dump frame that names its module is asked about the module and never
hits this; only a dump that names a group and no module does.

## Two bugs fixed on the way

**`programURI` sent function groups to `/programs/programs`.** `SAPLSBAL_DB`
became `/sap/bc/adt/programs/programs/saplsbal_db`, which is a 404 on every
system — and a silent one, because the caller treats an unreadable frame as a
frame that contributes nothing. It now defers to `unitForFrame`, which handles
class pools, interface pools, function groups, function modules and programs.

`TestProgramURIUnwrapsAClassPool` **asserted the bug**: it required
`SAPLZDEMO_FG` to resolve under `/programs/programs`. The expectation was
updated and the comment says why.

**`vsp graph FUNC <name>` asked for a class.** `runGraph`'s URI switch had no
`FUNC` case, so a module fell into the default and was requested as
`/oo/classes/<name>`. It now resolves the group through TFDIR first.

## One thing improved rather than duplicated

`vsp graph <T> <N> --direction callers` now asks the where-used list before
falling back, since the resource it used to try first is absent everywhere. The
difference on a real module:

```
before:  raw include names, one column
         CL_RSO_WORKSPACE_BACKUP_CREATECM003
         /BOBF/I_COM_GEN_FRW_IMPL_18

after:   object, type, package, code-or-test, and the referencing methods
         CL_RSO_WORKSPACE_BACKUP_CREATE  CLAS/OC  BW_CORE_TOOLS  code
           FIND_ERRORS_WARNINGS, IF_STCTM_TASK~SHOW_MESSAGE_DETAILS
```

An empty answer is now trusted instead of retried twice under a banner reading
"ADT call graph not available" — which described a query that had succeeded.

## Empty landscape or broken query?

The test system is a developer edition with almost no custom code, so an empty
result had to be told apart from a broken one. It was, three ways:

- A **known-good comparison**: one function module returned 48 references from
  the same code path that returned 0 for the group. The engine works; the group
  query does not.
- A **raw request with a CSRF token** for every 404, so a missing resource
  could not be confused with a rejected request.
- A **positive result in our own namespace**: the impact query on the
  `APC_ILLEGAL_STATEMENT` dump found `ZCL_VSP_APC_HANDLER` calling
  `ZCL_VSP_REPORT_SERVICE` from `CLASS_CONSTRUCTOR`, correctly placed it on the
  dump's own stack, and correctly reported no additional exposure. That is a
  true empty answer, and it is printed differently from an unanswerable one.

One case is genuinely unverified: a `--impact` run against 7.50, where the dump
detail resource is absent. The code reports `StackUnavailable` and falls back to
the dump's own program, but that path has not been exercised against a real
7.50 system.

## What this leaves open

- **Graph down is unfed on any release without CAI.** The CROSS-based
  replacement is sketched at `calleesOfStack` and not written.
- **`examples` and `rename-preview` still each carry their own inline
  CROSS/WBCROSSGT query.** Now that `Client.WhereUsed` exists and is better on
  both counts, they are the obvious next consolidation — which is also the
  `cli_deps.go` + `cli_extra.go` + `ctxcomp/analyzer.go` unification already on
  the graph-engine list.
- **Method-level impact** would need the `#start=line,col` form on the source,
  which means reading the class and locating the method. Worth it only if
  object level proves too coarse in practice.
