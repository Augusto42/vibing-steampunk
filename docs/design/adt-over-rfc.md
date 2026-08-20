# ADT over RFC: a real REST request through the classic-RFC tunnel

*Findings, 2026-08-21. Verified live against A4H (`a4h.desude.su`, SAP_BASIS 758,
kernel 793, client 001). Companion to [`rfc-opportunities.md`](rfc-opportunities.md).*

**It works.** `GET /sap/bc/adt/discovery` returns HTTP 200 with a 299,191-byte
`application/atomsvc+xml` body, and an ADT source read returns the program text with
its `ETag` and `Last-Modified` headers — over classic RFC only, with no ICF, no HTTP
port and no SAP SDK. That makes ADT reachable on systems where HTTP is closed.

```
$ vsp rfc adt GET /sap/bc/adt/discovery -H Accept=application/atomsvc+xml -o discovery.xml
HTTP/1.1 200 OK
~server_protocol: HTTP/1.1
Content-Type: application/atomsvc+xml
(299191 bytes)

$ vsp rfc adt GET /sap/bc/adt/programs/programs/RSUSR000/source/main -H Accept=text/plain
HTTP/1.1 200 OK
~server_protocol: HTTP/1.1
Last-Modified: Tue, 01 Aug 2023 16:39:05 GMT
ETag: 202308011639050011
Content-Type: text/plain; charset=utf-8
(28537 bytes)
************************************************************************
*  Currently Active Users.
...
```

A miss behaves like a miss: `GET /sap/bc/adt/programs/programs/ZZZ_NOT_THERE/source/main`
returns `HTTP/1.1 404 Not Found` with the ADT `exc:exception` XML body, and the command
exits non-zero.

---

## 1. The door: `SADT_REST_RFC_ENDPOINT`

The endpoint is SAP's own. It takes an HTTP request envelope and returns an HTTP
response envelope, dispatching into the very handlers the ICF nodes under
`/sap/bc/adt/` serve — this is how Eclipse ADT reaches a system whose HTTP port is
shut.

| Parameter | Class | DDIC type | Components |
|---|---|---|---|
| `REQUEST` | IMPORTING | `SADT_REST_REQUEST` (EXID `v`) | `REQUEST_LINE` (`SADT_REST_REQUEST_LINE`: `METHOD`, `URI`, `VERSION`, all `STRING`), `HEADER_FIELDS` (`TIHTTPNVP`, a table of `IHTTPNVP` `NAME`/`VALUE`), `MESSAGE_BODY` (`XSTRING`) |
| `RESPONSE` | EXPORTING | `SADT_REST_RESPONSE` (EXID `v`) | `STATUS_LINE` (`SADT_REST_STATUS_LINE`: `VERSION`, `STATUS_CODE`, `REASON_PHRASE`), `HEADER_FIELDS` (`TIHTTPNVP`), `MESSAGE_BODY` (`XSTRING`) |

Both are deep structures (EXID `v`) that reach a nested structure, a nested table and
two `XSTRING` cells, so they have no fixed-width form at all. They exist only on the
xRFC XML codec against an `RFC_METADATA_GET` (DEEP) type graph.

Note the flat DDIC view disagrees with the graph, which is the whole reason
`RFC_GET_STRUCTURE_DEFINITION` fails on these types: `SADT_REST_REQUEST_LINE` is an
`.INCLUDE`, so its `METHOD`/`URI`/`VERSION` appear *both* as the substructure and as
their own components, and their `RFC_FIELDS` offsets therefore overlap
(`RFC_FIELDS METHOD overlaps its preceding field`).

## 2. What `FMODE = 'X'` turned out to mean

`TFDIR-FMODE` has **two** remote values, not one:

| `FMODE` | Meaning | Examples on A4H |
|---|---|---|
| `' '` | Not remote-enabled | (most FMs) |
| `'R'` | Remote-enabled module | `STFC_CONNECTION`, `RFC_READ_TABLE`, `RFC_METADATA_GET`, `SADT_CORE_INVALIDATE_REG_RFC` |
| `'X'` | Remote-enabled module, **basXML-capable interface** | `SADT_REST_RFC_ENDPOINT`, `SADT_PROTECTED_DISCOVERY`, `BICS_PROV_OPEN`, `AGRDIST_SEND` |

