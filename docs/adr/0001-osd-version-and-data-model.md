# ADR 0001 — OSD version & data model: git-native, no object registry

**Status:** Proposed — transpiler agreed (its two amendments incorporated: generation-counter liveness id in point 4, schema-drift rule in point 7); awaiting open-steamgate
**Date:** 2026-09-13
**Deciders:** Alice; vsp (client), open-steamgate (façade/serving), transpiler (store/runtime)
**Context repos:** open-steamgate (OSD), vsp (the ADT client)

## Context

OSD (the Off-Stack Doppelgänger) has no SAP database of objects. Its objects
**are files in a git working tree**:

- `src/` — ABAP source (`*.clas.abap` + `*.xml`)
- `output/` — transpiled JS
- `data/` — seed rows (`*.tabu.json`); live rows in the DB (`STG_DB_PATH`)

A write through the ADT façade (vsp `source write`, or any ADT client) **edits
those working-tree files**: PUT lands in `src/` immediately, activation
transpiles into `output/` and synchronously recycles the serving runtime. The
serving unit is `(source tree, port, database)`.

There is a standing temptation to track per-object **versions/provenance at
runtime** — a UUID or version register saying "this activated object came from
version N of branch Y", with operations to promote/reconcile versions between
instances. That path **reproduces SAP's Transport-Request pain**: version
divergence, merge-by-copy, "object locked in another request", activation
overtakers, and the "which version is where" problem. Transports are a homegrown
versioning layer that is *worse than git*; re-deriving one on JS would be a
self-inflicted TR.

## Decision

1. **Git is the sole version / branch / provenance layer.** Objects are files;
   versioning is git commits and branches; combining work is `git merge` /
   `rebase` (real 3-way, with history); provenance is `git log` / `git blame`.
   **No bespoke per-object version register.**

2. **The experiment/instance unit is `(git worktree/branch [source], DB file
   [data], port [serving])`.** Parallel experiments are multiple such units.
   Already first-class as of open-steamgate 17e14c2. Example:
   ```
   main   → worktree/   → :8099 → osd.sqlite
   exp-A  → worktree-A/ → :8101 → osd-A.sqlite
   exp-B  → worktree-B/ → :8102 → osd-B.sqlite
   ```

3. **"Deploy" has two separate meanings; keep them separate:**
   - **Into OSD** (edit-and-run): ADT write → working-tree files → **synchronous
     activate** (transpile + recycle, empty-200 = true receipt) → live.
     `git commit` is an optional checkpoint. No UUID, no register.
   - **Out to a real SAP system** (the last mile): an **abapGit archive from a
     chosen git ref** → carried by vsp → the real system. **One transport at the
     boundary, not N.** abapGit is already git-shaped — anti-TR by design.

4. **The runtime liveness id is a generation counter, not a hash.** An integer
   that increments within one supervisor and resets to zero on restart, exposed
   in `/osd/serving` beside `root` and `database`. It is per-instance and
   **meaningless across instances by design**: a counter cannot be compared
   between processes, so no sentence like "instance B should be at generation 7"
   even parses. (A content/commit hash is deliberately *avoided* here — the
   moment two instances can compare hashes, someone asks whether B is at A's
   version, and that question is the first step of the register we refuse.) If a
   content/commit identity is ever wanted it is **derived from git at read time
   and never stored**. **Red line: never build a "move version X from instance A
   to instance B" operation** — that *is* a transport. Cross-instance combining
   is `git merge` + re-`deserialize`; shipping is the abapGit archive.

5. **OSD must run from a dedicated worktree, never a human's main checkout —
   DECISION, not yet implemented.** *Current state (2026-09-13): OSD runs from a
   human's checkout; ADT writes land in that checkout's tracked `src/` (a new
   object from an IDE lands in `src/osd/`), so edits accumulate uncommitted in
   the working tree an operator is using.* The decision: OSD should run from a
   **dedicated worktree**, so every ADT edit dirties the working tree of an
   isolatable, revertible branch (`git checkout .` / commit) rather than an
   operator's main checkout. Until that lands, treat this isolation as a target,
   not a guarantee — do not assume it exists. **`/osd/serving` answers with the
   tree and database file the instance is actually using**, so which checkout an
   instance serves is at least verifiable, not assumed (a real bug caught today:
   two trees served, the address looked right, the code behind it was not).

6. **Data (DB rows) is per-instance and persisted (`STG_DB_PATH`), separate from
   source (git).** A recycle must not eat a client's rows; source and data
   version on different axes (git vs the DB file).

