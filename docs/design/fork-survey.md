# Fork survey — what is worth harvesting back into vsp

Survey date: 2026-08-20. Upstream: `oisee/vibing-steampunk`, MIT, default branch `main`,
443 stars, 107 forks. Read-only survey: every fork's default branch and every
non-upstream branch was compared against `main` via the GitHub compare API. No PRs,
issues, comments or pushes were made, and no fork was added as a git remote.

## Headline

| | |
|---|---|
| Forks enumerated | 107 |
| Deleted / inaccessible (404) | 5 |
| Pure mirrors (0 commits ahead anywhere) | 61 |
| Forks with at least one commit ahead | 41 |
| Forks with **substantive functional work** | 19 |
| Forks with **authentication work** | 7 |

**A methodology warning that matters for reading the numbers below.** Upstream `main`
has been rewritten at least once (a large decompose/refactor). Forks that branched
before the rewrite therefore show inflated ahead-counts: their copy of the maintainer's
own commits has SHAs that no longer exist upstream. `txape10:fix/lock-nomodification-with-transport`
reports "407 ahead", but only 31 of those commits are actually the fork author's;
`pooyoukun/main` reports "375 ahead" of which 6 are the author's. Every count in the
detailed sections below is the *author-attributed* count, not the raw compare number.

A second framing point: **upstream already merges community PRs routinely.** The
maintainer has taken work from marianfoo, oklausen, Dominik Kropp, Andi, kts982,
Stanislav Naumov, cpru-prog, Kostas T., Robert Schulte and others. Several fork
branches that look interesting turn out to be *already upstreamed*, and one of the
biggest auth-looking branches (`cwbr:feature/browser-based-sso`) is the ancestor of
today's `pkg/adt/browser_auth.go`. That is a healthy signal, and it narrows the harvest
to genuinely un-landed work.

## Fork table

Only forks with at least one commit ahead of `main` are listed. "Ahead" is the
author-attributed count where the raw compare number is inflated by the upstream
history rewrite (marked `*`). The other 66 forks are untouched mirrors or deleted
repos and are listed in aggregate at the end.

| Fork | Ahead / behind | Last push | Verdict |
|---|---|---|---|
| `BurnerPat/vsp-enterprise` (`develop`, 5★) | 115 / 309 | 2026-08-17 | **(c) real work** — hard fork + re-architecture; native reentrance-ticket SSO, SNC landscape, JCo sidecar, role model, test server |
| `Basantmh/vibing-steampunk` | 49 / 10 | 2026-08-17 | **(c) real work** — integrator fork collecting Edgars (mTLS), frd1201, Dominik Miescher, Jannes Dailidow, ManuelFrieder |
| `cwbr/vibing-steampunk` (`feat/rfc-mode-go`) | 15 / 309 | 2026-03-27 | **(c) real work** — JCo sidecar + SAP UI Landscape + SNC; `feature/browser-based-sso` already upstreamed |
| `vinchacho/vibing-steampunk` | 110 / 10 | 2026-08-18 | **(c) real work**, thin — impact-gated writes; 138/288 files are Markdown, 61 `.go` files are an import-path rewrite |
| `Prolls/vibing-steampunk` | 11 / 62 | 2026-04-09 | **(c) real work** — BTP Cloud Foundry, XSUAA OAuth2, Cloud Connector proxy + 407 refresh |
| `Edgars-Ralfs-Dunis/…` (`feat/macos-keychain-client-cert`) | 9 / 86 | 2026-08-04 | **(c) real work** — macOS Keychain mTLS client-cert auth (origin of the Basantmh mTLS stack) |
| `blicksten/vibing-steampunk` | 45 / 132 | 2026-06-04 | **(c) real work**, ~10% — install silent-failure fix, SRIS source search, DUM transport fallback; rest is agent-config slop |
| `Augusto42/vibing-steampunk` | 32 / 10 | 2026-08-17 | **(c) real work** — CSRF session-mode fix, fail-closed fixes, ENHO enhancement layer, CGO-free SQLite |
| `pooyoukun/vibing-steampunk` | 6* / 379 | 2026-07-09 | **(c) real work** — lock session pinning, 423 retry, ENQUEUE release, namespaced syntax check, Windows test fixes |
| `txape10/vibing-steampunk` | 31* / 379 | 2026-07-31 | **(c) real work** — CSRF 403 fallback, DDIC endpoint verb fix, activation-result parsing, ABAP text-pool fixes |
| `wusxo24/vibing-steampunk` | 6 / 11 | 2026-04-22 | **(c) real work**, small — redirect header preservation, `ICMENOSESSION` cookie purge, redacted HTTP trace |
| `frd1201/vibing-steampunk` | 33 / 10 | 2026-08-03 | **(c) real work** — CSRF HEAD→GET fallback, session-type env, include write support, search type filter |
| `dme007/vibing-steampunk` | 18 / 11 | 2026-08-19 | **(c) real work**, small — deploy session affinity, websocket proxy env, mutation-gate skip |
| `marianfoo/vibing-steampunk` (3★) | 38 / 249 | 2026-03-25 | **(c) real work** — but largely already upstreamed (`GetDependencyZIP`, debugger schema, release workflow) |
| `Aagaard89/…` (`upstream/iam-authorization-chain`) | 3 / 34 | 2026-08-13 | **(c) real work** — IAM business catalogs / IAM apps over ADT (SAP-side authorization, not client auth) |
| `abapacademy/vibing-steampunk` | 3 / 11 | 2026-04-28 | **(c) real work**, small — 3-step CSRF fallback for BASIS 740 / ECC EhP7 |
| `Jannes-Dailidow/vibing-steampunk` | 1 / 10 | 2026-06-30 | **(c) real work**, small — `--allowed-read-packages` read-side safety gate, with tests |
| `ManuelFrieder/vibing-steampunk` | 1 / 10 | 2026-08-06 | **(c) real work**, small — `CreateStructure` for classic DDIC structures |
| `andreasmuenster/vibing-steampunk` | 3 / 11 | 2026-05-07 | **(c) real work**, tiny — `sap-client` query param on browser auth |
| `danielheringers/…` (`feat/mcp-go-0.43.2-http-streamable`) | 8 / 334 | 2026-03-16 | **(b/c)** — mcp-go upgrade to streamable HTTP; superseded by upstream's own transport work |
| `berndeplo/vibing-steampunk` (2★) | 2 / 339 | 2026-03-05 | **(c) real work** — earlier cut of the JCo sidecar, superseded by `cwbr:feat/rfc-mode-go` |
| `ejbrait/vibing-steampunk` | 2 / 339 | 2026-03-11 | **(b) trivial** — Dockerfile + entrypoint for streamable HTTP |
| `oklausen/…` (`docs/copilot-instructions`) | 10 / 11 | 2026-05-11 | **(b) trivial** — already upstreamed |
| `barkow15`, `Klairgo`, `zooloo303`, `ovetchenkoandrey`, `lin2qwer1-cloud`, `enricoandreoli`, `EdgarsRD`, `snymanpaul`, `ludecke`, `f1se4`, `trstroem`, `metalsXP`, `PraveenLS-dot`, `danielerdmannarv`, `jessiejames19391`, `mahlzeit1948`, `luketebo/BizVibe-Tools` | 1–11 | various | **(b) trivial / already landed / personal customisation** — see the third-tier notes |
| `gcggyl28/vibing-steampunk` | 14 / 11 | 2026-05-18 | **(b) noise** — an LLM took "steampunk" literally and generated a boiler/piston/flywheel simulation package. Nothing to do with SAP |
| 61 forks | 0 / 10–457 | — | **(a) untouched mirrors** |
| `d-eise`, `frozenraindrop`, `lopozs1245yndiej`, `Stew36195ardJeremiah`, `vitalratel` | — | — | deleted or made private since forking (404) |

---

# Authentication

This is what the survey was commissioned for, so it gets its own treatment. The short
answer: **six forks did real authentication work, and between them they cover four
flows vsp does not have today.** The suspicion was well founded.

## What vsp has today

