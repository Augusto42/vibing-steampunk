# The ADT surface vsp calls — Wave 0–4 contract

The finite list of ADT resources and verbs vsp's tools actually call, verified
from `pkg/adt/`. It exists so an ADT façade — the **Off-Stack Doppelgänger
(OSD)**, a local ABAP system with no SAP behind it — can be built against a
known target: each wave ends when vsp's tools for that wave go green against
`localhost`. The clients this surface must satisfy are **our own** — vsp first,
adt-fs alongside it (its surface folds into the same contract). Eclipse is
deliberately **out of scope for now** (a later tier); we build the responding
side for vsp/adt-fs first.

The **complete** inventory of every ADT endpoint vsp touches — not just the wave
subset — is the appendix at the end, marked in-scope vs undiscovered.

Object names here are synthetic (`zcl_demo…`) per the repo sanitize policy;
no live identifiers.

Verbs and paths are what the client sends; `{name}` is the object, URL-encoded
(a namespaced name arrives as `/programs/programs/%2Fdemo%2Fzreport`).

---

## Wave 0 — handshake

| purpose | method | path | headers sent | required back |
|---|---|---|---|---|
| CSRF + logon | **HEAD** (→ **GET** fallback) | `/sap/bc/adt/core/discovery` | `X-CSRF-Token: fetch` [+ `X-sap-adt-sessiontype: stateful`] | `X-CSRF-Token: <opaque, not "Required">` + `Set-Cookie` (`sap-contextid`, `SAP_SESSIONID`) |
| discovery | GET | `/sap/bc/adt/core/discovery` | `Accept: application/atomsvc+xml` or `*/*` | Atom service document listing the resources that exist |

**Expiry-by-shape — the #1 thing that breaks a façade first.** vsp decides
"logged out" when a 200 *redirected away* or *lacks* `X-CSRF-Token`. So the
façade must never 302 an authenticated request, must keep emitting the token,
and must keep the session cookie stable. `sap-contextid=` (empty) forces a fresh
context.

---

## Wave 1 — read a repository

| tool | method | path | Accept | response |
|---|---|---|---|---|
| GetSource class | GET | `/sap/bc/adt/oo/classes/{n}/source/main` | `text/plain` | raw ABAP |
| GetSource interface | GET | `/sap/bc/adt/oo/interfaces/{n}/source/main` | `text/plain` | raw ABAP |
| GetSource program | GET | `/sap/bc/adt/programs/programs/{n}/source/main` | `text/plain` | raw ABAP |
| GetSource include | GET | `/sap/bc/adt/programs/includes/{n}/source/main` | `text/plain` | raw ABAP |
| GetSource FM | GET | `/sap/bc/adt/functions/groups/{g}/fmodules/{fm}/source/main` | `text/plain` | raw ABAP |
| object structure | GET | `/sap/bc/adt/oo/classes/{n}/objectstructure` | `application/vnd.sap.adt.objectstructure.v2+xml` | methods/includes (resolves `--method`/`--include` reads; plain source read does NOT need it) |
| GetPackage meta | GET | `/sap/bc/adt/packages/{n}` | `application/vnd.sap.adt.packages.v1+xml` | package XML |
| package contents | POST | `/sap/bc/adt/repository/nodestructure` (parent_name/parent_type) | nodestructure XML | tree |
| SearchObject | GET | `/sap/bc/adt/repository/informationsystem/search?operation=quickSearch&query=&maxResults=` | search XML / `*/*` | result set |
| usageReferences (xrefs / who-calls) | POST | `/sap/bc/adt/repository/informationsystem/usageReferences?uri=` | xref XML | referencing objects |

Grep rides on the reads (fetch sources, match client-side) — no extra endpoint.
The `#fragment` on source URLs (`#start=12,4`, `#type=CLAS%2FOM;name=RUN`) is
client-side; the server never sees it.

---

## Wave 2 — read data & DDIC

