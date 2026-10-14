# ZADT_DEBUG — the ABAP debugger over classic RFC

This is the server side of the RFC debugger leg. It replaces nothing: `ZADT_VSP`
(the APC WebSocket) keeps working. What changes is the transport — a pinned
classic-RFC conversation instead of a WebSocket — and with it the reason the
shipped debug loop was unreliable.

The diagnosis is in [`docs/design/rfc-debugger-feasibility.md`](../../../docs/design/rfc-debugger-feasibility.md).
In one line: `attach_debuggee( )` hands back an **object reference**, and every
subsequent operation hangs off it, so attach and step must happen in the same
ABAP roll area. The WebSocket exists only to provide that roll area. A pinned
RFC conversation provides it natively.

## What is deployed

Everything lives in function group **`ZADT_DEBUG`**, package `$ZADT_DEBUG`.

| Module | Source | Purpose |
|---|---|---|
| `ZADT_DEBUG_RFC` | `zadt_debug.fugr.zadt_debug_rfc.abap` | the whole facade: local class `lcl_dbg` over `IF_TPDAPI_*`, plus a dispatcher on `I_OP` |
| `ZADT_DEBUG_STATE` | `zadt_debug.fugr.zadt_debug_state.abap` | roll-area probe on its own module |
| `ZADT_DEBUG_LOOP` | (pre-existing) | left as it is — it is the debuggee to aim a breakpoint at |

`I_OP` is one of `state`, `bp_set`, `bp_list`, `bp_delete`, `listen`, `attach`,
`step`, `stack`, `detach`. Typed scalars go in; one JSON string comes out, so
nothing on the ABAP side parses JSON (`ZADT_VSP`'s regex JSON parser is the
component this avoids) and no DDIC structure is needed per payload shape.
`/UI2/CL_JSON` serialises the TPDAPI tables as they are.

No RFC exception is ever raised: an exception discards the exporting parameters
and with them the message. Failures come back as `E_RC = 4` plus `E_MESSAGE`.

`bp_*` work standalone. `listen`/`attach`/`step`/`stack`/`detach` only work on a
**pinned** connection (`rfc.Client.Pin`), never through the pool — a pooled call
lands in a different roll area and the session reference is gone.

## Proven end to end

On A4H, 2026-08-21, in this order:

1. `vsp rfc debug` pins one conversation; `state` twice returns the same `roll`
   with a rising `calls`, while two calls through the pool return two different
   `roll` values and `calls = 1` — the pinning is real and the pool is not a
   substitute.
2. `bp SAPLZADT_DEBUG/LZADT_DEBUGU01 9` — the breakpoint lands in
   `ABDBG_EXTDBPS` and reads back from a different session.
3. `catch 150` blocks; `ZADT_DEBUG_LOOP` is then called **over a second RFC
   connection**, stops at the breakpoint, and the listener returns it.
4. The attach reports `procname ZADT_DEBUG_LOOP`, and the stack shows the real
   RFC entry chain: `%_RFC_START` → `REMOTE_FUNCTION_CALL` → `ZADT_DEBUG_LOOP`.
5. Three `step over` walk lines 9 → 14 → 15 → 17, the stack following each one.
6. After `detach` the debuggee runs to completion — `TVARVC ZADT_DEBUG_COUNTER`
   advanced, so its `UPDATE` and `COMMIT WORK` really executed.

Two things learned there, both fixed:

- **Never hand a TPDAPI table straight to `/UI2/CL_JSON`.** Serialising the raw
  stack table hangs the call, and since the client is given a long timeout for
  the blocking listen, the caller sits out its whole RFC timeout with the
  debuggee still attached. `stack` projects five fields per frame instead.
- **`detach` kills the conversation.** `END_DEBUGGER` ends the debugger's own
  ABAP session with the debuggee's, so the transport reports
  `CM_NO_DATA_RECEIVED` with no reply to read. That is the success case; the
  driver treats a transport error on detach as "the session is gone".

## The one manual step: Remote-Enabled

`ZADT_DEBUG_RFC` has to be flagged **Remote-Enabled Module** by hand:

> SE37 → `ZADT_DEBUG_RFC` → Attributes → Processing Type → Remote-Enabled Module → activate