| Mechanism | Where | Notes |
|---|---|---|
| Basic auth | `Config.Username` / `Password`, `HasBasicAuth()` | The default path |
| Cookie auth | `Config.Cookies`, `pkg/adt/cookies.go`, `--cookie-file` / `--cookie-string` | Netscape format |
| SAML form-scraping | `pkg/adt/saml_auth.go` (418 L) | 4-step IdP dance against IAS, `maxSAMLHops = 10`, HTML form parsing via `golang.org/x/net/html`. **No MFA** — documented |
| Browser SSO (chromedp) | `pkg/adt/browser_auth.go` (421 L) | Drives a headed Chromium to a login page, harvests `MYSAPSSO2` / `SAP_SESSIONID` / `JSESSIONID`. Came from cwbr's PR #77 |
| Re-auth on 401 | `Config.ReauthFunc` | Only consulted when `!HasBasicAuth()`; re-runs the SAML dance |
| Session keep-alive | `Client.StartKeepAlive` | Also from cwbr's PR #77 |
| External credential helper | `pkg/adt/credential_cmd.go` | argv-based (no shell), 30s timeout, stderr discarded, stdout zeroed after parse. **Good hygiene** |
| Auth-preserving redirects | `Config.NewHTTPClient` `CheckRedirect` | Re-sets `Authorization` across hops (issue #90) |
| Proxy | `http.ProxyFromEnvironment` | Env vars only |
| Credential storage | `pkg/config/systems.go` → `.vsp.json` | `password` field marked *"Not recommended, use env var"*. Plaintext JSON, no OS keystore |

**Not present anywhere upstream** (verified by grep over the working tree):
TLS client certificates, any OS keystore integration, OAuth2 in any grant, JWT/bearer
tokens, XSUAA, BTP destinations, SAP Cloud Connector, principal propagation, SNC,
SAP UI Landscape parsing, reentrance tickets.

## What exists in the wild

### 1. macOS Keychain mTLS client certificates — `Edgars-Ralfs-Dunis:feat/macos-keychain-client-cert`

*Author: Edgars Dunis `<edgars.dunis@zalaris.com>` (Zalaris). MIT. Also carried, rebased,
on `Basantmh:pull/edgars-mtls-debugger-fixes` and merged into `Basantmh/main`.*

**Flow: X.509 client-certificate (mutual TLS) against the ICM.** The certificate is the
one SAP Secure Login Client (SLC) drops into the login keychain after an IAS/SLS login,
so this is effectively "SAP SSO via SLC, without a password" — the cert CN becomes the
effective SAP user.

- `pkg/adt/keychain_darwin.go` (130 L) uses `github.com/github/smimesign/certstore` (MIT)
  to enumerate Security.framework identities. **The private key never leaves the
  keychain** — it hands back an in-place `crypto.Signer`, which is exactly right.
- Two selectors: `LoadKeychainClientCert(cn)` pins one user's cert;
  `LoadKeychainClientCertByIssuers([]string)` picks the *freshest currently-valid* cert
  from a set of issuer CNs. The issuer variant is the clever bit — one shared `.vsp.json`
  works for a whole fleet, because each user's own SLC cert matches without a per-user CN.
- `Config.ClientCertProvider` resolves **lazily per TLS handshake** via
  `tls.Config.GetClientCertificate`. That means (a) the MCP server starts even when SLC
  is not logged in yet, and every tool call then returns the real error instead of a
  generic "connection failed" buried in a log; (b) an SLC re-login mid-session heals the
  next handshake with no restart. `NewCachingCertProvider` caches until 5 minutes before
  `NotAfter` and retries failed loads on every call.
- **`MaxVersion = tls.VersionTLS12` whenever a client cert is configured.** The comment
  says NetWeaver 7.50's ICM drops TLS 1.3 client-certificate handshakes. That is hard-won
  field knowledge you will not find in documentation, and it is the single most valuable
  line in the patch.
- `pkg/adt/websocket_base.go` suppresses the `Authorization` header in cert mode (an
  empty-password basic header 401s), applies the same TLS 1.2 pin to the dialer, and
  starts surfacing SAP's HTML error body instead of discarding it.
- Surfaced as `--client-cert-cn` / `--client-cert-issuer` and `SAP_CLIENT_CERT_CN` /
  `SAP_CLIENT_CERT_ISSUER`, counted as an auth method in `processCookieAuth`, and the
  effective username is derived from the cert's Subject CN.

**Quality: high, with three defects.**

1. **It does not compile off macOS.** `keychain_other.go` defines `LoadKeychainClientCert`
   and `LoadKeychainClientCertByIssuer` (singular), but `cmd/vsp/main.go:241` calls
   `LoadKeychainClientCertByIssuers` (plural). Blocking, five-minute fix.
2. `fmt.Errorf(errKeychainUnsupported)` is a non-constant format string — `go vet` will
   flag it.
3. `keychainKeepAlive` is a package-level global holding the store and identity for the
   process lifetime; it is overwritten on each reload, so every cert refresh leaks the
   previous store handle. Documented as deliberate for the first load, but not for refreshes.
   No tests for any of the keychain code.
4. `go.mod` marks `github.com/github/smimesign` as `// indirect` when it is a direct
   dependency — cosmetic.

**Verdict: take as a patch, near-verbatim.** It is the only real client-authentication
advance in the whole fork network.

### 2. Native reentrance-ticket SSO — `BurnerPat/vsp-enterprise:develop`

*Author: BurnerPat. MIT, with a 154-line `FORK.md` that credits upstream generously.*

**Flow: SAP reentrance ticket via the user's own system browser** — the same mechanism
Eclipse ADT uses. This is strictly better than the chromedp approach vsp ships today,
because it reuses the user's real browser profile (so an existing Kerberos/SAML/IAS
session, MFA included, is already there) instead of a throwaway Chromium profile.

`pkg/adt/browser_auth.go` grows to 908 lines:

1. `resolveSystemURLs()` GETs `/sap/public/bc/icf/virtualhost` and reads
   `relatedUrls.API` / `.UI` — this is what handles cloud systems whose API and UI live
   on different hosts. 404/501 falls back to the base URL.
2. Bind `127.0.0.1:0`, serve `/adt/redirect`.
3. `openSystemBrowser()` (`open` / `rundll32` / `xdg-open`) on
   `buildReentranceTicketURL()` = `/sap/bc/adt/core/http/reentranceticket?redirect-url=…`.
4. The ticket comes back on the loopback callback.
5. `exchangeReentranceTicket()` GETs `/sap/bc/adt/core/http/sessions` with
   `MYSAPSSO2: <ticket>`, `sap-adt-purpose: preflight_logon`,
   `x-sap-security-session: create` and the Eclipse ADT User-Agent.
6. `parseSystemInformationLink()` follows the `…/systeminformation` rel with
   `x-sap-security-session: use`, then harvests the cookie jar.

The chromedp path survives behind `--browser-exec` / `--browser-auth-url`. 15 tests in
`browser_auth_test.go` cover the new flow with `httptest`, including rejection of
missing SAP sessions, redirects and login HTML.

Also here: `reauthAndFetchCSRF()` (fires only when `Reauth != nil && !HasBasicAuth()`,
hot-swaps cookies, re-fetches CSRF, hard-fails if still 401) and `isAuthRedirectPage()`,
which sniffs an HTTP-200 HTML body for `/oauth/authorize`, `/saml2/idp/sso` or
`fragmentAfterLogin` and treats it as session expiry. The sniffing is a little fragile
but it solves a real Steampunk/BTP failure mode where expiry arrives as a 200.

**Verdict: take as a patch** for `nativeBrowserLogin` / `exchangeReentranceTicket` /
`resolveSystemURLs` (~1 day including the test port); **reimplement** the 401 re-auth and
redirect sniffing into your existing `pkg/adt/http.go` (~2-3 h) rather than adopting
their `pkg/adt/connection/` package split.

### 3. OAuth2 / XSUAA / BTP Destinations / Cloud Connector — `Prolls/vibing-steampunk`

*Author: DRANCOURT Christophe. MIT. 3 auth commits plus BTP packaging.*

This is the only OAuth2 implementation in the network, and the only Cloud Connector work.

**Flow: OAuth2 `client_credentials` against XSUAA → BTP Destination service → SAP Cloud
Connector HTTP proxy.** New package `pkg/btp/destination.go` (269 L):

- `ResolveDestination(name)` parses `VCAP_SERVICES`, takes the `destination` binding,
  fetches a token from `tokenServiceURL` (falling back to `credentials.url`, then the
  service URI) with `grant_type=client_credentials`, then GETs
  `/destination-configuration/v1/destinations/{name}` with a bearer token.
- For `ProxyType: "OnPremise"` it uses the inline `proxyConfiguration` if the destination
  service returned one, otherwise falls back to `resolveConnectivityProxy()`, which reads
  the `connectivity` binding (`onpremise_proxy_host`, `onpremise_proxy_http_port`,
  default 20003) and fetches its own XSUAA token.
- `resolveConnectivityProxy` returns a **`refresh func() (string, error)` closure** over
  the XSUAA credentials, so the token can be renewed later.
- `pkg/adt/config.go` gains `ProxyURL` / `ProxyAuth` / `ProxyAuthRefresh`, a `WithProxy`
  option, and a `proxyAuthTransport` `RoundTripper` that injects `Proxy-Authorization`
  and, on HTTP 407, calls `refresh` and retries once under a mutex.

**Quality: right shape, several real problems.**

- **`Proxy-Authorization` is set as a request header, which does not authenticate a
  `CONNECT` tunnel.** For `https://` destinations Go negotiates the proxy with `CONNECT`
  and uses `Transport.ProxyConnectHeader` / `GetProxyConnectHeader` — a request header
  goes *inside* the tunnel to SAP, not to the Cloud Connector. It works today only
  because the Cloud Connector proxy leg is plain HTTP. This must be fixed before adoption.
- **The 407 retry replays a request whose body has already been consumed.** Fine for GET,
  broken for every POST/PUT.
- **Only `BasicAuthentication` destinations are supported** — it explicitly rejects
  anything else. So no `OAuth2SAMLBearerAssertion`, no `PrincipalPropagation`, no
  `ClientCertificateAuthentication`. **There is no principal propagation anywhere in the
  fork network**; this is a technical-user-only integration.
- No token caching against `expires_in` — the destination token is refetched on every
  `ResolveDestination`, and the connectivity token is only renewed reactively on 407.
- `bindings[0]` blindly, with no instance-name selection.
- No `SAP-Connectivity-SCC-Location-ID` header, so multi-Cloud-Connector landscapes
  are unsupported.
