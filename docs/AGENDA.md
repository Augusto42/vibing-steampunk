# Agenda — what to do next

A working backlog distilled from three studies done on 2026-08-20, kept short on
purpose: each line says what to do, why, and where. The studies themselves hold the
evidence — [`design/pr-issue-triage.md`](design/pr-issue-triage.md) (19 open PRs, 47
issues), [`design/fork-survey.md`](design/fork-survey.md) (107 forks), and, for the
RFC track, [`design/rfc-opportunities.md`](design/rfc-opportunities.md) and
[`design/rfc-debugger-feasibility.md`](design/rfc-debugger-feasibility.md).

Status key: **[ ]** open · **[x]** done · **[~]** in progress.

## Sprints

Work is organised into four sprints. Each has one theme, a definition of done, and
items small enough to finish; nothing moves to the next sprint until the current
one's exit criteria hold. Items marked **(maintainer)** need a human decision — an
agent must not perform them.

### Sprint 1 — Make the project reviewable (½ day) — ✅ *done*

*Theme: a contributor's patch can be judged in minutes, and the front door is locked.*

- [x] Authenticate the HTTP transport (API key + Origin validation + /health).
- [x] Gate the live `ctxcomp` tests behind `//go:build integration`.
- [x] Bump `open-rfc-go` (nested structures, pinned sessions, 32-bit builds).
- [x] `.github/workflows/ci.yml` — build, vet (both tag sets), `go test ./...`, a
      four-target cross-compile check, and golangci-lint, on PR and on push to main.
- [x] `parseActivationResult` — it understood only the wrapped checklist, so when ADT
      returns `<chkl:messages>` as the document root (the usual shape) a failed
      activation parsed to nothing and reported success. Both shapes now parse;
      regression tests cover error, warning and empty responses.
- [x] Reuse one RFC client across MCP calls (`internal/mcp/handlers_rfc.go` dialled and
      closed one per call — an RFC logon per tool call). Calls that override the
      destination still get their own; a dead shared connection is dropped so the next
      call redials. `rfc.Session` pinning stays for the stateful debugger work.

**Done when:** CI is green on `main` and reports a status on every PR; a clean
checkout passes `go test ./...`; activation failures surface as failures.

### Sprint 2 — Correctness and the backlog (2–3 days)

*Theme: the bugs that make vsp look broken on systems we do not test on.*

- [ ] Redirect and session hardening (the remaining Tier 0 items in the fork survey).
- [x] Message classes are unwritable — `#162/#159/#160/#161`: three root causes, all
      fixed. The edit switch omitted `MSAG`; `WriteMessageClassTexts` was registered
      without its `texts` argument, so the tool could not be called at all; and the
      request marshalled the read model into a bare `<MessageClass>` while ADT serves
      and expects a namespaced `<mc:messageClass>` whose messages carry `mc:msgno`
      and `mc:msgtext` as attributes (confirmed against the live system).
- [x] CSRF `HEAD` → `GET` fallback.
- [x] WebSocket path ignores `HTTP_PROXY`.
- [x] Ship `LICENSE` and `NOTICE` with the release binaries — they embed
      open-rfc-go, which is Apache-2.0, and section 4 requires the licence and
      notice to travel with a distribution. vsp stays MIT: Apache-2.0 is permissive,
      not copyleft, so depending on it does not change vsp's own licence.
- [ ] **(maintainer)** Merge train: `#128, #126, #120, #107, #149` → `#152`;
      `#125` before `#108`; `#148` before `#150`; `#145`; `#121`. Remove the stray
      closing reference to `#2` from `#106` first.
- [ ] **(maintainer)** Close with an explanation: `#151`, `#138`, `#130`, `#139`.

**Done when:** the Tier 0 list is empty, message classes round-trip, and the open-PR
count is in single digits.

### Sprint 3 — Authentication (3–5 days)

*Theme: stop being basic-auth-only; harvest what the forks already proved.*

- [ ] An auth abstraction both transports can share (`Apply(req)` for HTTP; the RFC
      side keeps its own logon), so a new method is a plug-in rather than a fork.
- [ ] macOS Keychain mTLS (`Edgars-Ralfs-Dunis`) — fix the non-darwin build first.
- [ ] Native reentrance-ticket SSO (`BurnerPat`) — replaces the chromedp path, handles MFA.
- [ ] OIDC / JWT bearer + principal propagation (`marianfoo`) — reimplement on a real
      JWT library; one key and cookie jar per session, not per request.