ADT has no way to set it. Everything else here was created and activated over
ADT; the flag lives in the function module's properties, and neither the
`fmodules` creation payload nor a source PUT touches it. `TFDIR-FMODE` shows the
result: `R` once it is set, blank until then.

Then, from a pinned session:

```sh
vsp rfc call ZADT_DEBUG_RFC '{"I_OP":"state"}'
vsp rfc call ZADT_DEBUG_RFC '{"I_OP":"bp_set","I_PROGRAM":"SAPLZADT_DEBUG","I_LINE":10}'
vsp rfc call ZADT_DEBUG_RFC '{"I_OP":"bp_list"}'
```

`state` called twice through the pool returns two different `roll` values and
`calls = 1` both times — that is the pool doing its job, not a bug. On a pinned
session the `roll` repeats and `calls` climbs.

## What deploying this taught us about the ADT path

Three findings, all reproducible on A4H (SAP_BASIS 758) through the vsp MCP server:

1. **No class can be locked.** `POST …/oo/classes/<any>?_action=LOCK` returns
   `MODIFICATION_SUPPORT=NoModification` — for a class we had just created, for
   one in `$TMP`, and for `ZCL_VSP_APC_HANDLER`. Function groups and function
   modules lock normally on the same system, same user, same session. So the
   facade became a local class inside a function module include rather than the
   global class it was written as. Worth its own issue: something about the class
   resource's lock (an ABAP language version the client has to declare?) differs
   from every other object type.
2. **Locks do not survive between MCP tool calls.** Each call gets its own
   stateful ADT session, so a `LOCK` in one call and an `UPDATE_SOURCE` in the
   next fails with `ExceptionResourceInvalidLockHandle` — even though the lock
   returns the same handle. Only the single-call workflows work: the high-level
   `edit` and `EDITSOURCE` (which does GetSource → replace → syntax check → lock
   → update → unlock → activate inside one call).
3. **`EDITSOURCE` on a function module accepts more than the FUNCTION block.**
   A `CLASS … DEFINITION`/`IMPLEMENTATION` written before `FUNCTION` compiles
   into the function pool and activates. That is what makes a self-contained
   facade in one module possible without touching the group's TOP include —
   which, incidentally, cannot be locked either (`R3TR PROG LZADT_DEBUGTOP`:
   "This syntax cannot be used for an object name").

## Authorizations

The deploying user on A4H is `SAP_ALL`, so nothing here demonstrates a
least-privilege setup. A real user needs `S_DEVELOP` with `ACTVT = 03` on
`OBJTYPE = DEBUG`, and — to debug *another* user's session — the external
debugging authority for that user. Note that SAP's own RFC-enabled debugger
modules (`TPDAPI_TEST_*`) answer a missing authorization with a silent `RETURN`
and an empty result, not an error.

## What this facade is still for (2026-08-21)

Most of it is now redundant, and that is a good outcome rather than wasted work.
SAP's own ADT resources carry the whole debug loop — breakpoints included —
over a pinned RFC conversation *and* over a stateful HTTPS session, so
`listen`, `attach`, `step` and `stack` here duplicate what
`/sap/bc/adt/debugger` already does with no Z code at all. Go stopped calling
them; the ABAP stays because removing it from a live system costs more than it
returns.

Three operations remain worth having, and only one of them is small:

- **`bp_list`** — the server's own view of the external breakpoints
  (`ABDBG_EXTDBPS`). ADT answers its breakpoint GET with 200 and an empty body,
  because in ADT the set is the IDE's state; this is the only way to see what
  the *system* holds, including breakpoints another client set.
- **`state`** — the roll-area probe. Two calls returning the same `roll` with a
  rising `calls` is what proves a connection is really pinned; nothing else
  demonstrates it.
- **`detach`** as a broom — `STOP_LISTENER_FOR_USER` clears a stale
  `ABDBG_LISTENER` row that would otherwise conflict with the next listener.
  Note what it also does: it ends external debugging for the user, and the
  external breakpoints go with it. A client that sets breakpoints and then
  detaches deletes its own work — vsp detaches only when it actually listened
  or attached, for exactly this reason.

`vars` never had to be written here: the variable model comes typed from
`/sap/bc/adt/debugger` `getVariables` / `getChildVariables`.