- No `context.Context` plumbing; fixed 15s timeouts ignore caller cancellation.
- Destination `User`/`Password` are carried in plaintext through `DestinationConfig` into
  `adt.Config` with no zeroing, unlike upstream's `credential_cmd.go`.
- No mTLS-bound (X.509) XSUAA bindings, which is now the default for new BTP bindings.
- **Zero tests.**
- The fork also commits `vsp_linux` and `vsp_cf_binary` build artefacts to git.

**Verdict: reimplement, do not take.** The design sketch is genuinely useful and worth
following; the code has a protocol bug, a retry bug, and no tests.

### 4. SNC and SAP UI Landscape — `cwbr:feat/rfc-mode-go` (also in BurnerPat and berndeplo)

*Authors: Benjamin Bockmuehl, Patrick Jackes, Christian W (cwbr), BurnerPat. cwbr's repo
is MIT; `berndeplo`'s fork has **no LICENSE file**, so take the cwbr or BurnerPat copy.*

This branch is a three-fork collaboration (cwbr merged BurnerPat's PR #1 and berndeplo's
`feat/jco-proxy-sidecar`). Two separable halves:

**Half A — SAP UI Landscape parsing + SNC discovery. Pure Go, no Java, no SAP binaries.**

- `pkg/adt/landscape.go` (588 L) parses `SAPUILandscape.xml`: `<Messageserver>`,
  `<Router>`, `<Service>` with `sncop` / `sncname` / `sncnosso`, `msid`, `routerid`,
  `systemid`, `mode` (0 = load-balanced via message server, 1 = direct app server).
- `FindLandscapeFiles()` follows the same discovery order as Eclipse ADT's
  `SapUiLandscapeReader.java`: explicit path → `SAPLOGON_LSXML_FILE` → Windows registry
  → platform defaults. `landscape_windows.go` (204 L) reads
  `HKCU\Software\SAP\SAPLogon\LandscapeFilesLastUsed` and the
  `Options\PathConfigFilesLocal` fallback.
- `FindSystemByID()` prefers SNC-with-SSO, then any SNC service, then anything.
- `ResolveSNCJcoProperties()` emits `jco.client.snc_mode/partnername/qop/lib`,
  `mshost`/`msserv`/`r3name`/`group` or `ashost`/`sysnr`, and `saprouter` from the
  referenced router entry.
- `findSNCLibrary()` locates `sapcrypto` / `sncgss64` by `exec.LookPath` and then by
  scanning SAP Secure Login Client, CommonCryptoLib, SAPgui and System32/SysWOW64
  directories — because Eclipse and other IDE-spawned processes do not inherit the full
  system `PATH` and JCo then fails with "Unable to load GSS-API DLL".
- 410 lines of tests in `landscape_test.go`.

**Half B — a Java JCo sidecar.** `sidecar/jco-proxy/` (~2.4k LOC Java: Javalin + Gson +
slf4j, `sapjco3` at `provided` scope so nothing proprietary is redistributed), driven from
Go by `pkg/adt/sidecar.go` (591 L), `stdio_transport.go`, `rfc_transport.go` and
`jco_discovery.go`. A 6.2 MB shaded `embedded/deps/jco-proxy.jar` is committed to git.

