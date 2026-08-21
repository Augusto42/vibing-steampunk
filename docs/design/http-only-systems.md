# Systems where you have a cookie but no RFC

The RFC leg assumes you can log on to the gateway with a user and a password.
On plenty of real systems you cannot: you reach ADT through a browser-style
single sign-on, the tool ends up holding a cookie, and nobody has an RFC
password to give you. This note answers what carries across, what does not, and
what to do instead — and it is deliberately explicit about which parts are
established and which need testing on such a system.

## Does an HTTP authentication carry into classic RFC?

| What the HTTP side holds | Carries into RFC? | Why |
|---|---|---|
| **Basic auth** — a user and a password | **Yes** | the same credentials are what an RFC logon wants; the user still needs `S_RFC` |
| **`SAP_SESSIONID_<SID>_<CLNT>`** | **No** | it names an ICF session. It is not a credential, and classic RFC has no concept it maps onto |
| **`MYSAPSSO2`** — an SAP logon ticket | **In principle yes**, and it is the only real bridge | RFC logon by ticket is a supported thing: the SAP client libraries take the ticket in place of a password. It needs the system to issue tickets (`login/create_sso2_ticket`) and to accept them (`login/accept_sso2_ticket`), and the ticket is bound to issuer, client and user with a short life. **Our client does not implement it** — see below |
| **SAML 2 assertion, OIDC / JWT bearer** | **No** | there is no classic-RFC logon that takes them. On BTP ABAP there is no classic RFC at all |
| **Kerberos / SPNEGO, X.509 client certificate** | **Not for us** | RFC does carry these, but only through SNC, which needs SAP's CommonCryptoLib. This client is SDK-free and has no SNC |

So the short answer: **unless the session also carries `MYSAPSSO2`, a cookie
does not become an RFC logon.** Expect no classic RFC on such a system.

### What it would take to support ticket logon

The ticket travels as a field of the CPIC logon in place of the password. We
know the shape of the logon we send today because we captured it; we have never
captured a ticket logon, so the field is unknown to us. It is a research task of
exactly the kind `cmd/rfc-lab` exists for: point an SM59 destination configured
for ticket logon (or a SAP GUI session with SSO) at the sniffer, capture the
logon, and read it. Until someone does that, treat ticket-based RFC as *not
supported* rather than *not possible*.

## What to do instead, in order of effort

### 1. Notice that most of what we built does not need RFC

This is the important one. The debugger flow — listener, attach, stack, step,
variables — is **SAP's own ADT REST resources**. RFC was only the transport we
tunnelled them through. Over HTTPS the same resources answer directly; the only
thing that made a stateless client unable to use them is that ADT keeps the
debug session in an ABAP roll area and finds it again through `sap-contextid`.

vsp already models that: `pkg/adt` has `SessionStateful` and the contextid
handling. What is missing is a **process that holds one session for the whole
loop**, exactly as `vsp rfc debug` holds one pinned RFC conversation. The
operations in `pkg/saprfc/adtdebug.go` are written against an envelope, not
against RFC — pointing them at a stateful HTTP transport is a small change, and
it gives a cookie-only system the same debugger.

**Proposed:** `vsp adt debug`, the same REPL, transport chosen by flag.

### 2. Try the SOAP RFC endpoint

`/sap/bc/soap/rfc` is the classic ICF endpoint that exposes RFC-enabled function
modules over HTTP. Where it is active, a cookie is enough to call **any**
RFC-enabled FM — `RFC_READ_TABLE`, the XBP job BAPIs, our own facade — without
the gateway port and without an RFC password. It is often switched off, and it
is worth ten minutes to find out, because it would restore the whole RFC feature
set on an HTTP-only system.

**Untested by us.** The probe is in the next section.

### 3. Let Eclipse hold the session

The [`vsp-ide`](https://github.com/oisee/vsp-ide) plugin exists for this shape:
Eclipse has already authenticated, by whatever mechanism the landscape uses, and
a small bundle lets an outside tool borrow that connection without extracting
anything from it.

### 4. Ask for a technical user

Unromantic and often fastest. An RFC user with `S_RFC` for the function groups
in use, and the RFC leg works as designed. Worth asking for explicitly rather
than engineering around.

## What to test on such a system, in this order

Each step is cheap and the order is chosen so that a positive answer early saves
the rest.

1. **Is the gateway reachable at all?** `nc -vz <host> 33<nn>`. Network first —
   it may be that RFC is fine and only the credentials were the problem.
2. **Which cookies does the session actually carry?** In the browser's dev tools
   or Eclipse's connection: is there a `MYSAPSSO2` beside the
   `SAP_SESSIONID_…`? If yes, ticket-based RFC becomes worth pursuing; if no,
   stop considering it.
3. **Is `/sap/bc/soap/rfc` alive?** A `POST` with a minimal SOAP envelope calling
   `RFC_PING`, using the same cookie the browser has. A `200` with a SOAP
   response, or even a SOAP fault about the payload, means the endpoint is
   active and the door is open. A `404` or an ICF error page means it is not.
4. **Does a stateful ADT session survive several calls in one process?** Take
   the lock-then-write sequence we ran over RFC — `_action=LOCK`, then a source
   `PUT` with that handle — and run it over HTTPS from a single vsp process with
   `SessionStateful`. It should behave the same; if it does, the debugger will
   too.
5. **Then the debugger**, over HTTPS: `POST /sap/bc/adt/debugger/listeners`,
   attach, stack. Same requests as `dbg> eclipse`, different transport.

Record the answers; they decide which of the four routes above is the one for
that landscape.
