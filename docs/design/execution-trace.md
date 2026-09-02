# Execution truth: what really ran, with what arguments, and how it differs from the graph we extracted

Design note for Sprint 5. Everything below rests on one fact established on
2026‑08‑21: ADT's own REST resources work through the classic-RFC tunnel on a
pinned conversation, so any resource in the discovery document is reachable
without an HTTP port and without installing anything. The resources this note
depends on are all present on A4H (SAP_BASIS 758), verified in the discovery
document:

| Resource | What it gives |
|---|---|
| `/sap/bc/adt/debugger/variables` | the debugger's typed variable model |
| `/sap/bc/adt/runtime/traces/abaptraces` + `/requests`, `/parameters` | ABAP runtime traces (SAT): the **measured** call tree |
| `/sap/bc/adt/crosstrace/traces` | ABAP Cross Trace — the RFC/HTTP boundaries |
| `/sap/bc/adt/amdp/debugger/main` (`…v4+xml`), `…/debuggees/{id}/variables/{var}` | the AMDP (HANA) debugger |
| `/sap/bc/adt/datapreview/amdpdebugger` | table cells inside an AMDP debug session |
| `/sap/bc/adt/runtime/workprocesses` | who is doing what right now |

## The four questions worth answering

1. **What did this program actually call?** A static graph cannot know: ABAP
   resolves `CALL FUNCTION lv_name`, `CALL METHOD (lv_meth)`, `PERFORM (f)`,
   `SUBMIT (rep)` and every RFC destination at runtime. The extracted graph is a
   hypothesis; a trace is evidence.
2. **What crossed the boundary?** At the edge of a code unit — a function
   module, a method, a form — the arguments in and out are the contract. Capture
   them and a unit becomes replayable and testable in isolation, without knowing
   how it is wired into the system.
3. **What happened, in order, everywhere?** The full statement-level history,
   for exact replay and for answering "what was that variable when it went
   wrong" after the fact.
4. **Does AMDP work the same way?** ABAP-managed database procedures run in
   HANA, and have their own debugger resource. If it tunnels, the story covers
   code that leaves the ABAP stack.

## The three mechanisms, and their honest costs

**SAT traces — cheap, measured, no values.** A trace request is created for a
user/program, the workload runs, and SAP hands back a call tree with hit counts
and times. Nothing is stepped, so it costs almost nothing and can run against
real workloads. It answers question 1 completely and question 2 not at all.

**Debugger scripting — the only way to capture values without changing code.**
`IF_TPDAPI_SESSION~GET_SCRIPT_HANDLER` exists in the API we already drive. SAP's
debugger scripting runs ABAP at each stop, inside the debuggee's context, so a
script can read parameters and continue — **without a network round trip per
step**. That last point is what makes it viable: stepping from outside costs one
RFC call per statement, which is milliseconds each and therefore minutes for a
real program. A script that records into an internal table and hands it back in
batches turns that into one call per batch.

**Stepping from outside — simple, slow, and fine for small things.** We already
have it. Use it for a bounded region (one method, one loop) and never for a
whole transaction.

## What we build

### A record format that is one thing, not three

One JSONL stream, one object per event, so the same file serves the call graph,
the boundary capture and the full history — differing only in how much of it was
recorded:

```json
{"seq":41,"kind":"enter","unit":{"type":"FUNC","name":"ZADT_DEBUG_LOOP","program":"SAPLZADT_DEBUG","include":"LZADT_DEBUGU01"},"line":9,"caller_seq":12,"args":{"IV_X":"…"},"ts":"…"}
{"seq":88,"kind":"exit","unit_seq":41,"returns":{"EV_Y":"…"},"exception":null,"ts":"…"}
{"seq":89,"kind":"call","from_seq":41,"target":"Z_OTHER","resolution":"dynamic","destination":"A4H@GEN"}
```

`resolution` is the field the whole exercise exists for: `static` when the
extracted graph already had this edge, `dynamic` when only the run knows it.

### `vsp trace` — the collector

- `vsp trace run <PROGRAM>` — SAT trace of one run, returns the measured tree.
- `vsp trace watch --user X` — trace what a user does next, for interactive flows.
- `vsp trace units <PROGRAM> --capture args` — debugger-script capture at unit
  boundaries.
- `vsp trace full <PROGRAM>` — statement-level history; refuses without a bound
  (`--max-events`, `--timeout`) because it is unbounded by nature.

### `vsp trace study` — the offline tool

Modelled on `rfc-viewer`: reads the JSONL, never touches a system.

- the observed call graph, and a **diff against the extracted one**: edges only
  in the static graph (never exercised — dead, or untested), edges only in the
  trace (**dynamic**, the interesting ones), edges in both;
- a unit view: every call of one unit with its arguments, so you can see which
  inputs actually occur in practice;
- **replay skeletons**: pick a recorded call, get an ABAP unit test with the
  captured inputs asserted against the captured outputs — the isolation the
  arguments were captured for;
- an HTML mode (`--html`) and a `--serve` mode, as `rfc-viewer` has, because
  reading a graph in a terminal is a waste of a graph.

### Redaction is not optional

A boundary capture contains business data by construction. Values are redacted
by default and included only with an explicit flag, exactly as `rfc-viewer`
already does. Field-name and type-based rules (anything `PASSWORD`, `*KEY*`, a
`BAPIRET2` message, a bank account) stay redacted even then.

## Sequencing, cheapest decisive first

1. **Variables** — the debugger is advertised as inspecting them and does not
   yet. `/sap/bc/adt/debugger/variables` gives the whole typed model for free.
2. **SAT traces** — the measured call tree, no new mechanism, immediately useful.
3. **Graph diff** — the static graph already exists in vsp; the diff is the
   first genuinely new insight and needs no debugger at all.
4. **AMDP spike** — one blocking question: does the AMDP debugger tunnel, and
   what HANA privileges does it want.
5. **Boundary capture via debugger script** — the first hard one.
6. **Full history and replay** — only once the record format has survived (5).

## What could make this fail

- **Debugger scripting may be unavailable or heavily authorised.** If so,
  boundary capture falls back to outside-stepping, and the honest answer is
  "small regions only".
- **Trace authorisations** are their own set (`S_DEVELOP`, trace-specific), and
  tracing another user needs more than tracing yourself.
- **Volume.** A statement-level trace of a real transaction is large; the format
  is JSONL precisely so it streams and truncates cleanly.
- **AMDP debugging needs HANA-side rights** that an ABAP developer often does
  not have.