| tool | method | path | note |
|---|---|---|---|
| GetTableContents / freestyle SQL | POST | `/sap/bc/adt/datapreview/freestyle` | SQL text in body, row-limit param; rows back |
| table-scoped preview | POST | `/sap/bc/adt/datapreview/ddic` | table name; rows back |
| table def | GET | `/sap/bc/adt/ddic/tables/{n}` | DDIC structure |
| view def | GET | `/sap/bc/adt/ddic/views/{n}` | |
| structure def | GET | `/sap/bc/adt/ddic/structures/{n}` | |
| data element | GET | `/sap/bc/adt/ddic/dataelements/{n}` | |
| CDS DDL source | GET | `/sap/bc/adt/ddic/ddl/sources/{n}` | |
| service definition | GET | `/sap/bc/adt/ddic/srvd/sources/{n}` | |

For LSD the data rows come from the local store (SQLite); the DDIC defs come
from the DDIC the runtime already carries.

### Cross-reference & load tables — derive them, don't hand-build them

vsp's graph tools (`loads`, who-calls, `references`, boundaries) don't use a
special endpoint — they read SAP's own xref tables over **freestyle SQL**
(`/sap/bc/adt/datapreview/freestyle`). So if LSD populates these tables, the
whole graph layer works locally with no extra façade code:

| table | read by | note |
|---|---|---|
| `CROSS` | who-calls, references | classic cross-reference |
| `WBCROSSGT` / `WBCROSSGTX` | who-calls, references, boundaries | GT = long names |
| `D010INC` | `vsp loads` | the compile-time *load* graph |

These are **derivable from the parse/transpile LSD already does** — the AST
knows what references what and what loads what, so the rows are generated, not
authored. Row IDs/keys can either mirror a real system's format or be whatever
is convenient locally; vsp reads them by SQL and does not care which.

---

## Wave 3 — the development loop (write / activate / check / test)

| step | method | path | note |
|---|---|---|---|
| lock | POST | `<object-uri>?_action=LOCK&accessMode=MODIFY` | returns a **lock handle** (synthetic is fine) |
| write source | PUT | `<object-uri>/source/main?lockHandle=…` | body = new ABAP; on an **affine (stateful) session** |
| create object | POST | `/sap/bc/adt/<collection>` | object XML in body |
| unlock | POST | `<object-uri>?_action=UNLOCK&lockHandle=…` | |
| syntax check | POST | `/sap/bc/adt/checkruns?reporters=abapCheckRun` | → façade maps to **abaplint** output |
| activate | POST | `/sap/bc/adt/activation?method=activate&preauditRequested=true` | → façade maps to a **transpile succeeding** |
| inactive list | GET | `/sap/bc/adt/activation/inactiveobjects` | |
| ABAP Unit | POST | `/sap/bc/adt/abapunit/testruns` | → façade maps to a **runtime test run** |

The write path is lock → write → activate on one **stateful/affine session**
(session affinity is real; a stateless hop between lock and write retires the
handle). Synthetic locks/transports are fine as long as the session is affine.

---

## Wave 4 — extras cheap on a local system

| tool | method | path | note |
|---|---|---|---|
| dumps feed | GET | `/sap/bc/adt/runtime/dumps` | list of runtime errors |
| dump detail | GET | `/sap/bc/adt/runtime/dumps/{id}/…` | the ST22 formatted document — a runtime exception in the engine can be **emitted ST22-shaped**, and `vsp dumps --explain` reads it |
| revisions | GET | (revisions resource) | version list + a version's source — if reading from git is enough, that's the source |

---

## Object types the façade must serve (Tier-1 finite list)