- [ ] OAuth2 / XSUAA / BTP destinations / Cloud Connector (`Prolls`) — fix the
      `Proxy-Authorization`-on-`CONNECT` bug before adopting.
**Acceptance criteria for anything auth-shaped**, applied to every harvested PR
before it lands. These are ordinary secure-engineering rules, but each one below
is here because a real system was found breaking it:

- [ ] Secrets never reach a rendered string. A password/token field is a type
      whose `String`/`GoString`/`MarshalText`/`MarshalJSON` are `[redacted]`,
      with an explicit `Reveal()` at the one place the protocol needs it.
      (Done for the RFC password; extend to the ADT config.)
- [ ] The issuer is **pinned**, not read out of the token being validated.
      Validating the issuer against a value taken from the unverified token
      leaves only the audience check with teeth.
- [ ] No flag, env var or config key can switch signature validation off. If a
      bypass is unavoidable for local development, it takes several
      simultaneous conditions, an allow-list, and is disabled in production
      regardless.
- [ ] Identity is derived from the authenticated session, never accepted from
      the request body.
- [ ] Credential parameter groups are all-or-nothing: the whole group or an
      explicit error, never a silent fall-through to the next source. Our own
      RFC chain (`rfc_user` → `SAP_USER` → the ADT credentials) violates this
      today and produced a logon failure that read as a wrong password.
- [ ] Misconfiguration is fatal and distinguishable from absence of
      configuration. A typo in a system name must not degrade into "no
      credentials, no error".
- [ ] In a non-interactive environment, fail loudly rather than starting an
      interactive flow that cannot complete.
- [ ] Token caches are single-flighted and keyed on expiry with a refresh
      buffer. The lock is for the user experience — one browser prompt — as
      much as for efficiency.
- [ ] Bearer tokens are asymmetrically signed, or short-lived. A symmetric
      signature means every validator can mint tokens.
- [ ] Discovery over `https` only, and exactly one retry after a challenge —
      an unvalidated metadata URL is an SSRF hole.
- [ ] The credential belongs to the process that talks to the system, not to
      the agent. Where a proxy is possible, the agent gets a loopback URL and
      never holds the secret at all.

- [ ] **(maintainer)** Open a crediting issue for each author and invite the PR; the
      DCO makes that better than copying.

**Done when:** vsp authenticates to an on-prem system with a client certificate and to
a BTP system with OAuth2, both configured from `.vsp.json`.

### Sprint 4 — The RFC platform (1–2 weeks)

*Theme: turn today's proof-of-concept into features.*

