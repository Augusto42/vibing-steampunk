# The AMDP debugger is a native ADT API

**Date:** 2026-08-23
**Subject:** AMDP debugging without ZCL_VSP_AMDP_SERVICE

## What this changes

Every previous attempt at AMDP debugging here went through ABAP we
installed: `ZCL_VSP_AMDP_SERVICE` calling `CL_AMDP_DBG_MAIN` and
`CL_AMDP_DBG_CONTROL`, driven over the APC WebSocket, with the
conclusion that breakpoints are set without error and then never fire
([2025-12-22-001](2025-12-22-001-amdp-debugging-investigation.md)).

ADT exposes the whole thing itself. No Z code, no WebSocket, no APC —
the same shape as the ABAP debugger, where "REST breakpoints 403 on
newer SAP" turned out to be the stateless client rather than the
release.

## The API, as the system describes it

All of it is in the discovery document as template links, which means it
does not have to be guessed:

| relation | template |
|---|---|
| start | `POST /sap/bc/adt/amdp/debugger/main{?stopExisting,requestUser,cascadeMode}` |
| breakpoints | `/main/{mainId}/breakpoints` |
| breakpoints/llang | `/main/{mainId}/breakpoints` |
| breakpoints/tablefunctions | `/main/{mainId}/breakpoints` |
| resume | `/main/{mainId}` |
| debuggee | `/main/{mainId}/debuggees/{debuggeeId}` |
| step/over | `/main/{mainId}/debuggees/{debuggeeId}?step=over` |
| step/continue | `/main/{mainId}/debuggees/{debuggeeId}?step=continue` |
| vars | `/main/{mainId}/debuggees/{debuggeeId}/variables/{varname}{?offset,length}` |
| setvars | `…/variables/{varname}{?setNull}` |
| lookup | `/main/{mainId}/debuggees/{debuggeeId}/lookup{?name}` |
| terminate | `DELETE /main/{mainId}{?hardStop}` |

Table-valued variables come back through data preview rather than
through the debugger:
`/sap/bc/adt/datapreview/amdpdebugger{?rowNumber,colNumber,sessionId,debuggerId,debuggeeId,variableName,schema,provideRowId,action}`,
with a `cellsubstring` variant for long values. So an AMDP intermediate
table is read the same way a table is read anywhere else.

## What was verified

Against a live 7.58 system, over plain HTTPS on one stateful ADT
session:

1. **The session starts.**
   `POST /sap/bc/adt/amdp/debugger/main?requestUser=…&stopExisting=true`
   answers 200 with
   `application/vnd.sap.adt.amdp.dbg.startmain.v1+xml`, carrying one
   parameter: `HANA_SESSION_ID`, of the form `host:port:session`.

   That is the bridge the earlier investigation listed as a possible
   missing piece ("HANA debugger not connected"). ADT establishes it.

2. **`HANA_SESSION_ID` is *not* the `mainId`, and the session does not
   outlive its connection.** An earlier version of this report said both,
   and both were wrong. The reasoning that produced them is worth keeping
   as a warning:

   - The start response carries `HANA_SESSION_ID`, so I took it for the
     id. `CL_AMDP_DBG_ADT_RES_MAIN` sets `me->main_id =
     …-start-debugger_id` and `me->session_id = …-db_dbg_session_id`.
     Two different fields; the body returns the second. The first comes
     back in the **`Location` header** —
     `response->set_location( |/sap/bc/adt/amdp/debugger/main/{ me->main_id }| )`
     — which the `adt` passthrough was not printing. A probe that shows
     only the body looks like it got an answer while missing the answer.
     Fixed: the passthrough now prints Location, Content-Location and
     ETag.
   - Asking a *second* connection for `/main/{that id}/breakpoints`
     answered 405 rather than 404, which I read as "the id was
     recognised". It was not: 405 comes from routing, before any session
     state is consulted. Worse, the breakpoint POST then answered **200
     for a `mainId` of `DUMMY`** — that handler never checks the id, it
     uses its own. Two successes in a row, neither meaning what it
     appeared to.

   The session state is `class-data` on the resource class —
   `debugger_main`, `debugger_control`, `main_id`, `session_id` — which
   is ABAP session memory. So the whole AMDP choreography has to run on
   one held stateful session, exactly like the ABAP debugger. `GET
   /main/{mainId}` does check: `if l_main_id <> me->main_id or
   me->debugger_main is not bound. raise…`

3. **The breakpoint document's shape**, which the server dictates one
   step at a time if you let it:

   - media type `application/vnd.sap.adt.amdp.dbg.bpsync.v1+xml`
     (announced by the 415 for any other type)
   - root element `{http://www.sap.com/adt/amdp/debugger}breakpointsSyncRequest`
   - required attribute `syncMode` (`full` and `delta` both accepted)
   - required child element `breakpoints`

   "bpsync" is `IF_AMDP_DBG_CONTROL->sync_breakpoints` behind a
   resource, so the ADT path and the Z path reach the same ABAP.

