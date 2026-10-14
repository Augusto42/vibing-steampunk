# Handoff: the graph, forward from here

Written for the session taking this over. Read `CLAUDE.md`, then this,
then `agenda/2026-08-23-004-graph-audit.md` and
`agenda/2026-08-23-005-upward-tracing.md` — this is the map, those are
the findings.

## Files

`pkg/graph/`, `pkg/adt/callees.go`, `pkg/adt/methodinclude.go`,
`pkg/adt/correlate.go`, `pkg/adt/dumpimpact.go`,
`internal/mcp/handlers_graph.go`, `cmd/vsp/cli_extra.go` (the `graph`
command).

## The state, honestly

Thirteen graph-facing capabilities are exposed. **Three are documented.**
All thirteen answer live as of today, which was not true this morning:
`callers`, `callees`, `object_structure` and `usage_examples` were dead,
and `where_used_config` had never returned a row in its life.

Every one of those was in the undocumented ten. That is the finding to
carry: **undocumented is unaudited.** Nothing said they were supposed to
work, so nothing noticed when they did not.

The three documented ones are in `docs/graph-guide.md`, which is an
honest document — it states its limits without softening them. The ten
have no such protection.

## The data, checked rather than assumed

Both cross-reference tables read over **plain ADT free SQL**. No RFC, no
Z code. That is the whole basis for doing this in Go.

| | |
|---|---|
| `CROSS.TYPE` | `C(1)`. Invocations: F function module, R report/SUBMIT, T transaction, U subroutine, P program, D dialog module. Constants in `pkg/adt/callees.go`. |
| `WBCROSSGT.OTYPE` | `C(2)`. `TY` type reference, `DA` data object. |
| `WBCROSSGT.COMPONENT` | **A flag, not a name.** `C(1)`, holds `X` where an object describes itself. The component is packed into `NAME` behind a backslash: `ZCL_X\DA:GT_SERVICES`. |
| `CROSS.PROG` | Exists, empty in every row sampled. Not the shortcut to the owner it looks like. |
| `TMDIR` | `(CLASSNAME, METHODINDX, METHODNAME)`. The method-include mapping. |

Two of those five mislead by their names. That is not accidental — it is
why this area rotted.

## What was just unblocked

**A method include decodes.** `ZCL_X===========CM001` → method 1 →
`TMDIR` → `CLASS_CONSTRUCTOR`. The `CM` suffix is the index in
**hexadecimal**: one class live had `CM001, CM003, CM009, CM00A`, which
is 1, 3, 9, 10 — decimal has no `A`.

`adt.DecodeMethodIncludes(ctx, includes)` does it, grouped by class so a
class costs one query however many of its methods appear. Sections that
are not methods (`CI`, `CU`, `CCIMP`) come back as sections rather than
being dropped, because "this reference sits in the class definition" is a
real answer.

**This is the piece upward tracing was missing**, and it is unused so
far. Nothing calls it yet. That is the obvious next move: cross-reference
rows carry the include, and now the include carries the method.

## Three traps, all of which cost time today

**`/sap/bc/adt/cai/` does not exist.** Not "not on older releases" — it
is advertised in the discovery document of none of 7.50, 7.57 or 7.58 and
answers 404 on all of them. The call graph and the object explorer were
both built on it. The functions are deleted with a gravestone where they
stood; do not rebuild on that namespace.

**Discovery lies in both directions.** On 7.50 `breakpoints/vit` answers
200 without being advertised; `applicationlog/objects` is advertised and
answers 400; the runtime-dump resources are advertised nowhere at all.
Ask the system, do not read the catalogue.

**A rule inferred from behaviour fits the examples to hand.** `'FU'` in a
`C(1)` column. A section-prefix list covering `U01` and missing `U27`.
Both looked right and failed silently — an empty result, never an error.
When SAP does something in the kernel, look at what the same class reads
from a table; `reports/2026-08-23-002-reading-the-handler.md` has three
worked cases.

## What ZRAY already solved, and worth stealing

`ZLLM_00_NODE` / `ZLLM_00_EDGE` on a4h — a property graph with three
decisions `pkg/graph` has not reached:

- **`SEED`**, a crawl-run id on every node and edge. A re-crawl is a new
  snapshot rather than a destructive rewrite, so two coexist and can be
  compared. We have no persistence at all.
- **Containment in the node** — `ENCL_OBJ_TYPE`/`ENCL_OBJ_NAME`, so a
  method knows its class without a traversal.
- **Three booleans** — `IS_CODE_UNIT`, `IS_TYPE`, `IS_COMPOSITE`. Exactly
  the invocation-versus-type-reference distinction `callees` had to
  invent this week.

`ZCL_XRAY_GRAPH` (~1700 lines) is a bidirectional crawler with the two
things crawlers usually lack: `IS_END_OF_RECURSION` and
`UPDATE_PROCESSED_ID`. `ZCL_RAY_10_SPIDER` takes the direction as a
*parameter* (`bottom_up`/`top_down`/`flat`) and the work at each node as
an **injected LLM step**. Note that upward was never really built there
either — so there is nothing to copy for that half.

## The order I would keep

1. **Write down the ten undocumented capabilities** — what each answers,
   from which source, what it cannot see. Cheap, and it is the half that
   stops today repeating.
2. **Make each primitive state its own blindness in its answer**, the way
   `callees` does now ("references recorded at activation, not observed
   calls, so a dynamic CALL METHOD (name) appears nowhere"). Six of the
   thirteen still do not.
3. **Wire `DecodeMethodIncludes` into upward tracing**, so a caller is a
   method rather than a class.
4. **Then** think about a traversal layer, with ZRAY's model in front of
   you. The primitives are the product; the walk is the part that keeps
   being wrong — `max_depth` was removed this week because both sources
   are one hop and depth was never real.

## House rules that bite here

Only `a4h` may be named in tracked files. Never provoke a failure with a
real user and a wrong password — a locked account cost a day today; a
nonexistent user is safe, a client-side refusal (`--block-free-sql`) is
safer. And say what you did not verify: half of today's findings were
things that looked like they worked.
