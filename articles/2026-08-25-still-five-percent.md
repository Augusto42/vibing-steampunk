# VSP IS STILL ONLY 5% EXPLORED

**Or: 12 releases, 3 SAP systems, 42 probes, 0 defects standing — and the April article that never shipped**

---

There is a file in this repository called `articles/2026-04-07-vsp-only-5-percent-explored.md`.

It is 307 lines. It has a scoreboard, a contributor table, an honest-assessment section and a closing line in bold caps. It was finished on April 7 and never published, because the project went quiet eight days later and did not commit code again until August 20.

I am not publishing it now either. I am writing its sequel, because the interesting thing is not that it sat in a folder — it is that almost every number in it was true and one of them was a fabrication nobody had checked. Finding out which took four months and a tool that could ask.

---

## The Scoreboard

| Metric | April 7 | Today (Aug 25) | Delta |
|---|:---:|:---:|:---:|
| **GitHub Stars** | 257 | **446** | +189 |
| **Forks** | 58 | **108** | +50 |
| **Commits** | 455 | **857** | +402 |
| **Releases** | v2.38.1 (55) | **v2.51.0** (67) | +12 |
| **Tests** | 821 | **1,213** across 17 packages | +392 |
| **Contributors** | 15 | **19** | +4 |
| **MCP Tools** | "147" | **1 / 102 / 147**, pinned by a test | measured |
| **Code** | — | **+70,419 / −3,512** across 385 files | since April |

The last row of the table is the one worth reading twice. April published *147 tools* as a single number. It is three numbers, they were never the same, and nothing in the codebase asserted any of them. That is the theme of this whole period: not building more, but being able to say what is there.

---

## The Big Shape Change

Three fronts, and the third is the one I would keep if I could only keep one.

### 1. The debuggers stopped being impossible

For eight months this project's own documentation said REST breakpoints return 403 on newer SAP, so use the WebSocket path, which needs ABAP installed on the server.

It was wrong. The 403 was **our own stateless HTTP client**. ADT's debugger wants a held session and we were sending it requests that had none. A client bug that lived for eight months as a version-compatibility myth.

With that fixed, over plain HTTPS against a stock system with **nothing installed**: breakpoints, stepping, variables read *and written*, movement between stack frames, statement-level traces with values, batch capture.

And then the one nobody expected. **AMDP debugging** — stepping through the SQLScript that ABAP generates for a HANA-side method — had been attempted here since December through an installed class over a WebSocket, and abandoned with the conclusion that breakpoints are accepted and never fire.

ADT exposes the whole thing natively. It is in the system's own discovery document as template links; it never had to be guessed:

```
POST /sap/bc/adt/amdp/debugger/main            → start
     /main/{mainId}/breakpoints                → set
     /main/{mainId}/debuggees/{id}?step=over   → step
     /main/{mainId}/debuggees/{id}/variables/… → read
```

A breakpoint fires, steps, and reports its variables and its call stack with both the ABAP and the native line. Months of "impossible" were entirely self-inflicted.

**And the debugger became testable without SAP at all.** `vsp adt debug --record` writes a cassette from a live session — every request and response, with cookies, session ids, server names and instance names redacted at record time — and the tests replay it. `go test ./...` drives the real debugger with the wire substituted and nothing else. Four defects surfaced the first time a recording ran.

### 2. Context became two-directional

The thing an AI agent actually consumes is not a tool list. It is the code plus enough of what surrounds it to reason about safely. That surround got substantially better, in two directions that need different machinery.

**Downward** — what this code calls — got narrower and truer:

- The contract of a dependency is trimmed to **the methods this code actually calls**. Measured: one class's context carried an interface with 56 methods of which it calls nine. The header keeps the count, so nothing is silently dropped: `IF_ATO_DB_ACCESS (interface, 56 methods; 10 called here)`. Whole contexts shrank 27%.
- The dependency reader stopped marking a class's own collaborators as false positives. It had been treating "the regex found it and the parser did not" as evidence of a string literal — and the two layers read *different things*, declarations versus calls. In the class whose whole job is to dispatch to them, every service it dispatched to was reported as likely-false at confidence 0.3. On this repo's own ABAP: 51 true and 20 false became **71 and 0**.

**Upward** — who calls this — did not exist, because a source cannot supply it:

```
* === Called from 73 place(s) in 45 package(s) ===
*   ZCL_ABAPGIT_EXCEPTION_VIEWER → SHOW_CALLSTACK  [$ZGIT_DEV_UI_LIB]
*   /BOBF/CL_TOOL_CC_UI → DISPLAY_CALLSTACK  [/BOBF/TOOLS]
*   … and 67 more, not listed
```

Nothing in a class's source says that seventy-three others depend on it. A method called from one place can be changed; the same method called from **3,760 places across 694 packages** cannot, and no amount of reading reveals which one it is.

### 3. The tool learned to check itself

This is the front that did not exist in April, and it is why the other two can be believed.

In one week, **ten capabilities were found that had been advertised, registered, reachable by a user, and had never once returned a correct answer.** Not bugs — a defect class:

