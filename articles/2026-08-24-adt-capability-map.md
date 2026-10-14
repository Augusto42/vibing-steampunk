# The ADT Capability Map

**What a SAP system can be driven to do over plain HTTPS, classified by what each capability requires — and the evidence for each class**

---

## 1. The principle

ADT — the REST interface SAP ships for Eclipse — is not an editor protocol. It is a complete programmable interface to an ABAP system. Nearly everything that is believed to require SAP GUI, an Eclipse installation, or ABAP code deployed on the server is reachable over plain HTTPS with a session cookie.

This is the single claim the project rests on, and everything below is derived from it. Two consequences follow immediately, and they are the reason the claim is worth stating rather than merely using:

**First: the requirement, not the feature, is the correct axis of classification.** Two capabilities that look adjacent in a menu — read a class, debug a class — sit in different classes because one needs a stateless request and the other needs a held session. A tool organised by feature will route both the same way and one of them will fail on half the systems it meets. A tool organised by requirement routes each correctly and knows in advance which systems it can serve.

**Second: the boundary of the claim is small and can be stated exactly.** Only two capabilities in the whole surface still require code installed on the server. Naming them precisely is more useful than any amount of general enthusiasm, because it tells an architect what a deployment costs before they start.

---

## 2. The classification

Four classes, ordered by what each requires of the target system.

| Class | Requires | Contains |
|---|---|---|
| **A** | Plain ADT, stateless | Read, edit, search, free SQL, transports, module lists, activation, ATC, unit tests, dependency and transport analysis, post-mortem |
| **B** | Plain ADT, one held session | The ABAP debugger; the AMDP debugger |
| **C** | Classic RFC — no ADT, no SDK, no gateway library | Any function module, in and out; and the same protocol served |
| **D** | ABAP deployed on the server | Stateful RFC; full-coverage git import and export |

The classes are not a taxonomy imposed on the code; they are the routing table. `vsp compat` asks a system which of them it can serve and prints the answer as a table two systems can be diffed on.

---

## 3. Class A — plain ADT, stateless

### 3.1 What it contains

The whole read and write surface, and — the part that is generally not expected — the whole *analysis* surface. Dependency graphs, dead code, package boundary direction, Clean Core API inventory, transport history as change data, runtime error post-mortem, application log correlation. None of it needs anything installed.

### 3.2 The analysis surface, by question answered

Ordered by the question a person actually has:

| Question | Command | Source of truth |
|---|---|---|
| What changed here, and when? | `vsp changelog` | E071 → E070 → E07T |
| Which transports were one logical change? | `vsp changes --attribute` | E070A transport attributes |
| What in this package is dead? | `vsp slim --level methods` | WBCROSSGT, CROSS, TDEVC |
| Which crossings are architectural errors? | `vsp boundaries` | TDEVC hierarchy + direction |
| Which standard APIs do we depend on? | `vsp api-surface` | TADIR + release state |
| In what shape is this package? | `vsp health` | unit tests, ATC, boundaries, E070 |
| What keeps failing at runtime? | `vsp dumps --group` | `/sap/bc/adt/runtime/dumps` |
| What did the failing program write to the log? | `vsp applog` | BALHDR via free SQL |

### 3.3 Two design decisions worth stating

**Boundary crossings are directional, not binary.** A dependency between two packages is not one fact but eight. A child calling its parent and a parent reaching into a child's internals are opposite architectural errors; a report that says "cross-package dependencies: 47" describes neither.

| Direction | Verdict | Reason |
|---|---|---|
| UPWARD — child → parent | OK | dependency flows toward the root |
| UPWARD_SKIP | WARN | a level is skipped; an abstraction may be missing |
| COMMON — → the shared `_00` package | OK | that is what a common package is for |
| SIBLING | **BAD** | the most actionable finding: it names what to extract |
| DOWNWARD — parent → child | **BAD** | the parent cannot be understood without reading the child |
| EXTERNAL | INFO | outside the caller's control, but tracked |

SIBLING is the finding a team can act on the same afternoon, because the fix is named by the finding itself.

**The application log is correlated by structure, not by time.** A dump and a log entry that are seconds apart are usually unrelated; a log entry written by a program that appears in the dump's call stack usually is. `vsp applog` ranks by the call stack and the call graph, and uses the clock only to break ties.

### 3.4 The evidence

