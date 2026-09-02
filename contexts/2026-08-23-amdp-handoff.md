# Handoff: the debugger and AMDP, forward from here

For a parallel session. Read `CLAUDE.md` first, then this.

## Where to work

**Not in this directory.** Take a worktree:

```bash
git worktree add ../vsp-amdp -b feat/amdp-forward
cd ../vsp-amdp
```

Two sessions in one working tree share the index and the files: `git add
-A` picks up the other's half-written work, commits interleave, and a
branch switch pulls the floor out from under the other. Two *isolated*
agents still collided today on a helper name and on two identical
"find a dump" helpers, so this is not theoretical.

The other session is working on `pkg/graph/`, `pkg/adt/callees.go`,
`pkg/adt/correlate.go`, `internal/mcp/handlers_graph.go`. Yours is
`pkg/saprfc/amdp*.go`, `pkg/saprfc/adtdebug*.go`,
`cmd/vsp/rfc_debug.go`, `cmd/vsp/adt_debug.go`,
`internal/mcp/handlers_amdp*.go`. Say something before touching anything
else — we can message each other by session name.

## What works, verified live on 7.58

AMDP debugging over plain ADT, nothing installed on the server. The
breakpoint fires and a statement-level trace comes out:

```
41 → 42 → 45 → 49 → 50 → 40 → 41 → 42 → 45
```

Line 40 is the `WHILE`, so that is the loop going round — actual control
flow inside HANA.

```bash
vsp adt debug -s a4h --user <you> \
  -c "astart; abp ZCL_VSP_00_AMDP_TEST 41; aresume 12; atrace 8; astop"
# then, while it waits, from anywhere:
vsp execute -s a4h 'zcl_vsp_00_amdp_test=>calculate_squares( EXPORTING iv_count = 5 IMPORTING et_result = lt ).'
```

Commands: `astart`, `abp <CLASS> <LINE>`, `aresume [MAX]`, `astep
[over|continue]`, `atrace [MAX]`, `avar <NAME>`, `astop`. Also on MCP as
`AMDP_ADT_START` / `_BREAKPOINT` / `_AWAIT` / `_STOP`.

## Three things that will cost you a day if you rediscover them

**The answers are a queue, and acknowledgements sit at its head.** The
first resume after a breakpoint sync returns `SYNC_BREAKPOINTS`; the next
returns `ON_TOGGLE_BREAKPOINTS` with SAP's verdict on each breakpoint.
Neither is a stop. Resume once, see an acknowledgement, and you conclude
the breakpoint never fired — while the debuggee is blocked on it at that
moment. This project believed that for months. `AMDPAwaitStop` waits
past them; the same trap repeats one level down, because a step also
answers with an empty body and its new position arrives through the same
queue.

**Everything must stay on one held session.** `CL_AMDP_DBG_ADT_RES_MAIN`
keeps `debugger_main`, `debugger_control`, `main_id` and `session_id` in
**class-data**, which is ABAP session memory. A second connection finds
an empty session. Worse, the breakpoint resource does not check the id
at all and answers 200 for a `mainId` of `DUMMY`, so you can get two
successes in a row that mean nothing.

**Read the handler, do not guess the payload.** The breakpoint document
took several rounds of guessing and the namespace was still wrong.
`CL_AMDP_DBG_ADT_RES_BPS` names its transformation,
`amdp_dbg_adt_sync_bp_req`, and transformations are readable over ADT at
`/sap/bc/adt/xslt/transformations/{name}/source/main`. The template
states every element and attribute. One request beat three rounds of
inference. The same trick applies to everything else here.

## What is mapped and not done

From the discovery document, all under `/sap/bc/adt/amdp/debugger/main/{mainId}`:

- `debuggees/{debuggeeId}/variables/{varname}{?offset,length}` — `avar`
  exists and is **untested**; it was written and never exercised against a
  stop. Start here.
- `.../variables/{varname}{?setNull}` — writing a variable, unimplemented.
- `.../lookup{?name}` — unimplemented.
- Table-valued variables come through data preview, not the debugger:
  `/sap/bc/adt/datapreview/amdpdebugger{?rowNumber,colNumber,sessionId,debuggerId,debuggeeId,variableName,schema,provideRowId,action}`,
  plus a `cellsubstring` variant for long values.
- `breakpoints/llang` and `breakpoints/tablefunctions` — two more
  breakpoint flavours, untouched.

Reading a variable at a stop is the obvious next thing: a trace with
values is what makes it worth having, and the ABAP recorder already
proves the shape.

## Releases

7.58 and 7.57 have the AMDP debugger. **7.50 has none of it** — no
`/amdp/debugger/main`, no `/datapreview/amdp`. That release also lacks
`/sap/bc/adt/debugger/stack` and the runtime-dump detail resource, so the
pattern is consistent.

Do not trust the discovery document as a list of what exists: on 7.50
`breakpoints/vit` answers 200 without being advertised, `applicationlog/objects`
is advertised and answers 400, and the dump resources are advertised
nowhere at all. Ask.

## The old route

`internal/mcp/handlers_amdp.go` still drives AMDP through a WebSocket to
`ZCL_VSP_AMDP_SERVICE`. Its breakpoints have never been observed to fire.
The ADT route is tried first now and both are listed in the help, marked.
Deciding its fate is open — it is the last thing keeping that Z class
alive for AMDP.

## Rules that are not optional here

**Never provoke a failure with a real user and a wrong password.** An
agent locked the developer account for a day today doing exactly that.
See the section in `CLAUDE.md`; a nonexistent user is safe, and a
client-side refusal is safer still.

**Only `a4h` may be named in tracked files.** Not a hostname, not a real
logon name, not another system's SID. Recordings are scrubbed on the way
to disk and a test re-checks the committed ones.

**Say what you did not verify.** Half of today's findings were things
that looked like they worked. An unverified fix, labelled, is worth more
than one that looks checked.