- `analyze type=callers`, `callees`, `call_graph`, `object_structure` — four features built on an ADT namespace that exists on no release we can test.
- `usage_examples` — asked a cross-reference table for a two-letter code in a one-character column. SAP returns 400; the caller read 400 as "nothing found". That path had never returned a row.
- `EXEC_RESULT` — matched with a prefix against a wrapped message. **"No output captured" described every run there had ever been.**
- Both ST05 calls — refused every content type but `*/*`, so they had failed on every invocation ever made. Fixing that made the requests succeed and both parsers wrong in the same minute, because no response had ever reached them.

**Not one was visible by reading the code.** Every one needed a live system.

So the looking got automated. `vsp sweep` walks the advertised surface and reports what did not answer. Its design turns on one distinction:

> There are two kinds of empty answer — the one that is true, and the one that is a failure wearing a truthful face.

Probes that cannot tell them apart carry an **oracle**: an independent second route to the same fact. When the cross-reference tables say twelve and the capability says none, the report says `dead`, not `no results`. It never writes, and it states its own coverage — because a clean result over part of a surface, printed without its denominator, is the health report saying GOOD over a scan that never ran.

---

## What Checking Found When Pointed At Three Systems

Then the surface was frozen — no new capabilities, fixes only — and swept on **7.58, 7.57 and 7.50**, on one build, with the build and the release stamped into every report.

| | release | probes | broken |
|---|---|---:|---:|
| A | 758 | 42 | **0** |
| B | 757 | 42 | **0** |
| C | 750 | 42 | **0** |

Eight differences across the three, none of them ours: two attributable to a release, four to what the systems contain, two to a target being heavy.

Getting there took four re-runs, and **only the first was avoidable by being careful** — a binary built without its version stamp. The other three were defects a single-system run cannot show:

- A probe asserting its target's type was **right by luck** on one release: the first cross-reference row happened to be a class.
- A resolver filtering after its query starved only where local packages filled the row limit.
- A report naming no release did no harm while there was nothing to compare against.

Which is the sharpest thing this period taught, and it is not "test on more systems because customers differ":

> **A second system is needed not for coverage but for disagreement.** One system cannot refute itself.

---

## Also, Quietly

- **Classic RFC in pure Go**, client *and server*, no NetWeaver SDK, no cgo. A live system ran six parametrized calls against a Go endpoint and every one returned `rc=0`. In SM59 all three test buttons are green.
- **`vsp boundaries` is 11× faster** — 18.8s to 1.6s on a 222-object package — and `vsp health` went from covering 50 objects with a caveat to covering all 221 without one. The speed was the means; the complete signal was the point.
- **`vsp loads`** — the compile-time load graph from `D010INC`. Nothing *references* an include; it is *included*. So an include nobody loads is dead in a way no where-used shows, and neither `CROSS` nor `WBCROSSGT` knows it.
- **Post-mortem**: runtime errors grouped by what keeps failing, and the application log correlated with a dump **by the call stack rather than by the clock**.
- **Browser SSO that repairs its own session**, which matters because an expired SSO session **does not return 401** — ICF forwards to the identity provider and a logon page arrives under a 200.

---

## The Honest Assessment, Updated

**What works, and is now checked on three releases**
- ADT debugging with nothing installed, testable with no system at all
- AMDP debugging: breakpoints, stepping, variables, call stack
- Classic RFC both directions, no SDK
- Two-directional context: what this calls, trimmed to what it uses, and who calls it
- Transport history as change data; directional boundary crossings; post-mortem

**What is still hard, or simply not wired**
- AMDP table contents. The address is right; HANA's own `INIT` refuses.
- `pkg/cache`: the invalidation signal is now established — the source ETag equals the maximum over the object's `REPOSRC` includes excluding the regenerated `CS`, exactly, verified on a class created and changed for the purpose — and the cache is still not built, because `boundaries` is 1.6s without one.
- The sweep covers 39 of 51 advertised capabilities. **The number is correct and incomplete, and it says so.**
- 19 open PRs.

**What surprised me**
- That the hardest thing on the roadmap was a documented REST API the whole time.
- That the worst defect in the codebase was not a crash but a tool that answered confidently and was making the answer up: `vsp graph callees` returned **SHA-1 hashes as the names of referenced objects**, because a name too long for `CHAR(120)` is stored hashed. Silence withholds an answer and can be noticed. Invention supplies one and cannot.

---

## Why Still 5%

Because the denominator moved.

In April, "5% explored" meant *there are 147 tools and most people use eight*. That was true and it was a claim about **breadth**.

Today the tool can be asked what it does and will answer with evidence — which reveals that the unexplored part was never mostly unbuilt. It was **unchecked**. Ten capabilities were mapped, labelled, published and empty. Six more defects lived inside the instrument built to find them, and five of those were invisible from a single system.

So: the map is bigger than it looks, and now a piece of it is *verified* — three releases, one build, forty-two probes, nothing broken. That verified piece is small. The unverified remainder is not unexplored territory; it is territory nobody has yet pointed an instrument at.

Which is a much better problem than the one April had, because it has a method attached.

---

**GitHub**: [oisee/vibing-steampunk](https://github.com/oisee/vibing-steampunk) · **v2.51.0** · 446 stars · 67 releases

*Previously: "Agentic ABAP: Why I Built a Bridge for Claude Code" (Dec 2025) · "Agentic ABAP at 100 Stars" (Feb 2026) · "VSP Is Only 5% Explored" (Apr 2026 — written, never published)*

#ABAP #SAP #MCP #ClaudeCode #GoLang #OpenSource #AI #S4HANA #Debugging #AMDP #RFC #VSP