- **The post-mortem resources are verified on two releases**, 7.58 and 7.50, including the case where 7.50 has the dump feed and not the dump detail resource — which the tool reports as an absence rather than as an empty result.
- **`BAL_DB_SEARCH` cannot be called remotely at all.** This is worth recording because it looks like a transport problem and is not: the modules are simply not remote-enabled, so neither an open gateway nor a tunnel helps. The log is read instead as what it is — an ordinary table, through free SQL. Verified on 7.58 and 7.50.
- **Long names are decoded.** `WBCROSSGT.NAME` is `CHAR(120)`; a reference too long for it is stored as a SHA-1, with the real name in `WBCROSSGTX`. The graph resolves them in batches, and a hash it cannot resolve is dropped and reported as a gap rather than passed on as a name.
- **1,066 test functions across 17 packages, all green.**

---

## 4. Class B — plain ADT, one held session

### 4.1 The principle within the class

The debugger resources are ordinary ADT resources. What they require is not a special release, a special authorisation, or installed code: they require **the same ICF session across calls**. A stateless client gets 403 from them and the natural reading — "newer SAP has locked this down" — is wrong. It is a client property, not a release property.

Once that is understood, the class opens completely.

### 4.2 The ABAP debugger

Working over plain HTTPS against a stock system with nothing installed:

- breakpoints, including inside a function module — which needs the module's include, not the group
- stepping, and movement between stack frames
- variables read **and written**
- statement-level traces with values
- batch capture

The release differences are handled rather than assumed: on releases with no stack resource the dispatcher answers instead; `Accept` defaults to `*/*` because a concrete vendor type is refused by some releases and required by others; detach succeeds *by* the conversation closing.

### 4.3 The AMDP debugger

Stepping through the SQLScript that ABAP generates for a HANA-side method. Native ADT, nothing installed:

```
POST /sap/bc/adt/amdp/debugger/main            → start; main id in the Location header
     /main/{mainId}/breakpoints                → set
     /main/{mainId}/debuggees/{id}?step=over   → step
     /main/{mainId}/debuggees/{id}/variables/… → read
```

The whole resource set is in the system's own discovery document as template links. It never had to be guessed.

**What works:** breakpoints fire, stepping, statement-level traces, variable values, the full scope at a stop, and the call stack carrying both the ABAP line and the native one.

**What does not, stated precisely:** table *contents*. The address is right; HANA's own `INIT` refuses. This is a boundary, not a defect, and it is one call wide.

### 4.4 The evidence

This class carries the strongest proof in the project, because it is the only part that can be replayed without a system at all.

`vsp adt debug --record` writes a cassette from a live session — every request and response, with cookies, session ids, server names and instance names redacted at record time. The tests replay it. **`go test ./...` drives the real debugger with the wire substituted and nothing else.** A capability that can be re-run on a laptop, in CI, with no credentials, is proven in a way a screenshot is not.

Four defects surfaced on the first replay, and a fifth from replaying across releases.

---

## 5. Class C — classic RFC, in pure Go

### 5.1 What it contains

`vsp rfc` speaks SAP's classic Type-3 RFC protocol with **no NetWeaver RFC SDK, no native library, no cgo**. Every scalar type including STRING/XSTRING, packed decimals, DATE/TIME and UTCLONG; flat and deep structures and tables; classic and fast serialization.

And it speaks the protocol **as the server**, not only as the client.

### 5.2 The evidence

A live SAP system ran an ABAP program of six parametrized calls against a Go endpoint:

| `RFC_PING` | `RFC_SYSTEM_INFO` | `STFC_CONNECTION` | `STFC_STRUCTURE` | `RFC_READ_TABLE` | `STFC_STRING` |
|---|---|---|---|---|---|
| rc=0 | rc=0 | rc=0 · echo + callback | rc=0 · struct + table | rc=0 · 17×2 | rc=0 |

In SM59, all three test buttons are green against it — Connection Test, Unicode Test, Fast Serialization Test — across three serialization modes.

### 5.3 The stated limit

Research preview. Classic RFC carries no transport encryption, and the API is not stable. The capability is real; the maturity is named.

---

## 6. Class D — what still needs ABAP on the server

Two things. The list is short and it is the whole list.

**Stateful RFC.** SOAP-RFC cannot hold a session, so a session-bound RFC sequence needs a receiver.

**Full-coverage git import and export.** And the dependency here is not our package: the native ADT path handles six object types — PROG, CLAS, INTF, DDLS, BDEF, SRVD — and the installed service delegates everything else to abapGit by calling `zcl_abapgit_objects=>supported_list( )`. So the real dependency is **abapGit**, and our class is a bridge to it.

