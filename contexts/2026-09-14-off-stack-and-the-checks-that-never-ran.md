# Handover: the off-stack push, and a day of checks that had never run

Written at the end of a long session that ran across three repositories and two
other agent sessions. Nothing here is in flight on my side: everything of mine
is merged to `main` and released as **v2.58.0**. What is outstanding belongs to
other sessions or is deliberately backlogged, and is marked as such.

The raw transcript is `~/.claude/projects/-home-alice-dev-vsp/f67c44b2-….jsonl`
(37 MB). This file is the part worth reading.

## The thread that ties the day together

Almost every defect found today was the same shape, and it is worth naming
because it kept reappearing in places that had nothing to do with each other:

> **A check that was never run is indistinguishable from a check that passed.**

- A PR opened from a fork against `abaplint/transpiler` gets CI and *not*
  Regression — `regression.yml` triggers on `push, branches-ignore: [main]` and
  has no `pull_request` trigger at all, so the fork's push fires nothing here.
  Green PR, never regression-tested. (Lars told us the rule; the reason was
  verified in the workflow file and written into `/home/alice/dev/transpiler/CLAUDE.md`,
  untracked.)
- `vsp search --type TABL` returns zero structures with the same face it would
  use for a package that has none — the filter compares the full type code and
  drops `TABL/DS`. Backlogged, `agenda/AGENDA.md`, 2026-09-14 entry.
- `GetTypeInfo` had never returned anything to anybody: `406` on every name,
  because it asked for `application/xml`. Its twin four files away had the same
  bug found, fixed, and explained in a comment — and the twin was missed.
- OSD's `:8099` accepted TCP and answered nothing, because the test runner had
  taken the port. Every "is it up?" check says yes.
- `make build-all` builds three platforms, not nine. Following the release
  procedure as written would have shipped five binaries of the *previous*
  release under the new tag, and nothing about the files says so.

The last one is the one to keep: **it was in our own release procedure**, and it
was only found by walking the procedure rather than reading it. `/celebrate` is
fixed (`e8c5391`) — right make target, tag before build so `git describe` gives
the binaries the version they claim, and a credential check that someone will
actually read.

## What shipped

