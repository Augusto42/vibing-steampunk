# ZADT_DEBUG — the ABAP debugger over classic RFC

This is the server side of the RFC debugger leg. It replaces nothing: `ZADT_VSP`
(the APC WebSocket) keeps working. What changes is the transport — a pinned
classic-RFC conversation instead of a WebSocket — and with it the reason the
shipped debug loop was unreliable.

The diagnosis is in [`docs/design/rfc-debugger-feasibility.md`](../../../docs/design/rfc-debugger-feasibility.md).
In one line: `attach_debuggee( )` hands back an **object reference**, and every
subsequent operation hangs off it, so attach and step must happen in the same
ABAP roll area. The WebSocket exists only to provide that roll area. A pinned
RFC conversation provides it natively — already proven on this landscape, by
holding an enqueue lock across two calls on one `rfc.Client.Pin` session.

## What is here

| Object | Kind | Purpose |
|---|---|---|
| `ZCL_ADT_DEBUG` | class | all the logic; session state lives in `CLASS-DATA` |
| `ZADT_DEBUG_STATE` | FM (RFC) | roll-area probe: same `roll` + rising `calls` proves the session held |
| `ZADT_DEBUG_BP_SET` / `_BP_LIST` / `_BP_DEL` | FM (RFC) | external breakpoints — **stateless**, no debuggee needed |
| `ZADT_DEBUG_LISTEN` | FM (RFC) | blocking listen, returns the waiting debuggees |
| `ZADT_DEBUG_ATTACH` / `_STEP` / `_STACK` / `_DETACH` | FM (RFC) | the session-bound half |
| `ZADT_DEBUG_LOOP` | FM (existing) | left as it is — it is the debuggee to aim a breakpoint at |

Interfaces are split on purpose: **typed scalars in, one JSON string out**. Nothing
on the ABAP side parses JSON (`ZADT_VSP`'s regex JSON parser is the component this
avoids), and no DDIC structure has to be created for every payload shape —
`/UI2/CL_JSON` serialises the TPDAPI tables as they are.

No module raises an RFC exception: an exception discards the exporting parameters
and with them the message. Failures come back as `E_RC = 4` plus `E_MESSAGE`.

## Deploying

The function group `ZADT_DEBUG` and the package `$ZADT_DEBUG` already exist on A4H;
`ZADT_DEBUG_LOOP` is in the group. So the deployment is: create the class, then add
nine function modules to the existing group.

1. `ZCL_ADT_DEBUG` — global class, package `$ZADT_DEBUG`, source from
   `zcl_adt_debug.clas.abap`.
2. Nine function modules in group `ZADT_DEBUG`, each flagged **Remote-Enabled
   Module**, with the interfaces given in the header comments of
   `zadt_debug.fugr.abap` and the body from the same file.

Local (`$`) package, so no transport is involved.

Check it worked without a debuggee:

```sh
vsp rfc call ZADT_DEBUG_STATE          # -> {"roll":"…","calls":1,…,"available":true}
vsp rfc call ZADT_DEBUG_BP_SET '{"I_PROGRAM":"SAPLZADT_DEBUG","I_LINE":10}'
vsp rfc call ZADT_DEBUG_BP_LIST
```

`ZADT_DEBUG_STATE` called twice through the pool returns two different `roll`
values and `calls = 1` both times — that is the pool doing its job, not a bug.
On a pinned session the `roll` repeats and `calls` climbs.

## Authorizations

The probing user on A4H is `SAP_ALL`, so nothing here demonstrates a
least-privilege setup. A real user needs `S_DEVELOP` with `ACTVT = 03` on
`OBJTYPE = DEBUG`, and — to debug *another* user's session — the external
debugging authority for that user. Worth an authorization trace before this is
promised to anyone.

## What is deliberately missing

`ZADT_DEBUG_VARS`. The variable model (`IF_TPDAPI_DATA_{SIMPLE,STRUC,TABLE,OBJREF}`)
needs a typed walk to be worth anything, and shipping the eight hard-coded `SY-*`
fields `ZADT_VSP` returns today would be worse than shipping nothing. It comes
once the loop above is verified against a live debuggee.
