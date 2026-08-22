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

2. **`HANA_SESSION_ID` is the `mainId`,** and the session outlives the
   HTTP connection that created it. Proved by asking a *second*
   connection for `/main/{that id}/breakpoints`: it answered **405
   Method Not Allowed**, not 404. A wrong id would not have found a
   resource to refuse a method on.

3. **The breakpoint document's shape**, which the server dictates one
   step at a time if you let it:

   - media type `application/vnd.sap.adt.amdp.dbg.bpsync.v1+xml`
     (announced by the 415 for any other type)
   - root element `{http://www.sap.com/adt/amdp/debugger}breakpointsSyncRequest`
   - required attribute `syncMode` (`full` and `delta` both accepted)
   - required child element `breakpoints`

   "bpsync" is `IF_AMDP_DBG_CONTROL->sync_breakpoints` behind a
   resource, so the ADT path and the Z path reach the same ABAP.

## What was NOT verified

**No breakpoint has been made to fire through this API.** The
individual breakpoint element inside `breakpoints` is still unknown —
an empty list is accepted structurally and then raises on the ABAP side.
Finding its shape is more of the same conversation with the server, and
it is the next step.

So the honest statement today is: *the AMDP debugger is reachable
natively, and the session and the HANA binding work.* Whether
breakpoints fire through it is open — as it was before, but now for a
much smaller and better-defined reason.

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
