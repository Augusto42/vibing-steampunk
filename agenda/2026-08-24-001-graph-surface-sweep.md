# The graph surface, exercised rather than counted

Every graph-facing capability called against a live 7.58 system, one after
another, to see what answers. Not a code review: each row below is a request
that was made and a reply that came back.

## Read this first: what was actually tested

The MCP server process had been running since **19:31 the day before**, and a
process keeps the binary image it started with. So this sweep exercised the
build as it stood at that moment, not the release cut later the same night.
Some defects below may already be fixed in code; none of them can be assumed
fixed, and the two marked *stale?* are the ones with a known fix in the tree.

That is the first finding, and it outranks the rest:

> **A sweep must report which build it exercised.** Testing through a
> long-lived server measures whatever that process started with, and the answer
> looks identical either way. This nearly went into the record as "the state of
> the release".

And the corollary, learned the same night from the other direction: **a stale
image must not become the default explanation.** Two failures here were
artefacts of one and needed no fix; a third, reported by a neighbouring project,
looked like the same artefact and was a real defect that had been shipping since
January. The rule cuts both ways — check the build, then check the code anyway.

## The second finding: the inventory was wrong

The previous audit worked from a list of thirteen — four documented, nine not.
The router in `internal/mcp/handlers_analysis.go` dispatches **fifteen**.
`trace_execution` and `compare_call_graphs` appear in neither column.

A sweep of a list inherits the list's blind spots. The count here came from
reading the router, which is the thing that decides what exists.

## The surface

| # | capability | verdict |
|---|---|---|
| 1 | `callers` | answers; names its source and the method inside each caller |
| 2 | `callees` | answers; 27 rows, with the activation caveat |
| 3 | `call_graph` | answers; `both` returns two answers rather than one merged tree |
| 4 | `impact` | answers |
| 5 | `co_change` | answers; "No transports found" is honest for a local package |
| 6 | `usage_examples` | answers; snippet, line number, confidence |
| 7 | `analyze_deps` | answers; nine dependencies, two layers, line numbers |
| 8 | `compare_call_graphs` | answers — see *quality* below |
| 9 | `graph_stats` | answers, but only for source handed to it — see *narrower than its name* |
| 10 | `references` | answers — see *unbounded* below |
| 11 | `object_structure` | **fails**: 404 on a resource that does not exist |
| 12 | `where_used_config` | **fails**: 400, the C(1) bug *(stale?)* |
| 13 | `check_boundaries` | **lies**: CLEAN on a package with three crossings |
| 14 | `trace_execution` | **silent**: returns almost nothing, says nothing *(fixed in tree)* |
| 15 | `analyze_call_graph` | **miscounts**: 27 edges, 2 nodes |

Ten answer. Five do not, or answer wrongly.

## The failures, worst first

### `check_boundaries` returns CLEAN for a package it never read

Asked about `$ZADT_VSP`:

```
Total dependencies: 0
  Violations: 0
  ✓ CLEAN — no boundary violations
```

The CLI's own boundary command, same package, same system:

```
Boundaries: $ZADT_VSP (1 packages, 13 objects scanned)
  WARN  EXTERNAL     3
    ZCL_VSP_GIT_SERVICE → INTF ZIF_ABAPGIT_OBJECTS
    ZCL_VSP_GIT_SERVICE → INTF ZIF_ABAPGIT_DEFINITIONS
    ZCL_VSP_GIT_SERVICE → CLAS ZCL_ABAPGIT_I18N_PARAMS
```

Thirteen objects and three crossings, against zero and a clean bill. This is the
sentence in `pkg/adt/client_fugr_gaps_test.go` — "a boundary report comes back
clean about code nobody read" — observed one floor up from where it was written.

**Lead, not yet confirmed:** the package branch calls
`s.adtClient.GetSource(ctx, "", obj.Name, nil)` with an **empty object type**,
where the CLI passes the type. `GetSource` switches on that type and has no
branch for the empty string. If every read fails, every object contributes no
edges, and no edges is what a clean package looks like.

Worth noting the verdict is not merely wrong — it is wrong in the reassuring
direction, which is the direction nobody double-checks.

### `trace_execution` answers with silence

```json
{ "execution_time_us": 0,
  "static_stats": { "total_nodes": 2, "total_edges": 27, ... } }
```

No trace, no actual edges, no comparison — and no word that the trace never
happened. `Comparison` is the entire point of the call, and it is absent both
when prediction and reality agree and when nothing ran. Opposite conclusions,
identical output.

Fixed in the tree (`e678848`): four swallowed failures now reported. The fix was
written from reading the code; this run is the live confirmation.

### `analyze_call_graph` counts two nodes for twenty-seven edges