That distinction sets the design constraint for a lean installer, and it is a hard one: the six native types include **PROG and CLAS**, and a program deploys to a bare system over plain ADT — verified twice, on two releases. Therefore a lean receiver must itself be a report or a class. Not a function group, not a package of objects: those cannot be installed by the six, and a receiver that needs a receiver is the circle it exists to break.

---

## 7. The verification layer

A capability map is only worth the mechanism that keeps it true. Four mechanisms, each closing a different failure.

| Mechanism | Closes | Runs |
|---|---|---|
| `vsp compat` | release differences discovered at runtime by surprise | against a system, diffable between two |
| Debugger cassettes | untestable behaviour behind a live dependency | `go test ./...`, no system |
| Pinned counts | published numbers drifting from the code | every build |
| `vsp sweep` | a capability that is advertised and answers nothing | offline pass in CI, live pass on a system |

### 7.1 The distinction the sweep is built on

There are two kinds of empty answer and they are not distinguishable from the outside: the one that is true, and the one that is a failure wearing a truthful face. So a probe may carry an **oracle** — a second, independent route to the same fact. When the where-used list says an object has twelve callers and the capability returns none, the report says `dead`, not `no results`.

Verdicts are deliberately more numerous than pass/fail, because "nothing came back" is five different situations:

| Verdict | Meaning |
|---|---|
| `answered` | a real answer |
| `dead` | nothing, and an oracle says there was something — the finding |
| `empty` | nothing, and there was nothing |
| `refused` | the system said no, out loud — the capability works |
| `absent` | not on this release — a fact about the system |
| `broken` / `unreachable` | ours |
| `misprobed` / `skipped` | the sweep's own fault, reported and never counted against the product |

Two rules it obeys rather than intends. **It never writes** — enforced by a checked list and a test over the probe table, because a verification tool that mutates a customer's system to verify itself is not one. And **it states its own coverage**: on a4h it reports 23 answered out of 28 probed of 38 advertised, with the ten unprobed named. A clean result over part of a surface, printed without its denominator, is not a clean result.

### 7.2 What the layer currently reports

Against a4h: three findings. Two actions named in the universal tool's own description are routed nowhere; one ST05 resource answers 406. All three are on the board, and all three were found by the build rather than by a person.

---

## 8. The state of the surface

| | Value |
|---|---|
| Releases | v2.45.0, 61 published |
| Commits | 766 |
| Test functions | 1,066 across 17 packages, all green |
| MCP tools | 1 hyperfocused / 101 focused / 146 expert, pinned by a test |
| Contributors | 19 |
| Stars · forks | 444 · 106 |

---

## 9. The boundary, ordered

What is not done, in the same classification, so the map has an edge rather than a fade:

**Class B.** AMDP table contents — one call wide; the address is correct and HANA's `INIT` refuses.

**Class A.** `D010INC`, the compile-time *load* graph, as distinct from the runtime *call* graph: a program can load a class pool for a type definition and never call a method on it, and only that table knows the difference. Designed, not built. Side-effect and LUW classification exists as a tested Go library with no command reaching it. The SQLite cache is configured and not wired, so `vsp boundaries` still takes a minute where it should take two seconds.

**Class C.** No stable API, and no transport encryption in the protocol itself.

**Class D.** The lean receiver is designed to the constraint in §6 and unbuilt; the first step is fifteen minutes of verification, not a sprint.

**Reach.** The universal tool is registered only in hyperfocused mode, so agents in the other two modes cannot address the analysis surface at all. A day of work.

---

## 10. Summary

The claim is that ADT is a complete programmable interface, and the map above is the claim discharged: three of four capability classes need nothing installed on the target system, the fourth contains exactly two items, and each class carries evidence of a different kind — replayable cassettes for the debuggers, a live protocol conformance run for RFC, cross-release verification for the analysis surface, and an automated sweep that calls the whole advertised surface and reports what did not answer.

The map is the deliverable. The mechanisms in §7 are what make it something other than a brochure.

---

**GitHub**: [oisee/vibing-steampunk](https://github.com/oisee/vibing-steampunk) · **v2.45.0**

#ABAP #SAP #ADT #MCP #GoLang #OpenSource #S4HANA #Debugging #AMDP #RFC #CleanCore #VSP
