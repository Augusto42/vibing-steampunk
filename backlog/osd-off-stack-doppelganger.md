# Backlog — OSD (Off-Stack Doppelgänger)

A local, off-stack ABAP system with no SAP behind it: write ABAP → transpile →
run on `bun:sqlite` → serve OData v2 + APC → Fiori consumes it, all offline.
Output = an abapGit archive, carried into a real system by **vsp**. Shipped as a
Bun `--compile` binary, a sidecar to vsp.

Working name **OSD** (Off-Stack Doppelgänger); public alias in reserve
**MIRAGE** (Mock IWBEP·RFC·ADT Gateway Environment).

Legend: ✅ decided · ⏳ gated on Alice's go · 🔲 open · 🔗 external dep · 🌱 side-quest · 📏 measured.
Contract map for the ADT surface: [`../docs/adt-surface.md`](../docs/adt-surface.md).

> **Nothing starts until Alice says go.** This board is the settled plan, not work in progress.

---

## 0. Two products on one substrate — the clarification that sizes everything

These are separate and must not be conflated in scoping:

- **OSD speaks ADT** — a local ADT server: read / write / activate / test *objects*.
  It never *runs* a service, so it does **not** need BAPIs, utility classes, or the
  dependency-closure of a real `_DPC_EXT`. This is what the three tiers below are about.
  **Cheaper than it looks.**
- **steamgate serves OData** — the original Fiori dev-loop: actually *execute* a SEGW
  service. This needs the `/IWBEP/` Gateway runtime + the `$filter`→SELECT-OPTIONS
  bridge + the `_DPC_EXT` dependency-closure. **The expensive axis; the closure is its
  long pole, not Tier 1's.**

Both share the substrate (transpiler + runtime + `bun:sqlite` + abapGit-in-a-box).

---

## Decision gates (Alice)
- ✅ **G1 packaging:** Bun `--compile`, one exe. (goja/Go-embed = optional future.)
- ✅ **G2 boundary:** two projects (vsp | open-steamgate); the Bun exe is a *release
  artifact* of open-steamgate, not a third repo.
- ⏳ **G3 agent integration:** North Star = full **ADT façade** (vsp changes a base URL,
  zero new tools). Fallback: C-minimum (control-API + vsp local backend).
- ⏳ **G4 deploy-back route:** is ZADT_VSP-on-target acceptable? (rung 0 covers the demo
  without it.)
- 🔲 **G5 Eclipse oracle:** run Step 0 (below) to learn Eclipse's transport before Tier 2.

## Roles
- **vsp** — system I/O both ways + the round-trip oracle for the façade.
- **open-steamgate** — Bun host + ADT façade (protocol surface) + build/stitch/packaging.
- **transpiler** — object store + transpile-as-activation + abapGit-in-a-box + SEGW app
  layer + APC + the `_DPC_EXT` closure (the OData-product long pole).
- **odgp / open-rfc-go** — Tier-3 DIAG and RFC fronts (later).

---

## Tiers (the "when") × waves

### Tier 1 — OSD speaks ADT (vsp-grade)
Exit gate (two halves): **vsp's Wave 0–4 tools green against localhost** *and*
**open-steamgate's own suites unchanged** with the façade in the build. This gate is the
handoff — nothing climbs to Tier 2 until it passes.

- **Wave 0 — handshake.** Session/CSRF, `X-sap-adt-sessiontype`, `sap-contextid`,
  expiry-by-shape, `/sap/bc/adt/core/discovery` advertising only what's implemented.
- **Wave 1 — read repo.** Source of CLAS/INTF/PROG/INCL/FUGR; object structure; package;
  nodestructure; search; usageReferences.
- **Wave 2 — read data + DDIC.** `datapreview/freestyle` over the local store; DDIC defs.
  - 🌱 **xref & load tables** (`CROSS`/`WBCROSSGT`/`WBCROSSGTX`, `D010INC`): **no new
    endpoint** — read via freestyle SQL; rows **derived from transpiler's parse**, not
    authored. Populating them makes vsp's whole graph layer (loads/who-calls/references/
    boundaries) work locally for free. IDs may mirror real SAP or be convenient.
- **Wave 3 — dev loop.** lock→write(PUT)→activate on an affine session; create; unlock;
  syntax→abaplint; activation→transpile; ABAP Unit→runtime test run.
- **Wave 4 — cheap extras.** Runtime exception → **ST22-shaped dump** (so `vsp dumps
  --explain` reads local dumps); revisions from git if that's enough.
- **abapGit-in-a-box** (transpiler): run transpiled abapGit inside OSD; `deserialize` fills
  the store from any repo (open-abap-core, a corpus, ZADT_VSP). Content becomes an *input*,
  not hand-built work. `ImportSet` lands rows, `RepoSet` gives a repo back. **This is the
  Tier-1 content mechanism, not a later flourish.**
- **APC** (transpiler): a real push-channel handler runs unchanged, driven by open/message/
  drain; a Bun WebSocket wraps it in ~30 lines; ZVSP/ZADT daemons live there.
- **Never (undiscovered, so `compat` sees the truth):** transport organiser, jobs/spool,
  identity, debugger-over-ADT, real cluster dumps.

### Tier 2 — OSD speaks ADT (Eclipse-grade)
Eclipse ADT connects and doesn't scream (logon, browse the tree, read). A much bigger façade
surface than vsp exercises.
- **G5 first.** vsp-passing ≠ Eclipse-passing. Needs an Eclipse-traffic oracle.
- 📏 **Sequence** (what Eclipse requests) is capturable **with no system** — point Eclipse
  at OSD, iterate the 404s. Only the **expected responses** need a real (A4H+Eclipse)
  session. Capture → `.local/`, protocol facts only.