`'X'` is not a restriction and does not mean "local". SAP sets it on modules whose
interface carries deep/nested parameters, which is exactly the shape that needs basXML
(or, for a classic client like ours, the xRFC XML fallback). Proof that it is callable
over plain classic RFC is the 200 above.

The mapping is confirmed from the other side: `RFC_GET_FUNCTION_INTERFACE` returns
`TFDIR-FMODE` verbatim in `REMOTE_CALL` and sets `REMOTE_BASXML_SUPPORTED` exactly when
it is `'X'` —

| FM | `REMOTE_CALL` | `REMOTE_BASXML_SUPPORTED` | `TFDIR-FMODE` |
|---|---|---|---|
| `STFC_CONNECTION` | `R` | false | `R` |
| `RFC_METADATA_GET` | `R` | false | `R` |
| `SADT_REST_RFC_ENDPOINT` | `X` | true | `X` |
| `SADT_PROTECTED_DISCOVERY` | `X` | true | `X` |
| `BICS_PROV_OPEN` | `X` | true | `X` |

`TFDIR-FMODE`'s domain is a bare `CHAR1` with no fixed values, so there is no DDIC value
help to read this off; the data element text is only "Type of function module (local,
remote, ...)". The table above is the evidence.

**This is how the endpoint got mistaken for a local module.** Both `vsp rfc search` and
`rfc search` filtered TFDIR on `FMODE = 'R'`, so `SADT_REST*` returned *nothing*:

```
$ rfc search 'SADT_REST*'          # before
null
$ rfc search 'SADT_REST_RFC*'      # after
[ { "FUNCNAME": "SADT_REST_RFC_ENDPOINT", "PNAME": "SAPLSADT_REST" } ]
```

Both tools (and the MCP tool listings behind them) now filter on
`FMODE IN ( 'R', 'X' )`.

## 3. Diagnosis: the recursive path was fine; the base64 was not

The brief expected the recursive metadata path not to engage. It does. Instrumented
against the live system, `Client.planLayoutOn` puts **both** parameters on the recursive
codec:

```
iface SADT_REST_RFC_ENDPOINT remoteCall="X" basxml=true
  param class=E name=RESPONSE tab=SADT_REST_RESPONSE exid="v"
  param class=I name=REQUEST  tab=SADT_REST_REQUEST  exid="v"
  resolve SADT_REST_RESPONSE: ERR RFC_FIELDS VERSION overlaps its preceding field
  resolve SADT_REST_REQUEST:  ERR RFC_FIELDS METHOD overlaps its preceding field
recursiveGraphOn err = <nil>
  graph version=1 nodes=6 params=2 identity=SADT_REST_RFC_ENDPOINT
  nodes: [IHTTPNVP SADT_REST_REQUEST SADT_REST_REQUEST_LINE
          SADT_REST_RESPONSE SADT_REST_STATUS_LINE TIHTTPNVP]
  ResolveParameter RESPONSE -> needed=true kind=structure node=SADT_REST_RESPONSE
  ResolveParameter REQUEST  -> needed=true kind=structure node=SADT_REST_REQUEST
PLAN recursive = [RESPONSE REQUEST]
```

`RFC_METADATA_GET` succeeds through the hardcoded bootstrap descriptor (the earlier
"Wrong parameter type in an RFC call" came from calling it *by hand* through the generic
CLI path, whose flat resolver builds the wrong row layout — the bootstrap does not).
`rfc describe SADT_REST_RFC_ENDPOINT` already rendered both parameters as nested JSON
Schema; the `(unresolved)` report predates the recursive-metadata merge.

**The request went out and the answer came back.** The generated request was

```xml
<REQUEST><REQUEST_LINE><METHOD>GET</METHOD><URI>/sap/bc/adt/discovery</URI>
<VERSION>HTTP/1.1</VERSION></REQUEST_LINE><HEADER_FIELDS><item><NAME>Accept</NAME>
<VALUE>application/atomsvc+xml</VALUE></item></HEADER_FIELDS>
<MESSAGE_BODY></MESSAGE_BODY></REQUEST>
```