## It works

An AMDP breakpoint fires. Verified on a live 7.58 system, over plain
HTTPS, with nothing installed on the server:

```
vsp adt debug -s a4h --user <you> -c "astart; abp ZCL_VSP_00_AMDP_TEST 41; aresume 12; astop"
# and, while it waits, from anywhere:
vsp execute -s a4h 'zcl_vsp_00_amdp_test=>calculate_squares( ... ).'
```

SAP answers `kind="ON_BREAK"` with a debuggee id and the position:
`procedureName="ZCL_VSP_00_AMDP_TEST=>CALCULATE_SQUARES"`,
`adtcore:uri=".../source/main#start=41"`. The debugger is stopped inside
the SQLScript.

### The trap that made this look impossible

Answers arrive as a **queue**, and acknowledgements sit at its head. The
first resume after setting breakpoints returns `SYNC_BREAKPOINTS`; the
next returns `ON_TOGGLE_BREAKPOINTS`, which carries SAP's verdict on each
breakpoint — `state="VALID"`, and a reason when it refuses one. Neither
is a stop.

A client that resumes once, sees an acknowledgement and stops looking
concludes the breakpoint never fired — while the debuggee is, at that
moment, blocked on it. That is the shape of the conclusion this project
held for months, and it was reached again here before the queue was
understood: the first attempt returned `SYNC_BREAKPOINTS` and looked like
a failure, and the *evidence* that it had worked was that the triggering
session hung on a timeout.

`AMDPAwaitStop` waits past acknowledgements and keeps the verdict rather
than skipping it unseen, because "SAP calls the breakpoint VALID" is the
most useful thing the API says before it stops.

## What is still not done

Stepping, variables and the debuggee resources are mapped but unused —
`step=over`, `step=continue`, `variables/{varname}`, `lookup`, and table
variables through `/sap/bc/adt/datapreview/amdpdebugger`. Reading a
variable inside a stopped SQLScript is the next thing worth having.

Nothing is exposed through MCP yet, so an agent cannot do any of this.

## The document, in full

```xml
<amdpdbg:breakpointsSyncRequest
    xmlns:amdpdbg="http://www.sap.com/adt/amdp/debugger"
    xmlns:adtcore="http://www.sap.com/adt/core"
    amdpdbg:syncMode="FULL">          <!-- FULL or PROGRAM, upper case -->
  <amdpdbg:breakpoints>
    <amdpdbg:breakpoint
        amdpdbg:clientId="vsp-1"
        adtcore:uri="/sap/bc/adt/oo/classes/zcl_x/source/main#start=41"
        adtcore:name="ZCL_X" adtcore:type="CLAS/OC"/>
  </amdpdbg:breakpoints>
</amdpdbg:breakpointsSyncRequest>
```

Media type `application/vnd.sap.adt.amdp.dbg.bpsync.v1+xml`. The position
is a plain `adtcore` object reference — the same shape used everywhere
else in ADT — which is why guessing `amdpdbg:uri` got nowhere.

This was not guessed either. `CL_AMDP_DBG_ADT_RES_BPS` names its
transformation, `amdp_dbg_adt_sync_bp_req`, and transformations are
readable over ADT at
`/sap/bc/adt/xslt/transformations/{name}/source/main`. The template
states every element and attribute. Two lines of that class also settle
the sync mode: `c_syncmode_program value 'PROGRAM'`, `c_syncmode_full
value 'FULL'` — and nothing else is accepted, which the server reports as
`INVALID SYNCMODE` in the exception's `subType`, while its `message`
stays a useless "An exception was raised".

**Read the handler.** Three rounds of guessing cost more than one read of
the class that parses the document.

## Releases

| | 7.50 | 7.57 | 7.58 |
|---|---|---|---|
| `/sap/bc/adt/amdp/debugger/main` | **no** | yes | yes |
| `/sap/bc/adt/datapreview/amdp` | **no** | yes | yes |
| `/sap/bc/adt/datapreview/amdpdebugger` | **no** | yes | yes |

7.50 advertises none of it. That release also lacks
`/sap/bc/adt/debugger/stack` and the runtime dump detail resource, so
the pattern is consistent: the older release has the feature and not the
modern resource for it.

## What to do with ZCL_VSP_AMDP_SERVICE

Not yet delete it — nothing is proven to work end to end on either path.
But it should stop being the assumed route. If the native API turns out
to fire breakpoints, the Z service and its WebSocket protocol become
dead weight of exactly the kind this project keeps finding and removing.

The lesson repeats for the third time today: **ask the system what it
offers before building something to compensate for what you assume it
does not.**