⚠️ **Security defect in the sidecar** (BurnerPat's copy, `pkg/adt/sidecar.go:424`):
`args = append(args, "--password", s.config.Password)`, and `buildArgs` also splats every
`JcoProperties` entry — including `jco.client.passwd` — as `--key value`. **SAP passwords
land in `argv` and are world-readable via `ps`.**

**Verdict: take Half A, ignore Half B.** The landscape/SNC discovery is clean, tested,
platform-aware and has no upstream equivalent. The Java sidecar is strategically the
opposite of what open-rfc-go exists to do, drags in a JRE and a 6 MB binary, and ships a
credential-exposure bug.

### 5. OIDC / JWT bearer + principal propagation — `marianfoo/vibing-steampunk`

*Author: Marian Zeis (`„marianfoo"`). MIT, 3★. On `main`, which is **not harvestable
wholesale** — it also deletes the debugger, git, LSP, WASM compiler, workflow, DSL,
reports, help and install layers to make an "enterprise connector" trim. Cherry-pick
individual files.*

This is the most ambitious auth work in the network and it fills the two gaps everything
else leaves open. Four separable pieces, each with tests and a setup document
(`docs/phase2-oauth-setup.md`, `phase3-principal-propagation-setup.md`,
`phase4-btp-deployment.md`).

**(a) OIDC / JWT bearer validation** — `pkg/adt/oauth.go` + `oidc.go`. Hand-rolled RS256
verification: JWKS discovery via `/.well-known`, `kid` lookup, `alg` **pinned to RS256**
(so no alg-confusion attack), `exp`/`nbf`/`iss`/`aud` checked, then a claim → SAP-user
mapping with an uppercase fallback. Two defects: a token with **no `exp` at all is
accepted** (`if claims.Exp > 0`), and there is no clock-skew leeway. The uppercase
fallback silently maps any IdP subject onto a SAP user name, which is a policy decision
hiding in a string transformation. **Verdict: reimplement on `go-jose` or `golang-jwt`**
— there is no good reason to hand-roll JWT validation.

**(b) Principal propagation via ephemeral X.509** — `pkg/adt/principal_propagation.go`
(245 L) + tests. **This is the one flow nobody else attempted.** OIDC identity →
`GenerateEphemeralCert` mints a short-lived (default 5 min) client certificate with
`CN=<username>` signed by a local CA → SAP trusts that CA via STRUST → SAP maps Subject CN
to the SAP user via CERTRULE. The result is that each MCP request authenticates to SAP
**as the end user, with a real audit trail, and no SAP credentials stored anywhere.**
`GenerateEphemeralCert` itself is correct: `ExtKeyUsageClientAuth`, a -1 minute skew
allowance, proper serial generation.

The implementation around it is not usable as-is. `Do()` generates a **fresh RSA-2048 key
plus a new `http.Client` and a new `cookiejar` on every single request**. That is
50-100 ms of CPU per call, and — more seriously — a new cookie jar per request destroys
`sap-contextid` continuity, so stateful locks cannot work at all. The fix is a per-user
cache of (certificate, `http.Client`) keyed by username and expiring with the cert.
**Verdict: reimplement. The design is right and worth building; the request path is not.**

**(c) File-based mTLS + custom CA** — `WithClientCert` / `WithCACert` in
`pkg/adt/config.go`. Complements Edgars' keychain path for the Linux/CI case where the
cert is a file, and adds a custom root pool for private CAs. Errors are swallowed into
`c.OAuthError` rather than failing hard, which should be reversed. **Take as a patch.**

**(d) BTP / Cloud Foundry** — `pkg/adt/btp.go` reads `VCAP_SERVICES` for XSUAA,
destination and connectivity bindings. It overlaps `Prolls`' `pkg/btp/destination.go`
almost completely. Prolls' structure is cleaner; take that one and the tests from here.

Also here: `cmd/setup-certrule/main.go` is a throwaway probe script with **hardcoded live
credentials** (`ABAPtr2023#00`). Ignore it, and note it as a reason to read any harvested
file rather than trusting the branch.

### 6. Authentication on vsp's own HTTP transport — a live gap, with a ready fix

Not SAP-facing, but it belongs in an auth review. `internal/mcp/server.go:233` serves
`server.NewStreamableHTTPServer(s.mcpServer)` **bare** — no API key, no `Origin`
validation. A localhost bind is therefore exploitable by DNS rebinding from any page the
user visits, and a `0.0.0.0` bind hands an unauthenticated remote caller the full ADT tool
surface under the operator's SAP credentials.

`marianfoo` has the complete fix: `apiKeyMiddleware` using
`subtle.ConstantTimeCompare` and answering `WWW-Authenticate: Bearer`,
`originValidationMiddleware` with an `isSameOriginHost` check that deliberately skips
wildcard binds, a `healthHandler`, and an RFC 9728 `protectedResourceMetadataHandler` so
MCP clients can discover the protected-resource metadata. The implementation is correct.

**Verdict: take as a patch, and treat it as the highest-priority item in this document
that is not a feature.** It is small, and it closes a real hole.

### 7. Session, CSRF and redirect hardening — five forks, converging

Not "authentication" in the flow sense, but this is where auth *breaks* in practice, and
the convergence is the strongest signal in the survey.

- **CSRF `HEAD` → `GET` fallback.** Upstream's `fetchCSRFToken` is `HEAD`-only against
  `/sap/bc/adt/core/discovery`. BASIS 740 / ECC EhP7 answer 400 with no token, so vsp is
  simply unusable there. **Four forks fixed this independently**: `abapacademy`
  (Ladislav Rydzyk — a 3-step ladder: `HEAD /core/discovery` → `GET /core/discovery` →
  `GET /sap/bc/adt/discovery`), `frd1201` (Fabian Diehl), `txape10`, and `Basantmh`
  (which adds a skip-fallback-on-401/403 guard). Four independent discoveries of one bug
  is as close to a confirmed defect as a survey gets.
- **CSRF session-mode binding.** `Augusto42:60ec08e8` makes the token fetch use the same
  session mode as the request that triggered it. Upstream binds only to
  `config.SessionType`, so a stateful lock after a stateless token fetch gets a 403. This
  is the same bug class from the other side; merge both fixes into one change.
- **Redirect header preservation.** Upstream re-sets `Authorization` across redirects.
  `wusxo24` (Dominik Miescher) and `Basantmh` extend that to `X-CSRF-Token` and
  `X-sap-adt-sessiontype`, with an honest comment that Go forwards custom headers anyway
  and this is belt-and-braces plus intent-documentation.
- **`ICMENOSESSION` cookie purge.** Upstream detects session expiry but never drops the
  dead `sap-contextid`, so a long-running MCP server loops. `Basantmh`'s
  `Transport.clearSAPSessionCookies()` replaces the whole jar, and explains why targeted
  deletion fails: Go's `cookiejar` keys by (name, domain, path) and will not reveal the
  stored path, while ICM sets `sap-contextid` at `/sap/`, `/sap/bc/` *and* `/sap/bc/adt/`.
- **`Secure` flag stripping over plain HTTP.** `Basantmh`'s `httpCookieJar` strips
  `Secure` from cookies received over plain HTTP, without which SAP behind an
  `nginx → ICM` reverse proxy never gets its session cookie back. Directly relevant to
  port-forwarded lab topologies.
- **`sap-client` on browser auth.** `andreasmuenster` (Andi) appends `sap-client=NNN` to
  the ADT URL so SSO cookies land on the right client. Correct; the signature change to
  `BrowserLogin` is a breaking API change and should become an options struct instead.
- **The WebSocket path ignores proxy environment variables.** `pkg/adt/websocket_base.go`
  builds bare `websocket.Dialer{}` and `http.Transport{}` literals, so `HTTP_PROXY` /
  `HTTPS_PROXY` / `NO_PROXY` are honoured on the HTTP path and silently ignored on the WS
  path — the debugger, AMDP, RFC and report-execution tools simply cannot connect from
  behind a corporate or BAS proxy. `dme007:fix/websocket-proxy-env` (Dominik Miescher)
  adds `newZADTVSPDialer` / `newPreAuthHTTPClient` with `http.ProxyFromEnvironment`.
  Two files, and it is the best value-per-line item in the entire survey.
- **Proxy-held `sap-contextid`.** Behind the Business Application Studio destination
  proxy, `Set-Cookie` never reaches the client: the proxy holds `sap-contextid` and
  injects it whenever the request carries no `Cookie` header, so a stateless request kills
  the context and vsp's recovery is useless against an empty jar. `dme007`'s opt-in guard
  sends an explicit empty `Cookie: sap-contextid=`. Live-verified via an ICM trace.
- **Redacted HTTP trace.** `wusxo24` adds `VSP_HTTP_TRACE=1` dumping requests/responses
  to stderr with `Authorization`, `Cookie` and `Set-Cookie` values redacted and bodies
  capped at 4 KB. Good hygiene, and exactly the tool you want when debugging an auth
  handshake.

### 8. SAP-side authorization (adjacent, not client auth) — `Aagaard89:upstream/iam-authorization-chain`

*Author: Patrick Aagaard `<patrickaagaard@me.com>`. MIT. 3 commits, ~1150 new lines.*

Not client authentication — this automates the **IAM/authorization chain** on S/4HANA
Cloud: business catalogs (ADT type `SIA1`), IAM apps (`SIA6`), and catalog assignments
(`SIA7`), plus `Client.RawGet` as a read-only escape hatch for unmodelled ADT types and a
`cmd/vsp/cli_adt_raw.go` CLI for it.

Quality is exceptional and the comments are the reason. It documents that the
serialization comes from ABAP package `SR_APS_IAM_WBI_SIA1`, names the transformations,
notes that the sub-resources appear in **no** ADT discovery document, and records how the
`$publish` query parameter was discovered — by sending the wrong one and reading the
400: *"Parameter businessCatalogID could not be found."* It also self-documents its own
seams (`GetObjectURL` returns empty for `SIA1/BC`, so `CreateObject` skips its orphan-lock
retry). `RawGet` is deliberately GET-only and rejects other verbs rather than forwarding
them. 120 lines of tests.

Without this, the last mile of a RAP stack — publishing a service binding produces an IAM
app that grants nothing until a catalog and a business role pick it up — cannot be
automated at all.

**Verdict: take as a patch.** It is better documented than most of the upstream tree.

## The gap, stated plainly

| Flow | Upstream | In a fork | Assessment |
|---|---|---|---|
| Basic auth | ✅ | — | Fine |
| Cookie / Netscape file | ✅ | — | Fine |
| SAML form-scraping (no MFA) | ✅ | — | Works, brittle by nature |
| Browser SSO via chromedp | ✅ | — | Works; superseded by reentrance tickets |
| **Reentrance ticket (system browser, MFA-capable)** | ❌ | BurnerPat | **Take** |
| **X.509 / mTLS client cert** | ❌ | Edgars Dunis | **Take** |
| **OS keystore for the private key** | ❌ | Edgars Dunis (macOS only) | **Take, extend** |
| **OAuth2 client_credentials (XSUAA)** | ❌ | Prolls | **Reimplement** |
| **BTP Destination service** | ❌ | Prolls | **Reimplement** |
| **SAP Cloud Connector proxy + 407 refresh** | ❌ | Prolls (with a `CONNECT` bug) | **Reimplement** |
| **SNC config discovery (landscape XML, crypto lib)** | ❌ | cwbr / BurnerPat | **Take** |
| **OIDC / JWT bearer validation (inbound)** | ❌ | marianfoo | **Reimplement** on a real JWT library |
| **Principal propagation (ephemeral X.509 + CERTRULE)** | ❌ | marianfoo | **Reimplement** — right design, unusable request path |
| **File-based client cert + custom CA** | ❌ | marianfoo | **Take** |
| **Auth on vsp's own HTTP transport** | ❌ | marianfoo | **Take** — this is a live security hole |
| OAuth2 authorization-code + PKCE | ❌ | ❌ | Nobody built it |
| SAML2 bearer assertion / OAuth2SAMLBearer destinations | ❌ | ❌ | Nobody built it |
| OS keystore on Windows/Linux (CryptoAPI / PKCS#11) | ❌ | ❌ | Nobody built it |
| Secret storage outside plaintext `.vsp.json` | partial (`credential_cmd`) | ❌ | Weakest part of today's story |

## What a good auth story would look like

### For vsp

The pieces exist; what is missing is a shape that holds them. Today auth is a set of
mutually exclusive branches in `processCookieAuth` that counts `authMethods` and errors
if the count is not 1. That does not survive a fifth and sixth mechanism.

**1. A `Credential` interface, resolved once and consulted per request.**

```go
// pkg/adt/auth
type Credential interface {
    // Apply decorates an outbound request (basic header, cookies, bearer, nothing for mTLS).
    Apply(*http.Request) error
    // TLS contributes client certificates and version pins, or nil.
    TLS() *tls.Config
    // Refresh re-establishes the credential after a 401/407. Idempotent, single-flight.
    Refresh(context.Context) error
    // Name is for diagnostics; must never render a secret.
    Name() string
}
```

Basic, cookie, SAML, browser/reentrance, mTLS, XSUAA-bearer and Cloud-Connector-proxy all
implement it. `Config.ReauthFunc` becomes `Credential.Refresh` and stops being
special-cased to `!HasBasicAuth()`. Composition (mTLS *and* a Cloud Connector proxy token,
which is a real BTP-plus-SLC scenario) becomes a `chain` rather than an error.

**2. Move TLS assembly into one place.** Edgars' `Config.tlsClientConfig()` is the right
idea and should become the only constructor of `tls.Config` in the codebase — today
`saml_auth.go`, `browser_auth.go`, `websocket_base.go` and `config.go` each build their
own, which is exactly how the TLS 1.2 pin gets forgotten on one path.

**3. Fix proxy authentication properly.** `Transport.GetProxyConnectHeader` for the
`CONNECT` leg, `Proxy-Authorization` on the request only for plain-HTTP proxying, token
cached against `expires_in` with proactive refresh, and 407 retry that rewinds the body
via `GetBody`.

**4. Secrets.** `.vsp.json` with a plaintext `password` is the weakest link, and it is
the *default* path. Three steps, cheap to expensive: (a) refuse to read a `.vsp.json`
whose mode is group- or world-readable, and warn when `password` is set at all;
(b) promote `credential_cmd` from a corner feature to the documented default, since it
already composes with `pass`, `op`, `security find-generic-password` and
`gpg -d`; (c) native keystore backends — macOS Keychain (the `certstore` dependency is
already there), Windows Credential Manager via `wincred`, Secret Service on Linux.

**5. Redact by construction.** `wusxo24`'s `VSP_HTTP_TRACE` should be upstream, and the
redaction list should live next to the `Credential` implementations so a new mechanism
cannot forget to register its secret-bearing header.

**6. Principal propagation is the strategic item, and marianfoo has already sketched it.**
His ephemeral-X.509 + CERTRULE design is the correct on-premise answer: mint a short-lived
certificate with `CN=<end user>` from a CA that SAP trusts via STRUST, and let SAP map it
back. Rebuilt with a per-user certificate and client cache it becomes practical. The BTP
half — `OAuth2SAMLBearerAssertion` / `OAuth2JWTBearer` destinations plus the
`SAP-Connectivity-Authentication` header — is still unbuilt anywhere. Between them they
are the only way an MCP server can act *as* the requesting user rather than as a shared
technical account, which is the first thing any enterprise security review will ask for.

**7. The WebSocket bridge is Basic-auth only.** `pkg/adt/websocket_base.go` sends a basic
header and nothing else, so every SSO, cookie, browser-auth and (until Edgars' patch)
certificate user is locked out of the debugger, AMDP and report-execution paths entirely.
`barkow15` adds `SetCookies(map[string]string)` and a cookie-jar dial path; Edgars adds
the certificate path. Both are needed, and whatever `Credential` abstraction lands must
cover the WS dialer, not just `http.Transport`.

### For open-rfc-go

open-rfc-go is Apache-2.0, has SAProuter routing (`internal/saprouter`), and no SNC.
Three things from this survey transfer:

1. **`pkg/adt/landscape.go` transfers almost unchanged and is the highest-value item.**
   open-rfc-go's connection parameters today come from `SAP_*` environment variables. The
   landscape parser gives it `ashost`/`sysnr`, `mshost`/`msserv`/`r3name`/`group`, and —
   crucially — the **SAProuter string** from the `<Router>` entry, feeding the
   `saprouter.Route` machinery that already exists. That is a real usability win
   independent of SNC, and it is the same file SAP GUI and Eclipse ADT read, so it is
   already correct on every developer machine.
2. **`findSNCLibrary()` is the reconnaissance half of an SNC story.** Locating
   `sapcrypto` / `libsapcrypto.so` / `sncgss64.dll` across SLC, CommonCryptoLib, SAPgui
   and system directories is exactly the part that is tedious and platform-specific. The
   remaining work — a `cgo` GSS-API binding to CommonCryptoLib, then wrapping the RFC
   frames in `gss_wrap`/`gss_unwrap` after `gss_init_sec_context` — is a genuinely large
   project, but it starts here. Note this reintroduces a native dependency, which cuts
   against the SDK-free goal; a pure-Go SNC is not realistic because the token format is
   CommonCryptoLib's.
3. **Edgars' lazy-certificate pattern generalises.** A `func() (*tls.Certificate, error)`
   resolved per handshake, cached against `NotAfter`, retried on failure, is the right
   shape for any credential that an external agent (SLC, a Kerberos ticket cache, a
   short-lived cert minter) refreshes out of band. Whatever open-rfc-go eventually does
   for SNC credentials should follow it.

**Licence direction matters here.** vsp and all its forks are MIT; open-rfc-go is
Apache-2.0. MIT → Apache-2.0 is a permitted direction provided the MIT notice travels with
the code, so harvesting into open-rfc-go means adding the original copyright to `NOTICE`
and a provenance header on each ported file, in the style `docs/provenance.md` already
uses for the open-rfc port. **The reverse is not allowed** — nothing Apache-2.0 may flow
back into MIT-licensed vsp without relicensing. Also note open-rfc-go requires a DCO
sign-off, which a `Co-authored-by` trailer does not supply; for anything taken there,
either reimplement from the design or invite the original author to submit it.

---

# Beyond authentication

## Correctness bugs the forks found that upstream still has

These are the cheapest wins in the document. Each was verified against the current
working tree.

| Bug | Where | Found by | Effort |
|---|---|---|---|
| **`parseActivationResult` never parses anything.** The struct declares ``Messages messages `xml:"messages"` ``, but `xml.Unmarshal` maps the root `<chkl:messages>` onto the struct itself, so it looks for a `<messages>` child that cannot exist. **Every activation silently returns `Success: true` with no messages**, hiding real errors | `pkg/adt/devtools.go` | EdgarsRD (cleanest fix — flatten to `Msgs []msg` + `Entries []inactiveEntry`), also ManuelFrieder, txape10 | trivial |
| **`registerGCTSTools` is defined and never called** (`tools_register.go:1865`). Every gCTS tool upstream is dead code | `internal/mcp/tools_register.go` | f1se4 | one line |
| **Program includes misdetected as class includes.** `/sap/bc/adt/programs/includes/X` contains `/includes/`, so `workflows_edit.go:194`, `SyntaxCheck` and `client.go:167 normalizeObjectURLForPackageCheck` all drop `/source/main` (→ HTTP 406) and collapse the package gate to `/sap/bc/adt/programs`. Guard on `/oo/classes/` *and* `/includes/` | `pkg/adt/workflows_edit.go`, `devtools.go`, `client.go` | enricoandreoli (standalone), frd1201, Augusto42, Basantmh | trivial |
| **`SRVB` is advertised in the read tool description but missing from `routeSourceAction`'s case list** → silently dropped. Also fixes inverted `binding_category` docs (`crud.go:769` claims "0 = Web API"; it is UI) | `internal/mcp/handlers_source.go:21` | lin2qwer1-cloud | trivial |
| **The installer reports success on failed writes.** `WriteSource` returns `(result{Success:false}, nil)` — a **nil error** — and both call sites only check `err != nil`, so `InstallZADTVSP` prints `✓ Deployed` over empty class shells. Present in **three** places: `internal/mcp/handlers_install.go:433`, `cmd/vsp/devops.go:3164` and `:3344` | as listed | blicksten (2 sites), dme007 | 2 h |
| **`--type CLAS --max 10` returns fewer than 10 classes** — `maxResults` is applied server-side *before* the client-side type filter. Needs `CanonicalObjectType` (`CLAS→CLAS/OC`, `FUNC→FUGR/FF`, …) and ADT's real `objectType` parameter. The existing client-side prefix match could never have matched `FUNC` | `pkg/adt/client.go` | frd1201 (richest version), Augusto42 | trivial |
| **`handleCheckBoundaries` calls `GetSource(ctx, "", objectName, nil)`** with an empty type and a hardcoded `objType := "PROG"` | `internal/mcp/handlers_graph.go:90` | f1se4 | trivial |
| **`GetUserTransports` returns empty** where the CTS API answers with an empty document; also released transports are invisible (`TRSTATUS` must be `IN ('R','N')`), `user="*"` matches a literal `'*'`, and the SQL fallback swallows errors into an empty list | `pkg/adt/transport.go` | ovetchenkoandrey (smaller), dme007 (E070/E07T fallback with `sqlSafeValue` escaping and line-wrapping for the 255-char-per-line freestyle limit) | 1 h |
| **`LockResult.CorrNr` is parsed at `crud.go:105` and never used**, so a write with no explicit transport hits a spurious `409 ExceptionResourceLockConflict` even though the lock already told us the transport | `pkg/adt/crud.go` | Neil Ward (`resolveWriteTransport`, and correctly re-runs `checkTransportableEdit` on the resolved value so the gate cannot be bypassed) | small |
| **`.goreleaser.yml` cannot build a working macOS binary** once any cgo path lands — `CGO_ENABLED=0` is set globally | `.goreleaser.yml` | Edgars Dunis | trivial |
| **`PublishServiceBinding` hardcodes `odatav2`** (`crud.go:1094`), so OData V4 bindings cannot be published | `pkg/adt/crud.go` | DRANCOURT Christophe | small |
| **`vsp systems init` writes a file the loader never reads** — it writes `.vsp-systems.json`, which `config.ConfigPaths()` does not search | `cmd/vsp/cli.go:452` | vinchacho | 30 min |
| **`generateRecordingID()` uses a bare timestamp format and collides** | `pkg/adt/recorder.go:104` | Augusto42 | trivial |
| **`ExecuteABAP` discards the activation result** (`_, err = c.Activate(...)`) and cannot distinguish the wrapper's sentinel assertion from a real ABAP Unit failure — **a failing unit test reports success** | `pkg/adt/workflows_execute.go` | Augusto42 | small |

**Two contested items — do not merge without deciding first.**

- **`LockObject` rejects `MODIFICATION_SUPPORT="NoModification"`** (`crud.go:66`, guarded by
  `TestLockObject_RejectsNoModification` at `crud_reconcile_test.go:553`). Three forks argue
  from SAP source that the constant is `CO_MOD_SUPPORT_NOT_NEEDED` — the *normal* value for
  customer `Z*` objects, not an authorization verdict — and txape10 has a `VSP_HTTP_TRACE`
  capture showing `NoModification` with a populated `CORRNR` followed by a **successful
  PUT**. If they are right, upstream is blocking routine Z-object editing. Verify the ABAP
  constant before acting; the fix deletes a test you currently rely on.
- **`SyntaxCheck` before `Lock` vs after.** dme007 and wusxo24 move `SyntaxCheck` ahead of
  `LockObject` in `workflows_deploy.go`; upstream's documented order is Lock → SyntaxCheck.
  This is entangled with the stateless-hop issue below, so resolve them together.

**The most valuable bug *class*: a stateless hop between LOCK and PUT kills the lock.**
Three forks found this independently. A stateless GET landing between the lock and the
write retires SAP's ICM context, so a syntactically-valid lock handle produces a
misleading `423 invalid lock handle`. Edgars' fix is the surgical one: make
`getObjectPackage` call a new `searchObject(..., stateful=true)` (`pkg/adt/client.go`),
with an excellent comment citing live ZED 050 evidence. dme007's blanket
`mutationGateSkipKey` context boolean is the right diagnosis but the wrong instrument — it
bypasses inner-operation policy, so `ExecuteABAP` ends up suppressing inner `OpCreate` /
`OpDelete` checks. Reimplement that half with a scoped `MutationContext`.

## Features worth taking

- **`GetObjectOutline` / `GetObjectProperties` / `GetObjectNetwork` / `GetWhereUsed`** —
  `pkg/adt/objectinfo.go` (650 L, BurnerPat). The only real net-new capability in that
  fork. ~1 day.
- **Where-Used as an MCP tool** — `pkg/adt/whereused.go` (marianfoo). A real
  `informationsystem/usageReferences` implementation with the correct
  `.usagereferences.request.v1+xml` content types and a scope pre-flight. Upstream has
  only a CLI graph command. Same fork also has Enhancement Framework readers, DDIC CRUD
  (`Get`/`Create`/`Validate` for Domain and DataElement) and DDLX metadata extensions —
  13 tools absent upstream, harvestable file by file.
- **ADT endpoint probing → tool filtering** — `pkg/adt/adt_discovery.go` (99 L) +
  `FilterToolsByEndpoints` (BurnerPat). Parse the ATOM discovery document and hide tools
  the system cannot serve. Small, elegant, and a real token win. 2-3 h.
- **Fixture-driven ADT test server** — `internal/testserver/` (848 L) + `cmd/testserver` +
  13 YAML fixtures (BurnerPat). Basic-auth and CSRF emulation, no coupling to the rest of
  the tree. Near drop-in. Half a day.
- **`ActivateMultiple`** (txape10, also cherry-picked into vinchacho) — a single
  `<adtcore:objectReferences>` POST with `preauditRequested=true` so SAP resolves mutual
  dependencies. A program plus its includes genuinely cannot be activated one at a time.
  Minor bug: failure reasons are keyed by `msg.ObjDescr` but read with `ref.Name`. 2-3 h.
- **`CreateStructure`** (ManuelFrieder) — classic DDIC structures (TABL/DS), 690 lines of
  tests, the best coverage in the survey. Notably better than upstream's `CreateTable`:
  `structureComponentDDLType` **rejects** unknown types instead of silently treating a
  typo like `CHAR32X` as a data-element reference. 3-4 h.
- **`--allowed-read-packages`** (Jannes Dailidow) — `SafetyConfig.AllowedReadPackages` +
  `CheckReadPackage`, gated across `handlers_read.go` / `_source` / `_grep` / `_cds` /
  `_analysis`, with tests. Complete, if mechanical; costs a package-resolution round trip
  per read. 2 h.
- **`--mode readonly`** (~46 tools, implies `--read-only`) and a `W` disabled-group for
  hiding write tools from the model's context entirely (marianfoo). Genuinely useful for
  agent safety. Small.
- **`parseADTErrorMessage` + `endpointExists`** (oklausen) — surfaces the ADT
  `<exc:exception>` `localizedMessage` instead of dumping raw XML into `APIError.Message`,
  and replaces four copy-pasted `OPTIONS` + `strings.Contains(err.Error(), "404")` probes
  with one HEAD helper using `errors.As(*APIError)`. Watch one interaction:
  `IsSessionExpired` substring-matches `e.Message`, so add a test. Drop his
  `DBSTATC` / `CL_HDB_%` HANA fallbacks — two free-SQL round trips that die under
  `--block-free-sql`.
- **IAM chain** (Patrick Aagaard) — business catalogs (SIA1), IAM apps (SIA6), catalog
  assignments (SIA7), plus `Client.RawGet`. See the authentication section; it is the last
  mile of RAP publishing and it is better documented than most of the upstream tree.
- **Pre-7.55 PCRE probe** (Phil Barkow, in `embedded/abap/zcl_vsp_apc_handler.clas.abap`) —
  a `class_constructor` runtime test `FIND REGEX '\d' IN '1'` setting `gv_pcre_supported`,
  then swapping `\s` / `\d` for `[[:space:]]` / `[[:digit:]]`. 30 minutes, and it widens
  the supported release floor. Note this **conflicts with** ludecke's approach, which
  instead converts the last `FIND REGEX` stragglers to `FIND PCRE` and assumes 7.55+.
  Pick a floor. ludecke's `COND #(` → `COND string(` hunks are worth taking either way —
  upstream has 8+ bare `COND #(` that do not infer in string-template context.
- **CGO-free SQLite** — `mattn/go-sqlite3` → `modernc.org/sqlite`, DSN rewritten to
  `?_pragma=journal_mode(WAL)`, no build tags or dual-driver hack
  (`Augusto42:codex/cgo-free-sqlite-release`). 1-2 h. Note the tension with the macOS
  keychain work, which *needs* cgo — see the `.goreleaser.yml` fix above.
- **CI workflow** (frd1201) — 27 lines, `go-version-file: go.mod`, CGO enabled so the
  go-sqlite3 tests actually run. Trivial.
- **Dockerfile + entrypoint for streamable HTTP** (ejbrait). Trivial, and pairs with the
  HTTP-transport auth fix, which becomes mandatory the moment vsp runs in a container.

## Designs worth stealing, code worth leaving

- **Impact-gated writes** (vinchacho, `pkg/adt/impact.go` +411, `mutation_gate.go` +160,
  ~2400 lines of tests). Computes a blast-radius summary — where-used count, distinct
  packages, 90-day transport recency — tiers it, and in block mode refuses the write with
  an `ImpactBlockedError` carrying a single-use 128-bit confirmation token you replay via
  `adt.WithImpactConfirm` or an MCP `confirm` parameter. Enforced at the `UpdateSource`
  primitive rather than in handlers, and `impactGateActive` is a deliberate allowlist so
  an unrecognised config value stays inert. Self-aware, good code — but tangled with their
  import-path rewrite and their `SafetyConfig`. **Reimplement, 3-5 days.**
- **MCP tool-permission roles** (BurnerPat, `internal/config/roles.go` 318 L,
  `internal/mcp/permissions.go` 432 L). Named roles, `nested_roles`, glob patterns,
  deny-wins merge, per-tool `allowed_objects` / `blocked_objects`. The design is worth
  stealing; the enforcement is not. Two honest gaps, one of which the code itself admits:
  `allowed_packages` is parsed and documented but **never enforced**
  (`permissions.go:302`), and object extraction is a 7-key guess list
  (`router.go:115 extractObjectName`) so any tool using a different parameter name
  silently bypasses it. Per-role `Safety` is resolved and then never consumed. Your
  existing `SafetyConfig.AllowedPackages` enforces deeper. **Reimplement, 2-3 days.**
- **ENHO / enhancement reading.** Two independent implementations: Phil Barkow's
  `barkow15:feat(adt)/read-enhancements` (801 L + 678 L of tests) and Augusto42's
  `pkg/adt/enhancements.go` (+1178). Barkow's is the better one — a 4-step resolver
  (REST singular → REST plural → RFC-over-WebSocket → structured error) and a
  `D010INC`→`ENHINCINX`→`ENHHEADER` table fallback that recovers the `=`-padded `REPOSRC`
  name the `<NAME>E` convention misses on classic ECC, plus `GetIncludeMerged` /
  `spliceAtAnchor` to reconstruct SE80-merged source. **Two blockers:**
  `include_enhancements` **defaults to true**, silently adding N round trips and a
  WebSocket dial to every grep; and it **stubs out `handle_move_to_package`** in the ABAP
  class, which must not be merged. **Reimplement, large.** From Augusto42's version take
  the endpoint list and splice logic only, and **ignore its `CreateEnhancement`** — it
  generates and runs ABAP on the target via `RFC_ABAP_INSTALL_AND_RUN`.
- **Multi-system switching** (Klairgo — `feat-switch-systems-tool`, `transport-error` and
  `dev` are byte-identical, one commit). `ListSystems` / `SwitchSystem` over a
  `systems map[string]*systemEntry` with lazily built clients, passwords from
  `VSP_<NAME>_PASSWORD`, WS clients nil'd and `FeatureProber` rebuilt on switch, plus a
  cross-system `CompareSource`. Safety config is correctly carried onto each new client.
  Three defects: a `.vsp.json` `default` that disagrees with `cfg.BaseURL` leaves
  `activeSystem` naming system A while `s.adtClient` points at B; `s.adtClient` / `s.config`
  are mutated without a mutex; ~90 duplicated lines. **Reimplement, medium.**
- **`internal/mocksap/`** (Augusto42, 448 + 153 L) — a real offline ADT fixture with
  discovery/CSRF, stateful LOCK/UNLOCK handles, checkruns and a WS RFC path, but written
  around the enhancement work. BurnerPat's `internal/testserver/` is the better starting
  point; take one, not both.
- **Debugger fixes** (Edgars Dunis, via Basantmh). `getStack` was hitting
  `/sap/bc/adt/debugger/stack`, which 404s — the correct form is `POST /sap/bc/adt/debugger`
  with `method=getStack`. Plus `Stateful: true` on attach/stack/step (without it ADT
  answers `500 noSessionAttached`), `isDebuggeeGone()` treating
  `debuggeeEnded` / `DBGSESSIONEND` / `SLAVENOTCONN` as a successful detach, and a paired
  ABAP change resolving class-method breakpoints through
  `cl_oo_classname_service=>get_method_include` — TPDAPI silently never lands a
  method-line breakpoint without `i_include`. Also a `LongPoll` request option backed by a
  second `http.Client` that shares the jar and TLS config but sets `Timeout: 0`, because
  `http.Client.Timeout` is a hard ceiling a longer context cannot lift and upstream's 60 s
  default silently truncates the 240 s debugger listener. **Take as a patch, ~1 day.**
  Live-system findings you cannot derive from documentation.

## For open-rfc-go specifically

Two things beyond the SNC/landscape material already covered.

**The `SADT_REST_RFC_ENDPOINT` wire contract is fully documented in a fork, and it is the
single most reusable artifact in the network for the sibling project.**
`berndeplo:feat/jco-proxy-sidecar` →
`sidecar/jco-proxy/src/main/java/com/sap/mcp/proxy/RestRfcEndpointCaller.java` spells out
the function-module signature that Eclipse ADT uses when the ICM HTTP ports (50000 /
44300) are firewalled but the gateway port 33xx is open:

- `REQUEST` structure containing `REQUEST_LINE` (`METHOD` / `URI` / `VERSION`), a
  `HEADER_FIELDS` table of `NAME` / `VALUE` rows, and `MESSAGE_BODY` as raw bytes.
- `RESPONSE` mirrors it, with `STATUS_LINE.STATUS_CODE` returned as a **string** (their
  code parses it and defaults to 500) plus `REASON_PHRASE`.

That is enough to implement ADT-over-RFC natively in open-rfc-go with no Java at all —
which would let vsp reach a system through the gateway port alone, over the SAProuter
path open-rfc-go already has. Strategically this is the most interesting single finding
in the survey after the mTLS work.

`StatefulSessionManager.java` is the companion piece and explains a constraint you will
hit: `JCoContext` is thread-local, so each stateful session needs a **dedicated
single-threaded executor** to keep LOCK → PUT → UNLOCK on one connection, with an idle
reaper (theirs is 5 minutes). A Go port maps that onto one goroutine per session owning
one connection — the same shape, more naturally expressed.

**Three defects in the sidecar not to copy**: `--password` on the Java argv (readable via
`ps` by any local user); `killOrphanedSidecars()` doing `pgrep -f RfcProxyServer` with no
UID filter, so it kills every match on the box including other users'; and **no
authentication on the sidecar's HTTP port**, so any local process can issue RFC calls as
the logged-in SAP user. Also `ProxyResponse.Headers` is a `map[string]string`, which
collapses multi-valued `Set-Cookie`, and `RfcTransport` overwrites any caller-supplied
`Cookie` header.

A bonus find in the same branch: `handleSetBreakpointRfc` drives the debugger through the
plain ADT REST breakpoint API rather than the `ZADT_VSP` WebSocket helper class — i.e.
breakpoints without the custom ABAP class installed at all.

---

# Harvest plan

Ranked. "Take" means the patch can go in close to verbatim; "reimplement" means the design
is worth following but the code should not be copied.

## Tier 0 — do these first (about a day and a half total)

| # | Item | Action | Effort | Author |
|---|---|---|---|---|
| 1 | **Auth on the Streamable HTTP transport** (`apiKeyMiddleware`, `originValidationMiddleware`, RFC 9728 metadata) — closes a live DNS-rebinding / open-port hole | take | 2-3 h | Marian Zeis |
| 2 | **`parseActivationResult` parses nothing** — every activation silently reports success | take | 30 min | Edgars Dunis |
| 3 | **WebSocket path ignores `HTTP(S)_PROXY`** — debugger/AMDP/RFC unusable behind a proxy | take | 30 min | Dominik Miescher |
| 4 | **`registerGCTSTools` never called** — all gCTS tools are dead code | take | 5 min | f1se4 |
| 5 | **Program includes misdetected as class includes** → HTTP 406 + broken package gate | take | 30 min | enricoandreoli |
| 6 | **CSRF `HEAD`→`GET` fallback** (+ skip on 401/403) merged with **CSRF session-mode binding** — vsp is unusable on BASIS 740 / ECC EhP7 today | take, merged | 2 h | Ladislav Rydzyk, Fabian Diehl, Augusto42 |
| 7 | **Installer reports success on failed writes** — fix all three call sites | take | 2 h | blicksten, Dominik Miescher |
| 8 | **`SRVB` missing from `routeSourceAction`** + inverted `binding_category` docs | take | 15 min | lin2qwer1-cloud |
| 9 | **`vsp systems init` writes a file the loader never reads** | take | 30 min | Vincent Segami |

## Tier 1 — the authentication harvest (about a week)

| # | Item | Action | Effort | Author |
|---|---|---|---|---|
| 10 | **macOS Keychain mTLS client certificates** — `keychain_darwin.go`, `ClientCertProvider`, lazy per-handshake resolution, the TLS 1.2 ICM pin, WS cert mode. Fix `LoadKeychainClientCertByIssuers` in `keychain_other.go` first (the branch does not compile off macOS), add tests, and pair with the `.goreleaser.yml` cgo fix | take | 1 day | Edgars Dunis |
| 11 | **Native reentrance-ticket SSO** — system browser, `virtualhost` API/UI resolution, loopback callback, `MYSAPSSO2` exchange. Supersedes chromedp for MFA systems | take | 1 day | BurnerPat |
| 12 | **Session/cookie hardening bundle** — `Secure`-strip over plain HTTP, `clearSAPSessionCookies` on `ICMENOSESSION`, cookie-jar reset on 401, redirect preservation of `X-CSRF-Token` / `X-sap-adt-sessiontype`, BAS `sap-contextid` guard. Take **one** `Secure`-strip implementation; note `clearSAPSessionCookies` swaps in a plain jar and would silently drop a `Secure`-stripping wrapper | take, deduped | 1 day | Basantmh / Benjamin Bockmuehl / Fabian Diehl / Paul Snyman / Dominik Miescher |
| 13 | **Stateless-hop-kills-the-lock** — Edgars' stateful `searchObject` in `getObjectPackage`; resolve the Lock↔SyntaxCheck ordering question at the same time | take + decide | 4 h | Edgars Dunis, Dominik Miescher |
| 14 | **SAP UI Landscape + SNC discovery** — `landscape.go`, `landscape_{windows,notwindows}.go`, `findSNCLibrary()`. Take the cwbr or BurnerPat copy (berndeplo's fork has no LICENSE) | take, minus the JCo half | half a day | Patrick Jackes, Benjamin Bockmuehl |
| 15 | **File-based client cert + custom CA** (`WithClientCert` / `WithCACert`) — the Linux/CI counterpart to item 10. Make the error path fail hard instead of stashing into `OAuthError` | take | 2 h | Marian Zeis |
| 16 | **WebSocket `SetCookies`** — the WS bridge is Basic-auth-only, locking every SSO/cookie user out of the debugger | take | 2 h | Phil Barkow |
| 17 | **`VSP_HTTP_TRACE`** redacted request/response dump — the tool you need to debug all of the above | take | 1 h | Dominik Miescher |
| 18 | **A `Credential` interface** unifying basic / cookie / SAML / browser / mTLS / bearer / proxy-token, with one TLS-config constructor and a WS-aware application point | reimplement | 3-4 days | — (design synthesised from the above) |

## Tier 2 — cloud and identity (two to three weeks, and the strategic bet)

| # | Item | Action | Effort | Author |
|---|---|---|---|---|
| 19 | **BTP Destination + XSUAA + Cloud Connector proxy.** Follow Prolls' structure; fix `Proxy-Authorization` to use `GetProxyConnectHeader` for `CONNECT`, cache tokens against `expires_in`, rewind bodies via `GetBody` on 407 retry, add `SAP-Connectivity-SCC-Location-ID`, plumb `context.Context`, and write the tests neither fork has | reimplement | 3-4 days | DRANCOURT Christophe, Marian Zeis |
| 20 | **OIDC / JWT bearer validation** on a real library (`go-jose` / `golang-jwt`) — reject tokens with no `exp`, add clock-skew leeway, make the claim→SAP-user mapping an explicit policy rather than an uppercase fallback | reimplement | 2-3 days | Marian Zeis |
| 21 | **Principal propagation** — ephemeral X.509 with `CN=<user>` signed by a STRUST-trusted CA, mapped back via CERTRULE. Rebuild with a per-user (cert, `http.Client`) cache so `sap-contextid` survives and RSA keygen stops happening per request | reimplement | 1 week | Marian Zeis |
| 22 | **Secret storage** — refuse group/world-readable `.vsp.json`, warn on any inline `password`, promote `credential_cmd` to the documented default, then native keystore backends (macOS Keychain via the `certstore` dep already pulled in, Windows Credential Manager, Secret Service) | new work | 3-4 days | — |
| 23 | **ADT-over-RFC natively in open-rfc-go**, from the documented `SADT_REST_RFC_ENDPOINT` contract. No Java, no JCo | reimplement | 1-2 weeks | contract by Benjamin Bockmuehl |

## Tier 3 — features, in rough value order

| Item | Action | Effort | Author |
|---|---|---|---|
| Debugger fixes (`getStack` path, stateful verbs, `isDebuggeeGone`, method-include breakpoints, `LongPoll`) | take | 1 day | Edgars Dunis |
| IAM chain — business catalogs / IAM apps / assignments + `RawGet` | take | — (already complete) | Patrick Aagaard |
| `objectinfo.go` — outline / properties / network / where-used | take | 1 day | BurnerPat |
| Fixture-driven ADT test server | take | half a day | BurnerPat |
| Where-Used MCP tool + Enhancement Framework readers + DDIC CRUD (13 tools) | take, file by file | 2-3 days | Marian Zeis |
| ADT endpoint probing → tool filtering | take | 2-3 h | BurnerPat |
| `CreateStructure` (DDIC structures, 690 L of tests) | take | 3-4 h | ManuelFrieder |
| `ActivateMultiple` (dependency-resolving batch activation) | take | 2-3 h | txape10 |
| `--allowed-read-packages` | take | 2 h | Jannes Dailidow |
| `--mode readonly` + `W` disabled-group | take | small | Marian Zeis |
| `resolveWriteTransport` (reuse `LockResult.CorrNr`) | take | small | Neil Ward |
| `GetUserTransports` DUM/E070 fallback + released-transport visibility | take | 1 h | ovetchenkoandrey, Dominik Miescher |
| `parseADTErrorMessage` + `endpointExists` (drop the HANA free-SQL fallbacks) | take | small | oklausen |
| `CanonicalObjectType` + `SearchObjectByType` | take | 1 h | Fabian Diehl |
| ABAP `COND string(` fixes; PCRE floor decision (runtime probe vs 7.55+) | take | 30 min | ludecke / Phil Barkow |
| CI workflow with CGO on | take | trivial | Fabian Diehl |
| Dockerfile + entrypoint | take | trivial | ejbrait |
| CGO-free SQLite (weigh against the keychain cgo requirement) | take | 1-2 h | Augusto42 |
| SRIS `SourceSearch` with graceful 404/501 degradation | take | 2-3 h | blicksten |
| `PublishServiceBinding` OData V4 | take | small | DRANCOURT Christophe |
| `feat/incl-write-support` — land the `/includes/` disambiguation separately first | take | small | Fabian Diehl |
| `corrNr` at LOCK time (breaks `LockObject` across ~22 call sites) | take | small | Fabian Diehl |
| Impact-gated writes | reimplement | 3-5 days | Vincent Segami |
| ENHO/ENHS reading (4-step resolver + table fallback + `spliceAtAnchor`) | reimplement | large | Phil Barkow |
| MCP tool-permission roles | reimplement | 2-3 days | BurnerPat |
| Multi-system switching | reimplement | medium | karel.olwage |
| `feat/object_ref` object-ref unification (net −200 lines) | reimplement | not urgent | BurnerPat |
| `.goreleaser.yml` `changelog.use: git` (git-cliff is Pro-only — a genuine upstream bug) | take | 5 min | BurnerPat |

## Explicitly ignore, with reasons

- **The Java JCo sidecar** (cwbr / berndeplo / BurnerPat) — a JRE dependency, a 6.2 MB
  binary committed to git, passwords on `argv`, an unauthenticated local RFC port, and a
  `pgrep`-based orphan killer with no UID filter. It is also the exact thing open-rfc-go
  exists to replace. Take the documented FM contract, leave the implementation.
- **`cmd/setup-certrule/main.go`** (marianfoo) — a throwaway probe with hardcoded live
  credentials.
- **`CreateEnhancement`** (Augusto42) — generates and runs ABAP on the target via
  `RFC_ABAP_INSTALL_AND_RUN`.
- **The intelligence layer** (blicksten: `impact.go` / `regression.go` / `sqlperf.go`) —
  parses ABAP signatures with regular expressions, which is what `pkg/abaplint` exists to
  avoid; issue #86 is already closed. Its refactoring v2 has no rollback on a partial
  multi-object rename; #82 is already closed.
- **Architectural churn** — BurnerPat's config unification, `pkg/adt/connection/` package
  split and `handlers_*.go` → `tools/*.go` colocation. Reasonable taste, but 309 commits
  of drift against your tree for no functional gain.
- **`marianfoo/main` wholesale** — it deletes the debugger, git, LSP, WASM compiler,
  workflow, DSL, reports, help and install layers. Cherry-pick files only.
  `fix/GetDependencyZIP` adds a `GetDependencyZIP` that returns `nil` for every input.
- **Already upstream**: `cwbr:feature/browser-based-sso` (landed as PR #77, and it is the
  ancestor of today's `browser_auth.go`), `danielheringers:feat/mcp-go-0.43.2-http-streamable`
  and marianfoo's HTTP-streamable commits (upstream is on mcp-go v0.47.0),
  `marianfoo:fix/debugger-get-variables-array-schema` (`mcp.Items` is at
  `tools_register.go:725`), `marianfoo:feat/auto-release-changelog` (`cliff.toml` +
  `release.yml` present), `oklausen:docs/copilot-instructions`, `Prolls:feature/gcts-tools`
  and `feature/i18n-tools`.
- **Fork-local customisation** — `f1se4:tradebe-customizations` (flips
  `--enable-transports` / `--allow-transportable-edits` / `--mode expert` on by default,
  and its `resolveConfig` `||` rewrite breaks flag-beats-env precedence — but it *hides*
  two real bugs worth taking), `luketebo/BizVibe-Tools`, `blicksten`'s
  `.claude/agents/*.md` sync commits, README rebrands, `FORK.md` files, fork governance
  reports, `.goreleaser.yml` owner swaps, generated CHANGELOGs.
- **Committed binaries and leaked data** — `Prolls` ships `vsp_linux` / `vsp_cf_binary`
  placeholders; `vinchacho` has a stray shell-session transcript at repo root;
  `blicksten`'s `edge-with-profile.cmd` hardcodes a personal Windows path and its
  `.env.example` leaks an internal hostname. Read every harvested file rather than
  trusting a branch.
- **`gcggyl28`** — an LLM read "steampunk" literally and produced a boiler/piston/governor
  /flywheel simulation package with tests. Nothing to do with SAP.

## Licensing and attribution

**Every fork examined that has a LICENSE file is MIT and unmodified**, retaining
`Copyright (c) 2025-2026 Alice Vinogradova and contributors`. Nobody attempted to
relicense. `BurnerPat/vsp-enterprise` goes further with a 154-line `FORK.md` naming
upstream explicitly. **One exception: `berndeplo/vibing-steampunk` has no LICENSE file at
all** — take the SNC/landscape code from `cwbr` or `BurnerPat`, whose copies are covered.

**Into vsp (MIT → MIT):** no licence friction. Credit by `Co-authored-by:` trailers on the
harvest commits, using the identities recorded in the fork commits:

```
Co-authored-by: Edgars Dunis <edgars.dunis@zalaris.com>
Co-authored-by: Dominik Miescher <...>
Co-authored-by: Fabian Diehl <Fabian.Diehl@monads.ch>
Co-authored-by: Patrick Aagaard <patrickaagaard@me.com>
Co-authored-by: Jannes Dailidow <jannes@dailidow.de>
```

Where the design is reimplemented rather than copied, a file-header comment naming the
fork, branch and author is the honest form — and cheaper than a trailer that implies
code provenance you did not take. For the larger contributions (Edgars' mTLS stack,
BurnerPat's reentrance SSO, Marian Zeis' identity stack, Patrick Aagaard's IAM chain) the
better move is to **open an issue crediting the author and inviting them to send the PR
themselves**. That is more respectful than harvesting, it gets you a maintainer for the
code, and several of them are clearly still active — `Basantmh`, `Augusto42`, `dme007`,
`vinchacho` and `BurnerPat` all pushed within the last week.

**Two attribution flaws to be aware of when crediting:**

- `Basantmh:bdb04059 feat: add vsp-abap-developer Claude Code plugin` is authored *Basant
  Singh* but its file set is byte-identical to `vinchacho:84627276`. Credit Vincent Segami.
- Several `fix(adt)` / `fix(mcp)` commits in `vinchacho` are cherry-picks from `Augusto42`
  and `txape10` that retain original authorship in the commit metadata but are easy to
  double-count. Deduplicate before writing trailers.

**Into open-rfc-go (MIT → Apache-2.0):** permitted, but the MIT notice must travel. Add
the original copyright to `NOTICE` and a provenance header to each ported file, in the
style `docs/provenance.md` already uses for the open-rfc port. The reverse direction is
**not** available — nothing Apache-2.0 may flow back into MIT-licensed vsp. Also note
open-rfc-go requires a DCO sign-off, which a `Co-authored-by` trailer does not supply;
for anything landing there, either reimplement from the design under your own sign-off or
invite the original author to submit it themselves.

**Third-party dependencies introduced by harvested code:**

- `github.com/github/smimesign/certstore` (MIT) — the macOS Keychain path. Fine, but it is
  a dormant repository; vendor it or pin it hard, and it drags cgo back in.
- `modernc.org/sqlite` (BSD-3-Clause) — the CGO-free SQLite swap.
- `sapjco3` — proprietary SAP, correctly scoped `provided` in the sidecar's `pom.xml`, so
  nothing proprietary is redistributed. Moot if the sidecar is ignored, as recommended.

---

## Appendix — method

- Fork list: `gh api repos/oisee/vibing-steampunk/forks --paginate` (107 rows).
- Every fork's default branch compared with
  `gh api repos/oisee/vibing-steampunk/compare/main...<owner>:<repo>:<branch>`.
- Every fork's branch list enumerated (`/branches`), filtered against upstream's own 12
  branch names, and each remaining branch compared the same way — 63 non-upstream branches
  in total. Work sitting outside `main` accounted for most of the interesting findings,
  including the mTLS branch, the SNC branch and every Cloud Connector commit.
- Ahead-counts corrected for the upstream history rewrite by filtering commits to those
  actually authored by the fork owner.
- Every claim of "upstream does not have this" was checked against the working tree at
  `0ac7d64`, not assumed from the diff.
- Read-only throughout: no PRs, issues, comments, pushes, or remotes added.