- 🔗 **Risk:** if Eclipse refuses to speak ADT-HTTP without an RFC/message-server "is this
  real?" handshake, OSD must pull a minimal NI/message-server front (open-rfc-go) forward
  into Tier 2. Step 0 tells us.

### Tier 3 — OSD speaks everything
Beyond ADT: RFC + DIAG fronts. A transaction/screen launch (SE16) → OSD answers a **fake
DIAG screen "not yet implemented."** Family convergence:
- **odgp** — the DIAG server that emits the fake screens (it already draws dynpros from Go).
- **open-rfc-go** — the RFC front / type-3 server (the *server* side; transpiler's rfc work
  is the *client* side).
- 🔲 SOAP: not in scope; SOAP-RFC is only vsp's gateway-closed RFC fallback. A later
  question, not an item.

---

## Eclipse oracle plan (G5)

- **Step 0 (≈10 min, before any tap):** watch ports while Eclipse connects to the A4H
  sandbox (HTTP ICM port? DIAG 32xx / gateway 33xx / message-server 36xx?), and try to
  point Eclipse at a *bare* HTTP-ADT host. This decides HTTP-only vs RFC-first, i.e. whether
  Tier 2 pulls open-rfc-go forward.
- **Tap, per outcome:** HTTP (A4H is plain-HTTP → no TLS MITM) = a logging reverse-proxy or
  Wireshark on the ICM HTTP port. NI/DIAG/RFC = open-rfc-go's existing `tap`/sniffer (JSONL).
- vsp can stand up the HTTP logging proxy on demand; parked until Tier 1 is green.

---

## External dependencies
| dep | blocks | status |
|---|---|---|
| **bun** not installed | Tier-1 build | install first |
| **open-abap-apc** not cloned | APC | absent from `.local/` clones |
| npm (build-time, pinned): @abaplint/transpiler·runtime·cli | build | fetched at build only |
| **open-abap-odata** unlicensed ("todo") | Gateway (OData product) | reimplement under MIT |
| **open-abap-core** partial stubs | closure (OData product) | verify per class |
| **open-abap-adt** interfaces, no impl | ADT façade | head start |
| **ZADT_VSP** on target | deploy rung 2 | ⏳G4 |
| bun `--compile` mainstream-only | packaging | 5 platforms, not vsp's 9 |
| Eclipse traffic oracle | Tier 2 | ⏳G5 |

## Deploy-back ladder (vsp's OUT direction)
0. ✅ demo: abapGit-on-box pulls the repo — vsp deploys nothing (off critical path).
1. 🔲 zero-code: native CLAS+DDIC + manual IWMO/IWSV registration once; IWPR only if the
   project lives on target.
2. 🔲 automation: vsp triggers abapGit pull (ZADT_VSP/abapGit API) + round-trip check. ⏳G4
3. ❌ "ADT-native IW*" is impossible — ADT has no door for IWSV/IWMO/IWPR (abapGit /
   registration-FMs only).
IN direction (seed): vsp exports DDIC (`rfc export` abapGit ZIP) + TABU (Data Config) →
files OSD loads. Two bundles: code flows OUT, data flows IN; `tabu.json` is not deployed back.

## Ports (OSD = fake instance 99)
| protocol | bind | tier |
|---|---|---|
| **ADT + OData (HTTP, plain)** | `http://localhost:8099` | Tier 1 — one ICF listener |
| DIAG | `localhost:3299` | Tier 3 (reserved) |
| Gateway / RFC | `localhost:3399` | Tier 3 (reserved) |
| message server | `localhost:3699` | Tier 3 (reserved) |

open-steamgate's own OData dev server stays `3030` (no collision). vsp's `.mcp.json`
`osd` profile points at `:8099`; it's inert until Wave 0 binds there. Confirmed with
open-steamgate (2026-09).

## 📏 Measured facts
- **Bun 1.4.2 runs the WHOLE stack with NO lowering and NO shims** (open-steamgate,
  945ef2c): transpiled runtime + gateway + SADL + CDS + media; 107 ABAP-Unit (same as
  Node) + HTTP surface answering ($metadata, reads, media $value, virtual elements, a
  published CDS service, a write); 20 reads 220 ms vs Node 264 ms (Bun slightly faster).
  → the Babel-lowering + 3-shim complexity was **goja-only**; the chosen Bun path has none.
- goja (rejected path, for the record): no async generators → embed-as-is NO; Babel-lowered
  runs 40–85× slower, OData hot-path ~40 ms. Optional far-future only.
- **Defect — Bun vs Node ESM specifier (`%23`/`#`):** Bun won't decode `%23`, Node will;
  Node refuses the literal `#` Bun accepts — symmetric, no single specifier satisfies both.
  The transpiler emits 99 hash-named files (`#ui2#…`). 20-line rename works around it; real
  fix is a Bun issue or a transpiler filename change (**transpiler's queue**). Lands on vsp
  only if vsp ever compiles the bundle. See open-steamgate `docs/bun-spike.md` + `ANORMALIES.md`.
- DB seam: 11 async methods + 7 rewrites; `bun:sqlite`/modernc = the cheap case.
- abapGit archive = standard (13 files), vsp reads it without a mapping step.
- Eclipse: ADT payload is HTTP by design; RFC seen on connect is likely SAP-Logon system
  discovery, not the ADT traffic — Step 0 confirms (Eclipse tier is parked for now).

## vsp `.mcp.json` — `osd` profile (add at Wave-0-up)
```json
"osd": { "command": "…/vsp",
  "args": ["--url","http://localhost:3030","--user","OSD","--client","001","--language","EN","--insecure"],
  "env": {} }
```
Port/auth provisional; add when OSD answers on localhost (that's also when vsp's oracle work
begins). A dead entry only errors until then.