- [x] `ZADT_DEBUG_*` facade — parameterised attach / step / stack / variables, modelled
      on the test harness that already works over RFC (extends the existing ZADT_DEBUG
      group; no underscore straight after `Z`, per this landscape's convention).
- [ ] `vsp-debugd` — a daemon owning a pinned `rfc.Session`, with the short-lived
      MCP/CLI calls talking to it; revives the disabled debugger tools.
- [x] abapGit over RFC — `vsp rfc export <PACKAGE>` serializes a package to an
      abapGit ZIP with one call to abapGit's own `Z_ABAPGIT_SERIALIZE_PACKAGE`,
      replacing the `vsp export` → APC WebSocket → `ZCL_VSP_GIT_SERVICE` →
      `cl_abap_zip` chain. Needs abapGit on the system and no vsp helper at all.
      Verified: `Z_BADI_CHECK` → a 10 KB archive with `.abapgit.xml` and `src/*`.
      (Deserialize has no RFC entry point; that would need a Z wrapper.)
- [x] `vsp rfc probe` — fast system fingerprint in about a second: release, kernel,
      database, code page and Unicode, installed components from CVERS, ZADT_VSP and
      abapGit presence, and — the part ADT cannot answer — whether *this* user is
      authorized to call each function module vsp depends on
      (`RFC_SIMULATE_AUTH_CHECK`, which decides without executing anything). Also
      exposed as `SAP(action="rfc", params={"op":"probe"})`.
- [x] Reports and jobs over RFC — `vsp rfc run <REPORT>` schedules a report as a
      background job through the XBP BAPIs, optionally waits for it and fetches its
      spool; `vsp rfc spool <JOB> <COUNT>` reads any job's spool. This unblocks
      `#55`/`#113`: the APC ban on SUBMIT does not apply over RFC. Note
      `SUBST_START_REPORT_IN_BATCH` is the obvious call but fails with
      BATCH_SCHEDULING_FAILED on this system even with SAP_ALL — XBP takes an
      explicit target server and works. Verified: a job reached status F, and a
      real spool list was read back.
- [x] Investigate ADT over RFC (`SADT_REST_RFC_ENDPOINT`) — vsp where ICF is closed.
      Done: `vsp rfc adt <METHOD> <URI>`. The blocker was ours — SAP wraps an
      XSTRING in base64 at 76 columns, and our xRFC decoders rejected the
      newlines, so every body over 57 bytes failed to decode.

**Done when:** a breakpoint can be set, hit and inspected from an MCP client without
ZADT_VSP, and `vsp` works against a system with HTTP disabled.

### Later (not scheduled)

`landscape.go` harvest into open-rfc-go, `rfcgen`, observability hooks, the generating
"conscious" server, tRFC/qRFC, SNC.


---

*The sections below are the raw notes the sprints were distilled from; the sprint list
above is the working plan.*

## Now — small, high value

- [x] **Authenticate vsp's own HTTP transport.** `ServeHTTP` served the Streamable HTTP
  endpoint bare — no API key, no `Origin` check — so a localhost bind was exploitable by
  DNS rebinding and a `0.0.0.0` bind exposed the whole ADT tool surface unauthenticated.
  Fixed in `internal/mcp/server.go` (API key with a constant-time compare, `Origin`/`Host`
  validation, health endpoint). `marianfoo`'s fork has a fuller version worth taking
  later (RFC 9728 protected-resource metadata).
- [x] **Make `go test ./...` green on a clean checkout.** `pkg/ctxcomp/analyzer_test.go`
  and `benchmark_live_test.go` dial a live SAP system and fail with 401; they now carry
  `//go:build integration`, matching `pkg/adt/integration_test.go`.
- [x] **Bump `open-rfc-go`.** Picks up nested-structure support (`.INCLUDE`d and
  `STRU`/`TTYP` parameters now describe and call), pinned sessions, 32-bit builds.
- [ ] **Add PR CI** (`.github/workflows/ci.yml`: build, vet, test, lint). Nothing has
  merged since April and no PR has been reviewed; without CI there is no cheap signal.
- [ ] **Fix `parseActivationResult`.** It parses nothing, so *every* activation reports
  success — silent data loss for the caller. (fork survey, Tier 0)

## Next — correctness, then the merge train

- [ ] **Tier 0 bugs from the fork survey** (~1.5 days total): CSRF `HEAD`→`GET` fallback
  (four forks fixed this independently; BASIS 740 / ECC EhP7 are unusable without it),
  WebSocket ignoring `HTTP_PROXY`, redirect/session hardening.
- [ ] **Merge train** (maintainer decision, not an agent's): `#128, #126, #120, #107,
  #149` are clean and independent → then `#152`; `#125` before `#108` (they collide in
  `workflows_deploy.go`); `#148` before `#150`; `#145`; `#121`.
  ⚠️ `#106` carries a stray closing reference to issue `#2` — remove it before merging.
- [ ] **Message classes are unwritable** — `#162/#159/#160/#161`: the edit switch in
  `handlers_source.go` omits `MSAG`; `WriteMessageClassTexts` is registered without its
  `texts` argument, so the tool cannot be called at all; `MessageClass` has no `XMLName`,
  so the PUT body marshals as `<MessageClass>`.
- [ ] **Close with an explanation:** `#151` (obsolete — the old `CallRFC` forced every
  parameter through `map[string]string`; the typed RFC path handles structured tables),
  `#138` (110 files for what `#106` does in three), `#130` (silently disables four
  ZADT_VSP service domains), `#139` (contained in `#121`).

## Authentication — harvest from forks

Findings sat mostly on non-default branches. Licences are MIT throughout, but the DCO
means **inviting the author to submit** beats copying for the larger pieces.

- [ ] **macOS Keychain mTLS** (`Edgars-Ralfs-Dunis`, `feat/macos-keychain-client-cert`) —
  the best of the lot: per-handshake certificate resolution, issuer-CN fleet selection,
  the private key never leaves the keychain, and a deliberate `MaxVersion=TLS1.2` pin
  because NetWeaver 7.50's ICM drops TLS 1.3 client-cert handshakes. Fix the non-darwin
  build (`LoadKeychainClientCertByIssuers` missing from `keychain_other.go`) first.
- [ ] **Native reentrance-ticket SSO** (`BurnerPat`) — system browser + `MYSAPSSO2`
  exchange, 908 lines with tests; better than the current chromedp path and handles MFA.
- [ ] **OIDC / JWT bearer + principal propagation** (`marianfoo`) — the only such stack;
  reimplement on a real JWT library, and keep one key and cookie jar per session rather
  than regenerating them per request (which breaks stateful locks).
- [ ] **OAuth2 / XSUAA / BTP destinations / Cloud Connector** (`Prolls`) — the only work
  in this area; its `Proxy-Authorization` is sent as a request header, which does not
  authenticate an HTTPS `CONNECT` tunnel — fix before adopting.
- [ ] Open a crediting issue for each of the four authors above; several are active.

## RFC track

- [x] Classic RFC in vsp: `vsp rfc` CLI and `SAP(action="rfc")`; released in v2.40.0.
- [x] Debugger over RFC **proven end to end** — `TPDAPI_TEST_DEBUGGER` (RFC-enabled
  dynamic dispatcher) runs attach, stepping, line breakpoints, run-to-line, variable and
  locals reads, all in seconds, with no ZADT_VSP, no WebSocket and no deployed ABAP.
- [ ] **`ZADT_DEBUG_*` facade** — the test harness proves the API is reachable but only
  runs fixed scenarios; the facade exposes the same TPDA calls as parameterised
  operations (attach *this* debuggee, step *now*, read *these* variables). Extend the
  **existing** `ZADT_DEBUG` function group rather than creating a new object: it already
  ships with `vsp install zadt-vsp`, and `ZADT_00_RFC_TEST` is the precedent for an
  RFC-enabled FM in that family. Naming follows the convention in this landscape —
  no underscore straight after `Z` (`ZADT_DEBUG`, `ZADT_00_RFC`, `ZLLM_04`, `ZTST`), so
  the modules are `ZADT_DEBUG_ATTACH`, `ZADT_DEBUG_STEP`, `ZADT_DEBUG_STACK`,
  `ZADT_DEBUG_VARS`.
- [ ] **`vsp-debugd` session holder** — a daemon owning a pinned `rfc.Session` (state is
  proven to persist: a pinned session re-locks its own enqueue while another connection
  gets `FOREIGN_LOCK`), with the short-lived MCP/CLI calls talking to it. This is what
  revives the currently disabled debugger tools.
- [ ] **Use the pinned session in vsp** — `internal/mcp/handlers_rfc.go` still opens and
  closes a client per call, so `rfc.Client.Pin` (already pinned in `go.mod`) is unused.
- [ ] **abapGit over RFC** — `Z_ABAPGIT_SERIALIZE_PACKAGE` is RFC-enabled and returns a
  real ZIP; it replaces the `vsp export` → APC WebSocket → `cl_abap_zip` chain with one
  call. (Note `vsp install abapgit` is currently a no-op: both embedded ZIPs are 0 bytes.)
- [ ] **`vsp rfc probe`** — a fast system fingerprint ADT cannot produce:
  `RFC_SIMULATE_AUTH_CHECK` ("will this tool work for this user?" without executing),
  CVERS/installed components, kernel, unicode, ZADT_VSP and abapGit presence.
- [ ] **Reports and jobs over RFC** (`SUBST_START_REPORT_IN_BATCH`, `BAPI_XBP_*`, spool
  reads) — unblocks `#55`/`#113`, which are no longer an architectural limit.
- [ ] **ADT over RFC, natively** — `berndeplo`'s Java sidecar documents the
  `SADT_REST_RFC_ENDPOINT` wire contract, which would let vsp work where ICF/HTTP is
  closed. Strategically the biggest item here.

## open-rfc-go

- [ ] **Take `pkg/adt/landscape.go`** (from the fork survey): `SAPUILandscape.xml` →
  hosts, message servers, SAProuter strings, SNC partner names, plus cross-platform
  CommonCryptoLib discovery. Drops in almost unchanged and feeds `Destination.Router`.
- [ ] Never expose `SXPG_COMMAND_EXECUTE` as a tool (OS command execution; the probing
  user is authorized for it).
- [ ] Remaining roadmap: `rfcgen` (DDIC → typed Go structs), observability hooks, the
  generating "conscious" server, tRFC/qRFC, SNC.