```json
"stats": { "total_nodes": 2, "total_edges": 27,
           "unique_nodes": ["ZCL_VSP_GIT_SERVICE", "CL_ABAP_CODEPAGE"] }
```

Twenty-seven distinct callees are listed in the same answer. Whatever
`AnalyzeCallGraph` walks, it is not the edge list beside it. Every edge also
carries an empty `callee_uri`, so nothing in the result is addressable.

The same broken stats block is returned by `trace_execution`, so one fix serves
both.

### `object_structure` is still on a namespace that does not exist

```
404 at /sap/bc/adt/cai/objectexplorer/objects
```

The previous audit recorded this as fixed. In the tree,
`GetObjectStructureCAI` now delegates to `GetClassObjectStructure`, which speaks
a resource that answers. Either the running build predates that, or a second
caller was missed. **Recheck against the release before spending anything on
it** — this is exactly the case the stale-process caveat exists for.

### `where_used_config` still asks for `'DA'` in a `C(1)` column

```
400: 'DA' is not a valid value for C(1,0).
```

The defect the previous audit identified and reported fixed. Almost certainly
the stale process; recheck first.

## The three that answer but are not right

**`graph_stats` is narrower than its name.** It accepts only `source` handed to
it — `// For now, build a fresh graph from provided source` — and cannot be
asked about a repository object at all. Not broken; misnamed and undocumented,
which is how nobody found out. Given source, it also returned two edges for a
snippet containing a dynamic `CALL METHOD (lv_dyn)=>go`, so the dynamic call
either was not extracted or was not counted. Unverified which.

**`references` is unbounded.** One question produced 113 entries, 56,000
characters, 1,584 lines — more than an agent's context can hold, so the tool
that answers is the tool that cannot be used. The rows are the raw where-used
response, container entries included: most carry `isResult: false`, meaning they
are packages and function groups listed as scaffolding, not answers. Needs a
cap, a summary, and the non-results dropped.

**`compare_call_graphs` counts type references as untested paths.** It reported
`untested_paths: 26` and `coverage_ratio: 0.037` where most of the 27 static
edges are type and data references — `ABAP_BOOL`, `SYST`, `TADIR`. A type
reference is not a path anything could execute, so the ratio measures nothing.
The `calls: true` flag that `callees` already carries is the fix.

## What was done about it, the same night

| finding | outcome |
|---|---|
| `check_boundaries` CLEAN on an unread package | fixed, `b3a3bbc` — three defects stacked, see below |
| `trace_execution` silent | fixed, `e678848` |
| `analyze_call_graph` miscounts | fixed, `b1b4f29` — dedup keyed on an empty URI |
| `compare_call_graphs` ratio | fixed, `b1b4f29` — coverage over invocations only |
| `references` unbounded | fixed, `84487ae` — scaffolding separated, capped, cap declared |
| `object_structure` 404 | **no defect** — stale process; the replacement endpoint answers live |
| `where_used_config` 400 | **no defect** — stale process; the `'DA'` query is not in the tree |
| `graph_stats` source-only | **open** — a scope decision, not a bug |

`check_boundaries` turned out to be three defects, each producing the same
reassuring output. The package listing carries SAP's two-part codes (`CLAS/OC`)
and the filter compared bare kinds, so every object was skipped before anything
was read — which is why no "could not read" note appeared either: nothing was
attempted. Under that, the read passed an empty object type to `GetSource`,
which switches on it and has no branch for the empty string. And the verdict
ignored `Unknown` entirely, though `classify` sends an unattributable reference
there and never to `Violation`, so an unknown can only ever hide a violation.

Two findings arrived that the sweep did not go looking for. Probing the
dependency extractor with a snippet — rather than reading it — showed
`CALL FUNCTION lv_fm_name` emitting a **static** edge to a function group named
after the variable, an object that exists nowhere; and dynamic method calls
(`CALL METHOD (lv_dyn)=>go`) classified and extracted by nobody. Fixed in
`62c4c8e`. The first is the SHA-1 defect in another room: an invented answer
rather than a missing one, and it propagates into the boundary verdict.

## Order

1. `check_boundaries` — a false CLEAN is the only defect here that makes someone
   ship something.
2. Recheck `object_structure` and `where_used_config` against the release before
   touching either.
3. `analyze_call_graph` stats — one fix, two capabilities.
4. `references` cap and `compare_call_graphs` ratio — both quality, neither
   urgent.

`trace_execution` is done.

## What the sweep is worth as a habit

Ten of fifteen answer. The previous audit said thirteen of thirteen did, and was
working from a list that was two short and a system that had since moved. Both
are honest mistakes of the same shape: **a claim about the surface that was
never made against the surface.**

The cheap protection is not documentation. It is a command that calls every
capability, prints what came back, and names the build it called — so that the
next person's answer to "does this work" is a transcript rather than a belief.