7. **Schema drift between the two axes is detected, never pretended.** A DB file
   carries a **fingerprint of the schema it was created from**. A runtime that
   opens a file whose fingerprint does not match the schema its current code
   would generate must **say so and not serve the old schema** — otherwise
   "separate axes" (point 6) becomes a lie: a DB seeded from branch A's DDIC,
   opened by branch B whose tables differ, would silently serve rows the current
   code does not describe (reachable today by the very worktree-per-experiment
   workflow this ADR recommends). The fingerprint is a **compatibility check
   only** — never promotable, never compared between instances — and the answer
   to a mismatch is always to **rebuild local data**, never to move data from
   elsewhere. **Policy (default, Alice's to flip):** on mismatch, **report
   loudly and rebuild clean** (reseed for the current schema), since mismatched
   rows are incompatible with the current code anyway; an opt-out **refuses and
   lets the human choose** when the data is worth inspecting. Never silent either
   way — never silently serve the old schema, never silently wipe.

   **Mode now: ephemeral is the operational default.** Persistence is opt-in via
   `STG_DB_PATH`; with no path an instance boots fresh and reseeds every time, so
   reconciliation never arises (no file to check). Right for the
   build/experiment phase — no precious data, and seed regenerates from
   versioned `data/*.tabu.json`. Turn persistence (and therefore reconciliation)
   on only when runtime data is worth surviving a restart.

   **Build for extension — the one thing to get right now: store the schema
   DESCRIPTOR, not just a hash.** The file records the schema it was built for as
   a normalized descriptor (tables → columns, types, keys); its hash is the fast
   match check. Today's strategies need only match/mismatch, but a bare hash
   cannot tell migration *what* changed, which forces rework later; storing the
   descriptor makes the diff (added/dropped/type-changed) available for free.

   **Reconciliation is a pluggable strategy**, chosen by config: `reseed`
   (default) and `strict` (refuse; `STG_DB_STRICT`) now; `migrate` later — the
   same interface, added as one strategy over the already-available descriptor
   diff. Migration, when built: **additive auto** (add / drop column), **type
   change fails loud** (never a silent auto-convert), always **local** (evolve
   this file's data in place; never import from another instance or version —
   the anti-TR red line).

8. **A client must not assume a browsable object is runnable.** Because objects
   *are* files in a tree (the premise of point 1), an object can be present,
   readable, and navigable over ADT while having no transpiled module behind it
   — readable forever, never executable, and nothing in the read says so. This
   is not hypothetical: OSD's browse tree carries 466 abapGit classes in exactly
   that state, and the one opened by hand (`zcl_abapgit_apack_helper`) has no
   `output/` module. The two states are defined on the OSD side in
   open-steamgate's `docs/adt-facade.md` ("Browsing an object and running it are
   two different states", `2392ec6`); **this point is the client-side obligation
   that follows from it** — vsp must treat "listed in the tree" and "has a
   runnable module" as separate, separately-checkable facts, and must never
   report the first as evidence of the second. A green read is not a green run.
   Where the distinction matters to a caller, ask OSD which it is rather than
   inferring it from a successful GET.

   *Why it belongs in this ADR:* it is the cost of the git-native premise. A real
   SAP system has no such gap — an object in the repository is an object the
   kernel can run — so this is a state that only exists because the store is a
   file tree, and it is the one place where "objects are files" leaks into what a
   client may conclude.

## Consequences

- Merge, history, rebase, blame come for free from git; none of the TR-class
  pain is built, because it is not built at all.
- Parallel experiments = more (worktree, port, DB) triples; no shared mutable
  state, isolation by construction (a broken experiment can't touch another).
- Richer provenance, if ever wanted ("running object ← commit X of branch Y"),
  is **derivable from git at any time** (blame; the abapGit archive carries the
  commit) — a derivation, not a schema change. The design stays open.
- Cost: a **recycle within an instance is cheap** — the serving runtime does not
  parse (0.7–0.9s boot); only the façade parses (3.9s), which is why only the
  serving half is recycled. N parallel experiments on N *different* branches
  still cost N façade parses (each parses its own branch); optimization (later):
  share the immutable library parse (open-abap-core, gateway) and re-parse only
  the experiment delta.
- Discipline required: run OSD from a worktree; `git commit` to checkpoint.

## Alternatives rejected

- **Per-object UUID / version register with cross-instance promotion.**
  Reproduces Transport Requests — the exact pain OSD exists to escape. Rejected.
- **One shared instance with branch-switching.** Different branches = different
  code and data; a single running module graph and DB cannot represent several
  at once, and isolation is weak (one crash/mutation affects all). Rejected in
  favour of instance-per-worktree.

## One-line summary

OSD objects are files in a git tree; the ADT façade edits them; **git is the
only version/branch/provenance layer**; an experiment is `(worktree, port, DB)`;
the boundary to a real system is an abapGit archive. The TR problem is not
solved — it is **never built**, because git already does the hard part.