**v2.58.0 — "where the file actually ends"** (PR #224, five commits).

- **`vsp w3mi`** — the MIME repository (SMW0), byte-exact. vsp had *no* W3MI
  support at all. Most of the machinery already existed: `WWWDATA` is an
  INDX-style cluster table, so `vsp cluster read` already joined the fragments,
  decompressed LZH and parsed the rows. The missing step is the one that cannot
  be guessed — the bytes are fixed **255-byte lines** whose last line is
  zero-padded, so the cluster cannot say where the file ends; `filesize` in
  `WWWPARAMS` can. Verified on arithmetic that can fail: 205 × 255 = 52275,
  truncated to 52216, the 59 discarded bytes all zero, and then the file checked
  *itself* — a Z-machine v3 header carries its own length at `0x1A` and checksum
  at `0x1C`, both matched. Two other extractions, by a different route and by
  another session, gave the identical md5.
- **`GetTypeInfo`**, both layers. The Accept header, and the parser underneath
  it that read the type and lengths as root attributes when the document carries
  them as children of `dtel:dataElement`. Fixing only the header would have
  turned a `406` into a `200` returning `Length: 0` forever — a working-looking
  answer is worse than the honest failure it replaces. Four real documents read
  off a system rather than guessing element names; two committed **unmodified**
  under `testdata/` so a test parses the bytes a system actually sends. Both
  fixes verified by reverting them.
- **ADR 0001** — git as the sole version/branch/provenance layer for the
  off-stack system, so the Transport-Request class of problem is never built
  rather than solved. Its **point 8** is the client-side rule: *a browsable
  object is not necessarily a runnable one.* An object can be readable over ADT
  with no transpiled module behind it — OSD's tree carries 466 abapGit classes
  in exactly that state — and nothing in a successful read says so.
- **`docs/adt-surface.md`**, the discovery fixture, and the OSD backlog, all
  used daily while sitting untracked.

## The off-stack system, and who holds what

Three sessions worked in parallel. My role was the strict client — I verify over
the wire and I do not build the façade.

**Done and verified by me, not taken on trust.** An ABAP `if_http_extension`
handler, transpiled, caught a real HTTP request off-stack at `/sap/bc/zo4d_demo`
and routed per query — `?player=dev` → 40672 b "O4D DEV PLAYER" vs 9636 b "VIVID
VIBES", different md5s, so `handle_request` genuinely ran rather than a static
file being served with a lucky content type. Five repeats gave one md5, which is
what confirmed their double-mount fix. ADT and both `$metadata` unmoved.

**Waiting, and not on me.** The websocket half — 101 on upgrade, then a text
frame round-trip on `ZCL_O4D_APC_HANDLER`. open-steamgate needs a websocket
server driving `zcl_apc_host`; the transpiler's APC family is done and her
handler runs on it off-stack at ~1.4 ms a frame.

**One thing to watch when that runs.** The transpiler fixed four bugs on her
effects; three are loud and invisible from outside (nothing runs, so there is no
wire to watch). The fourth — a built-in emitted as `this.cos(…)` — compiles,
boots, and throws at runtime *inside* a method. Her handler catches broadly, so
it can answer with a frame that is short or missing a primitive, which reads as
a rendering bug rather than a runtime error. **A frame that arrives but is wrong
is that, not the handler's arithmetic.** Fixed in the linked build; if
open-steamgate has not linked it, that is the first thing to check.

**Two live findings I left for open-steamgate rather than acting on.** Their
`:8099` was hung (test runner holding the port), and there were **8 orphaned
`osd-serve.mjs` processes, 1.6 GB resident**, all reparented to `/init` — the
signature of a supervisor recycle that never reaps its predecessor. I killed
nothing: another session's process table is not mine to prune.

## Outstanding

- **The `--type` filter fix** and **SMIM**, in that order —
  `agenda/AGENDA.md`, 2026-09-14 entry, with the measurement that opens them.
- **The adversarial sweep of OSD's ICF surface.** Offered three times, never
  taken up; open-steamgate has been heads-down rather than declining. The one I
  would run first is their "a node with no handler is not servable" rule, because
  from outside *fails loudly* and *silently 404s* look identical and only one of
  them is what they built.
- **`pkg/adt` uses generic media types in ~54 places** against 28 precise
  `vnd.sap.adt.*` ones. Same looseness as the `GetTypeInfo` bug, but most of it
  **works** — real SAP accepts `application/*` — so changing 54 call sites on
  speculation would break working code to fix a proven bug in one. Left alone
  deliberately; noted in PR #224.
- **`TypeInfo.Length`/`Decimals` are now populated**, but the six DDIC types
  `if_abap_channel_types` carries that nobody has read are still absent from
  open-abap-apc, by the transpiler's choice and mine: an absent type fails
  visibly, a plausible one does not.

## Where things are

| | |
|---|---|
| Release | `v2.58.0`, nine binaries + `checksums.txt`, each reporting `v2.58.0` |
| Backlog | `agenda/AGENDA.md` — the 2026-09-14 entry |
| ADR | `docs/adr/0001-osd-version-and-data-model.md` |
| ADT contract | `docs/adt-surface.md` |
| APC source read off A4H | scratchpad, ~630 lines; the transpiler has its own copy in `open-abap-apc` |
| Transpiler repo notes | `/home/alice/dev/transpiler/CLAUDE.md` — untracked on purpose, upstream is Lars's |
| Zork export | `open-steamgate/.local/zork-abap/src/` — gitignored, and it must stay that way |

One sanitize note for whoever picks this up: `WWWPARAMS` carries a `filename`
parameter holding the path a file was uploaded *from* — a user name, a host, a
directory layout. `vsp w3mi --abapgit-dir` deliberately does not write it and a
test enforces that. It is the kind of field that looks like metadata and is an
identifier.