and the server answered a 404,512-byte xRFC XML `RESPONSE` beginning

```xml
<RESPONSE><STATUS_LINE><VERSION>HTTP/1.1</VERSION><STATUS_CODE>200 </STATUS_CODE>
<REASON_PHRASE>OK</REASON_PHRASE></STATUS_LINE>…<MESSAGE_BODY>PD94bWwgdmVyc2lvbj0i…
```

Only the **decode** failed:

```
rfc: protocol error: RESPONSE: xrfc: malformed xRFC XML:
     RESPONSE.MESSAGE_BODY contains non-canonical base64
```

### Root cause

The ABAP xRFC serializer emits an `XSTRING` cell as **base64 MIME line-wrapped at 76
columns with a bare LF**. Measured on this response: the cell is 404,172 characters in
**5,249 lines of exactly 76 characters**, 398,924 characters once joined, decoding to
299,191 bytes. No CR, no spaces, no tabs.

Both xRFC decoders — `DecodeBase64` in `internal/xrfc/classic_xrfc.go` and
`recursiveBase64Decode` in `internal/xrfc/recursive_xrfc_codec.go`, ported from
`open-rfc` — validated the **raw** cell text: `length % 4 == 0`, a strict
`^(?:[A-Za-z0-9+/]{4})*(?:…=|…==)?$` alphabet regex, then a re-encode round trip. A
wrapped value fails all three. The practical effect is that **every `XSTRING` longer
than 57 bytes was undecodable** — i.e. every real HTTP body, every ZIP, every
serialized document. The upstream project has the same bug; nothing in the earlier work
had exercised an `XSTRING` big enough to wrap.

### Fix

`unwrapBase64` (in `internal/xrfc/classic_xrfc.go`, used by both decoders) strips CR and
LF — and only CR and LF — before the canonicality checks. Everything else runs unchanged
on the joined text, so embedded spaces, a non-standard alphabet and non-zero padding bits
stay rejected. Regression tests `TestDecodeBase64LineWrapped` and
`TestRCDecodeLineWrappedBase64` pin both the acceptance and the continued rejection.

The flat fast path is untouched. Live regressions stay green: `RFC_READ_TABLE`,
`STFC_STRUCTURE`, `STFC_DEEP_STRUCTURE`, `STFC_DEEP_TABLE`, `Z_CALL_RFC`, and
`TPDAPI_TEST_DEBUGGER describe`.

## 4. The surface

```
vsp rfc adt <METHOD> <URI> [-H NAME=VALUE …] [--body file] [-o file]
```

Status line and headers go to stderr, the body to stdout, so a body pipes or redirects
as it stands; `-o` writes it to a file instead. A 4xx/5xx exits non-zero.
`pkg/saprfc/adt.go` holds `CallADT`, which is transport only.

## 5. What remains

- **Statefulness and CSRF.** `CallADT` fetches no `X-CSRF-Token` and keeps no session,
  so it is a read-only door today. A write flow (lock → PUT source → unlock → activate)
  needs the token round trip and a stateful ADT session; whether cookies/session state
  survive across separate `SADT_REST_RFC_ENDPOINT` calls on one pinned RFC session is
  unverified — `rfc.Client.Pin` is the obvious place to test it. **Nothing has been
  written through the tunnel.**
- **`SADT_PROTECTED_DISCOVERY`** is the second endpoint of this shape (also `FMODE='X'`)
  and has not been exercised.
- **Authorization.** Whether the ADT handlers see the same authorization context as over
  HTTP (`S_ADT_RES` and friends) was not probed beyond the two GETs above.
- **basXML.** We reach an `FMODE='X'` module over the classic serializer using the xRFC
  XML fallback. Real basXML negotiation is still unimplemented and still unnecessary.
- **Publishing.** vsp pins `open-rfc-go` by pseudo-version; `vsp rfc adt` needs the
  base64 fix, so the `go.mod` bump has to follow the open-rfc-go release. Locally, a
  `go.work` with `use ../open-rfc-go` is enough.
