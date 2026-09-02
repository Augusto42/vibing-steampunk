# A lean installer, so a system can be bootstrapped without abapGit

**Author:** wsl-claude
**Date:** 2026-08-22
**Status:** design, not started

## The problem it solves

Installing abapGit-full means creating 576 objects across two packages. Doing
that from outside, object by object over ADT, is roughly four calls each — lock,
write, unlock, activate — and it hits dependency ordering: a class that
references another not yet activated fails, and the caller has to work out the
order abapGit's own deserializer already knows.

## What is already there, and why it does not fit

`ZABAPGIT_STANDALONE` is a single object — 154,655 lines in one REPORT — and it
can already do the work. Measured in the published build: offline repositories,
ZIP import, and a deserializer. Two things stop it being the answer:

- **It is interactive.** The ZIP arrives through `cl_gui_frontend_services`, a
  SAP GUI file dialog. There is no headless entry point, and no parameter that
  takes an archive.
- **Its API is unreachable.** Every class in it is local to the report. The names
  begin with `zcl_`, which is misleading: `CLASS ... DEFINITION PUBLIC` appears
  zero times. Nothing outside the program can call them.

So the choice is not "trim abapGit down". Compactness and a reachable API trade
against each other, and 576 objects is the price of the second.

## The shape

One global class and one remote-enabled wrapper, following the convention:

```
ZCL_VSP_GIT_INSTALL     ← accepts a ZIP as xstring, installs, returns a log
ZVSP_GIT                ← function group
  └── ZVSP_GIT_INSTALL  ← remote-enabled, flat parameters
```

Callable from every transport vsp can reach: the APC handler over WebSocket,
classic RFC through the gateway, SOAP-RFC over HTTP, and the ADT tunnel. The
wrapper stays flat — the WebSocket RFC domain accepts only scalar strings, which
is why `RFC_READ_TABLE` cannot go through it, so an archive travels as a base64
string rather than as a table.

Itself installed by `vsp deploy` over ADT: two objects, no bootstrap problem.

## Scope

It deserializes, it does not serialize. That is the smaller half by a wide
margin, and it only has to handle what abapGit-full is made of — classes,
interfaces, packages, programs. Four object types, not 150.

Per object: read `.abap` and `.xml` from the archive, create or update, and
activate at the end in one pass rather than one at a time, so ordering is the
activation's problem rather than the caller's — `RS_WORKING_OBJECTS_ACTIVATE`
takes a list.

## Why it is worth doing

- **One call instead of ~2300.** The whole archive goes over in a single request.
- **Ordering is handled where the information is**, in the system.
- **It works where the system has no internet.** Standalone's normal bootstrap
  pulls from github.com, which a corporate landscape usually blocks — the reason
  an archive is handed to it instead of fetched.
- **It removes today's failure mode.** A missing abapGit currently breaks
  unrelated features: one class that references it failed to compile and took
  the whole APC application down with it, so the debugger, RFC over the tunnel
  and RunReport disappeared, and the error named a git service the caller had
  never invoked.

## Open questions

- Does activation of 576 objects in one call stay inside the work process
  timeout? If not, the pass has to be chunked, and the chunking has to respect
  dependencies.
- Package creation ahead of the objects that live in them, including transport
  attributes on a system where `$`-local is not allowed.
- What the log should carry so a failure is actionable: object, step, and SAP's
  own message rather than a boolean.
