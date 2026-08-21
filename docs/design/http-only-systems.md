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

### Measured, 2026-08-21, on A4H

A logon ticket issued by the web logon was tested directly. Two results.

**Over HTTP it is a complete credential.** With nothing but
`MYSAPSSO2=<ticket>` as a cookie — no user, no password anywhere in the
configuration — `vsp adt debug` ran the whole debugger loop: the listener caught
a debuggee, attached to it, and returned the stack. So on a cookie-only system
the ADT route works with exactly what the browser already has.

A ticket is **not bound to a hostname.** It is bound to the issuing system, the
client and the user, and it is signed; the cookie's domain only constrains a
*browser*. When the tool sets the header itself, any address that reaches the
same system will do — the ticket used above was issued against one hostname and
presented to another. (For a *different* system to accept it, that system needs
the issuer's certificate in its ACL — `STRUSTSSO2` — and if
`login/ticket_only_by_https` is set, plain HTTP is refused.)

Two details worth knowing before you try it. The cookie value is **not plain
base64**: SAP substitutes `!` for `/`, so a decoder must undo that (and the
value is URL-encoded on top). And the readable header of the ticket carries the
user, the client, the system id and the creation time in UTF-16, which is a
quick way to check *whose* ticket you are holding and how old it is.

**Over RFC the system would accept it; our client cannot send it.** The relevant
profile parameters on this system, read with `TH_GET_PARAMETER`:

| Parameter | Value |
|---|---|
| `login/create_sso2_ticket` | `2` — tickets are issued, with a digital signature |
| `login/accept_sso2_ticket` | `1` — **logon by ticket is accepted** |
| `login/ticket_expiration_time` | `8:00` |

So the gate is on our side. SAP's own client libraries take the ticket as a
connection parameter (`MYSAPSSO2`; JCo spells it `jco.client.mysapsso2`), which
means the protocol carries it — we have simply never captured a logon that does,
so the field is unknown to us.

**How to find out, with the kit we already have.** An SM59 type-3 destination
has a *Send SAP Logon Ticket* flag. Set it on a destination that points at
`cmd/rfc-lab`'s sniffer, call anything through it from a session that holds a
ticket, and the ticket-bearing logon lands in the capture; `cmd/rfc-viewer`
then shows the field. After that, supporting it is a small change to the logon
builder. Until someone does that, treat ticket-based RFC as **not supported**
rather than not possible.

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

**Done**, 2026-08-21: `vsp adt debug` — the same REPL, the transport chosen by
which command you run. Verified against A4H over plain HTTPS: the listener
caught a debuggee raised by a function module called from elsewhere, attached to
it, and returned the same five-frame stack the RFC path returns. Nothing in the
debug path touched RFC.

### 2. The SOAP RFC endpoint — tested, and it works

`/sap/bc/soap/rfc` is the classic ICF endpoint that exposes RFC-enabled function
modules over HTTP. It is often switched off; where it is active, **a cookie is
enough to call any RFC-enabled function module**, with no gateway port and no
RFC password.

Verified on A4H, 2026-08-21, authenticated by nothing but a logon ticket:

```sh
curl -X POST "http://host:50000/sap/bc/soap/rfc?sap-client=001"   -H 'Content-Type: text/xml; charset=utf-8' -H 'SOAPAction: ""'   -H "Cookie: MYSAPSSO2=$TICKET" --data-binary @- <<'XML'
<?xml version="1.0" encoding="UTF-8"?>
<soap:Envelope xmlns:soap="http://schemas.xmlsoap.org/soap/envelope/"><soap:Body>
  <RFC_SYSTEM_INFO xmlns="urn:sap-com:document:sap:rfc:functions"/>
</soap:Body></soap:Envelope>
XML
```

`RFC_PING`, `RFC_SYSTEM_INFO` (with its nested `RFCSI_EXPORT` structure) and our
own `ZADT_DEBUG_RFC` facade all answered `200`. So on a cookie-only system this
restores the whole RFC feature set: `RFC_READ_TABLE`, the XBP job BAPIs,
breakpoint management — anything remote-enabled.

Two limits. The endpoint is **stateless**: every call is its own ABAP session,
so the facade's session-bound operations (attach, step) cannot work through it —
which costs nothing, because the ADT route above already covers those over
HTTPS. And the envelope is SOAP, so parameter marshalling is XML rather than the
RFC codecs: a Go client for it is a modest amount of work, not free.

**Worth building:** a third transport for `vsp rfc call` / `read-table` /
`describe` that speaks this endpoint, so the RFC commands work unchanged on a
system where the gateway is unreachable.

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