From the paths above: **CLAS** (+ its includes/methods via object structure),
**INTF**, **PROG**, **INCL**, **FUGR**/**FUNC**, **DEVC** (package),
**TABL**, **VIEW**, **STRU**, **DTEL**, **DDLS** (CDS), **SRVD** (service def).
That is the breadth Tier 1 has to answer for — *not* the dependency-closure of
a running service (that belongs to the OData-runtime product, a different axis).

## Cross-cutting gotchas

- CSRF token must not be the literal `Required` (vsp reads that as "no token").
- Support **HEAD** on `/core/discovery` (vsp tries HEAD before GET).
- Never redirect an authenticated request; always return `X-CSRF-Token`.
- Namespaced names arrive URL-encoded (`%2F` = `/`).
- Source is `text/plain` raw ABAP; structure/package/search/DDIC are the
  `application/vnd.sap.adt.*+xml` shapes above.
- Thread the `lockHandle` from LOCK through the PUT to UNLOCK on one session.

Exact request bodies, query params, and response document shapes are pinned in
sanitized fixtures (from vsp's `httptest`-based tests, scrubbed of live
identifiers) supplied wave by wave.

---

## Appendix — the complete ADT surface vsp touches

Every `/sap/bc/adt/*` endpoint family the library calls (grepped from
`pkg/adt`, `internal/mcp`, `cmd`). The waves above are the **in-scope subset**
that makes vsp's dev-loop work against OSD; the rest stay **undiscovered** (so
`compat`/`sweep` see the truth) until a later tier.

**IN SCOPE — build the response (this is "make vsp work"):**
- `core/discovery`, `discovery` — handshake/discovery
- `programs/programs`, `programs/includes`, `oo/classes`, `oo/interfaces`, `functions/groups` (+ `/fmodules`), `messageclass` — object read/write/structure
- `textelements/programs`, `abapsource/prettyprinter/settings` — text pool, pretty-print settings
- `packages`, `repository/nodestructure` — package + tree
- `repository/informationsystem/search`, `.../usageReferences`, `.../objectreferences` — search + xrefs
- `datapreview/freestyle`, `datapreview` — data / freestyle SQL (also serves `CROSS`/`WBCROSSGT`/`WBCROSSGTX`/`D010INC` — see Wave 2)
- `ddic/tables`, `ddic/dataelements`, `ddic/ddl/sources` (CDS), `ddic/srvd/sources` (service def) — DDIC
- `checkruns` (→abaplint), `activation` (→transpile), `abapunit/testruns` (→runtime) — dev loop
- `runtime/dumps`, `runtime/dump`, `vit/runtime/dumps` — dumps (ST22-shaped)

**UNDISCOVERED — later tiers / not for OSD-speaks-ADT v1:**
- `cts/transportrequests`, `cts/transports`, `cts/transportchecks`, `.../searchconfiguration` — transport organiser
- `debugger/*` (`breakpoints`, `listeners`, `stack`, `conditions`), `amdp/debugger` — debugger over ADT
- `aps/cloud/iam/sia1|6|7` — identity/IAM (cloud)
- `atc/worklists`, `atc/runs`, `atc/findings` — ATC (candidate for a Tier-1.5 once dev-loop is green)
- `runtime/traces/abaptraces` — runtime trace
- `filestore/ui5` — UI5 filestore
- `enhancements/enhoxh*` — enhancement implementations (ENHO)
- `bo/behaviordefinitions`, `businessservices/bindings` — RAP BDEF/SRVB (out for classic-SEGW v1)
- `cai/callgraph` — advertised in the discovery of none of 7.50/7.57/7.58; stays undiscovered
- `vit/wb/object`, `testcodegen/dependencies/doubledata` — misc

The split is a *decision aid for the façade*, not a vsp change: OSD's discovery
advertises the in-scope set; vsp's tools for the undiscovered set correctly
report "not supported here" instead of failing.

---

## Appendix B — the dev-loop wire contract (Wave 3, state-changing)

Verified from vsp's code (crud.go, devtools.go, testing.go). The dev loop
changes state, so agree these before building. **Three cross-cutting rules bind
the whole sequence:**
- **One affine (stateful) session.** LOCK → PUT → ACTIVATE run on the *same*
  `sap-contextid` (vsp sends `X-sap-adt-sessiontype: stateful` on each). The lock
  handle is only valid inside that session; a stateless hop between them retires
  it (#88/#91). OSD must keep the context stable across the sequence.
- **CSRF token on every one** (all POST/PUT) — same rule as data preview.
- **Lock handle threads through:** LOCK issues it, PUT and UNLOCK carry it.

**LOCK** — `POST <obj-uri>?_action=LOCK&accessMode=MODIFY`
- Accept: `application/vnd.sap.as+xml;charset=UTF-8;dataname=com.sap.adt.lock.result`, stateful.
- Response: `asx:abap › asx:values › DATA` with `LOCK_HANDLE` (+ optional `CORRNR`,
  `CORRUSER`, `CORRTEXT`, `IS_LOCAL`, `IS_LINK_UP`, `MODIFICATION_SUPPORT`).
- Conflict/refusal: return an `exc:exception` document (vsp detects the
  `exc:exception` marker and surfaces `type id` + `message`, e.g. EU510).

**WRITE** — `PUT <obj-uri>/source/main?lockHandle=<h>[&corrNr=<transport>]`
- Content-Type `text/plain; charset=utf-8` (or `application/*` when the body
  starts with `<?xml`); body = new source; stateful. → OSD: file + transpile.

**UNLOCK** — `POST <obj-uri>?_action=UNLOCK&lockHandle=<h>` (stateful).

**SYNTAX CHECK** — `POST /sap/bc/adt/checkruns?reporters=abapCheckRun`
- Body: `<chkrun:checkObjectList><chkrun:checkObject adtcore:uri="<obj-uri, NO /source/main>" chkrun:version="active"><chkrun:artifacts><chkrun:artifact chkrun:contentType="text/plain; charset=utf-8" chkrun:uri="<obj-uri>/source/main"><chkrun:content>BASE64(source)</chkrun:content></chkrun:artifact></chkrun:artifacts></chkrun:checkObject></chkrun:checkObjectList>`
- **The object is identified by `chkrun:checkObject/@adtcore:uri` (a URI, not type+name), and `chkrun:content` is BASE64-encoded** (verified live — the server must decode it). `chkrun:artifact/@chkrun:uri` = obj-uri + `/source/main` (class includes are the exception, no suffix).
- Response: `chkrun:checkRunReports › checkReport › checkMessageList › checkMessage` (list wrapper, not report-at-root). → OSD: abaplint, shaped as check messages.

**ACTIVATE** — `POST /sap/bc/adt/activation?method=activate&preauditRequested=true`
- Body: `<adtcore:objectReferences><adtcore:objectReference adtcore:uri="<obj-uri>" adtcore:name="<NAME>"/></adtcore:objectReferences>`
- **Response: EMPTY body = success.** A non-empty doc with `activationExecuted="false"`
  + a `chkl:messages` list (and `inactiveObjects`) = failure; vsp reads those and
  will NOT report success on a failed activation. → OSD: transpile succeeding = empty; else the message doc.

**ABAP UNIT** — `POST /sap/bc/adt/abapunit/testruns`
- Body: `<aunit:runConfiguration><external>…</external>…<adtcore:objectReferences><adtcore:objectReference adtcore:uri="<obj-uri>"/></adtcore:objectReferences></aunit:runConfiguration>`
- Response: the aunit run result (program → testClasses → testMethods → alerts).
  → OSD: the runtime test run, shaped as an aunit result.

**Response element names vsp parses (exact — verified in testing.go / devtools.go):**
- **checkruns** (syntax): `chkrun:checkRunReports › checkReport(uri,status,reporter) › checkMessageList › checkMessage(uri, type=E|W|I, line, column, category, **shortText**)`. ⚠️ vsp's write-path parser (devtools.go) reads **`shortText` as an ATTRIBUTE** on `checkMessage` (`chkrun:shortText="…"`), NOT a child element, and takes the line from the `adtcore:uri` `#start=row,col` fragment. No findings = empty `checkMessageList`. (Real-ADT form; curl-verified against OSD.)
- **abapunit**: `aunit:runResult › program(uri,type,name) › testClasses › testClass(uri,type,name,uriType,navigationUri,durationCategory,riskLevel) › testMethods › testMethod(uri,type,name,executionTime,uriType,navigationUri,unit) › alerts › alert(kind,severity) › {title, details/detail@text, stack/stackEntry}`. A testMethod with **no `<alert>` = passed**; alerts carry the failure text.
- vsp strips the `aunit:`/`adtcore:` prefixes before unmarshalling — local names + attributes must match exactly.
