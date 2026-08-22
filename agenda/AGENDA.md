# vsp agenda

The living board. One file, kept current — not a dated series. Dated analyses
live beside it as `agenda/YYYY-MM-DD-NNN-topic.md`.

Written for whoever picks the work up next, including the other agents working
on this repo from other machines. Last updated 2026-08-22 by **claude-mac-m2**.

> Sanitize policy applies here like anywhere else in the tree: no live
> hostnames, usernames, transport IDs or customer packages. Operational detail
> with real identifiers belongs under `.local/` or `.private/`.

---

## Needs a decision

**PR #152 — `fix/lock-nomodification-with-handle`.** The same fix we merged as
`583f042`, opened three weeks earlier by an outside contributor. It now
conflicts with `pkg/adt/crud.go` and `pkg/adt/crud_reconcile_test.go`. The
guard condition differs slightly: #152 fails only on `NoModification && no
handle`, ours fails on `no handle` alone (stricter). Ours additionally parses
the ADT exception body so an EU510 lock conflict reaches the caller with SAP's
own message. **Close as superseded with thanks, or adopt their narrower
condition?** Either way the contributor deserves an answer.

**Tool modes** — see [2026-08-22-001-tool-modes.md](2026-08-22-001-tool-modes.md).
Recommendation: parity test and typed per-action params first, deprecate
`focused`/`expert`, delete in the next major. Not yet decided.

**Eight unmerged branches on origin** — `one-tool-mode` (+16), `worktree-
integration-test-infra` (+8), `pr-93-fix` (+5), plus five small ones from
December–March. Needs triage: what is still alive after the rewrites since.

---

## Cross-machine coordination

**`feat/function-module-edit` (`cf39e41`) is not pushed.** It exists only on the
Windows/WSL machine. Meanwhile `583f042` landed here and covers the *create*
path for function modules end to end. Before pushing:

1. `git fetch origin && git rebase origin/main` — main has moved by four merges
   (lock fix, function modules, host sanitization, browser SSO).
2. Read `pkg/adt/workflows_function.go` — the create path is done. What is
   **not** done is **editing an existing** function module as its own API,
   which is likely where that branch's value actually is.

What we learned about the ADT contract, so it does not get rediscovered:

- The remote-enabled flag is **ignored at creation**. `POST` with
  `fmodule:processingType="rfc"` returns `201` and the module reads back as
  `"normal"`. The flag only takes on a **metadata PUT under a lock**.
- A function module's **signature lives in its source**, not its metadata.
- **Activate after UNLOCK.** Activating while holding the lock returns 403
  EU510. This is true for classes too.

---

## Queued work

**Fast-RFC serializer captures** — the original goal everything else was
clearing the way for. The `ZCL_RFC_TEST` battery is written and active, no
locks outstanding. Next: bring the RFC relay back up, drive the battery through
the sniffing destination once per SM59 serializer mode (Classic / basXML /
Force basXML / Fast), capture one file per mode. The fast-path oracle is
already captured. Operational detail is in the private session notes.

**ZADT_VSP surface** — unblocked now that vsp can create RFC-enabled function
modules without SE37:

- move the enqueue-reset report out of `$Z` into `$ZADT_VSP`
- add the remote-enabled wrapper FM (logic in a class — `ENQUE_DELETE` is not
  remote-enabled, so the reset must live ABAP-side)
- delete the stray `Z_*` report that predates the naming convention
- wire the WS command through the APC handler

**Issue #154 — namespaced function groups return HTTP 406.** Our new
`GetFunctionModule` / `CreateFunctionModule` go through the same
`GetObjectURL`, so they almost certainly share the bug. Worth fixing together.

**`delete` through MCP needs a lock handle.** It is a two-step call that leaks
internal mechanics to the caller, unlike `create`, which now does the whole
flow. Same one-call treatment would suit it.

**`gofmt -l` reports 5 files** — `cmd/abapgit-pack/main.go`,
`fun/llvm2abap_demo.go`, `internal/lsp/server_test.go`,
`internal/mcp/handlers_context.go`, `internal/mcp/handlers_graph.go`.
Long-standing, unrelated to recent work.

---

## Recently landed

- `61a0375` — `NoModification` no longer fails a MODIFY lock that came with a
  handle, and the guard no longer leaks the ENQUEUE it just took. That leak was
  the cycle where every retry hit its own orphan lock and reported it as
  "NoModification".
- `583f042` — create RFC-enabled function modules in one call; SE37 is out of
  the loop. Verified end to end: the created module answers a classic RFC call.
- `99f6896` — a live host name removed from a context table, a design note and
  three reports.
- `3bb165f` — browser SSO merged from the Windows side.

---

## Other tracks (not this repo)

Two threads run alongside and have their own private notes, deliberately not
tracked here:

- **Inter-agent IRC bus** — channel autodiscovery from the git remote is
  written and waiting on a decision to push; an April security review has six
  of seven findings still open, three of them Critical. The write-up names
  unpatched issues in a public repo and must stay private until they are fixed.
- **Storage reorganization** on the shared network volume — done, reversible
  from a manifest.
