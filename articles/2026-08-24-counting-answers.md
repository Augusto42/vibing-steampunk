# I Counted 147 Features. Then I Checked How Many Answered.

**Or: the article I drafted in April, the four months I didn't ship it, and the five days that told me why not shipping it was lucky**

---

On April 1, 2026, two AI models reviewed this repository on the same day.

The first wrote 104 lines titled *"The Agentic ABAP Revolution."* It ran nothing. Every number in it was copied out of our own documentation: 99.5% token reduction, 7–30x compression, WASM-to-ABAP marked **Proven**, and a closing line calling the project *"the missing link that turns a 30-year-old enterprise platform into a playground."*

The second wrote 651 lines. Somewhere near the top there is a section called **"Current verification status from this checkout"**:

> I ran `go test ./...` in the repository. […] `pkg/dsl` fails to build under test because of several `fmt.Errorf` calls with non-constant format strings. `pkg/jseval` fails an oracle comparison test. So the repo is broadly alive, **but not fully green**.

It counted files instead of adjectives — 184 Go, 200 ABAP, **185 Markdown** — and then wrote the paragraph that turned out to be the whole year:

> One striking characteristic of this repo is that it preserves its thinking process in-tree. **Weaknesses of that approach:** claims can drift across README, architecture docs, roadmap, changelog, and reports; counts and maturity statements do not always stay synchronized […] **which can blur the current truth if read carelessly.** Examples of drift I observed: tool-count numbers vary across docs…

Four days later we ran a claims audit that reached the same conclusion. Four months later, an eleven-agent audit found sixty-eight of them.

There is also a file in this repo called `articles/2026-04-07-vsp-only-5-percent-explored.md`. It is 307 lines, it has a scoreboard and a contributor table and a closing line in bold caps, and it was never published.

This is that article, and what happened instead.

---

## Part One: what spring actually built

### March: the compiler lab

Two weeks in late March produced 171 commits and almost none of the product.

A **WASM → ABAP** ahead-of-time compiler, pointed at the QuickJS JavaScript engine: 1,410 functions, 100% opcode coverage. Then a week of trench warfare with `GENERATE SUBROUTINE POOL`, ending in one of my favourite commits in the repo:

> WASM allows block/end to cross if/else boundaries:
> ```
>   block; if; end_block; else; end_if   (valid WASM)
>   DO. IF. ENDDO. ELSE. ENDIF.          (invalid ABAP)
> ```
> GENERATE rejects this as "No open IF". Need architectural change.

The architectural change was to stop emitting WASM blocks as loops and emit them as **class methods**. After that: `QuickJS GENERATE SUBROUTINE POOL succeeds — rc=0 on SAP!`, then 11 of 11 self-hosting tests on a live system.

Then an **LLVM IR → ABAP** compiler — C through clang into typed ABAP class methods. QuickJS again: **537 functions, 121,000 lines of ABAP, 0 TODOs.** Function pointers became CASE trampolines, memory became a flat internal table with byte addressing, and `CASE` had to be auto-split into chunks of twelve because ABAP has limits.

Then on March 30 the whole thread was abandoned mid-stride for `pkg/jseval`, a JavaScript interpreter in pure Go — which was itself transpiled to ABAP as `ZCL_JSEVAL`, where it evaluates `fib(20)` in 378 ms.

All four still build and pass today. Here's what the April draft didn't say:

| package | source | tests | importers | honest verdict |
|---|---:|---:|---:|---|
| `pkg/wasmcomp` | 4,260 | 10 files | 1 | working prototype, weakly verified |
| `pkg/llvm2abap` | 1,973 | **1 file** | 2 | advanced sketch with a smoke test |
| `pkg/jseval` | 1,518 | 7 files | **0** | genuine, best-tested, unused |
| `pkg/ts2go` | 608 | **0** | **0** | abandoned |

`wasmcomp`'s suite verifies the compiler *produces output*, and separately runs the input `.wasm` under wazero as an oracle. **Nothing in CI checks that the generated ABAP is correct** — the "11/11 on SAP" was a manual run, synced back by hand. `llvm2abap`'s entire suite is three `strings.Contains` assertions on generated text.

And `pkg/jseval` was built so vsp could run the abapLint lexer without Node.js. Nothing in the product calls it. Its final commit, on April 7, was titled *"skip local-only JS and TS transpile fixtures in CI"* — the last thing that ever happened to it was turning its tests off.

They're a lab, and a good one. The April draft filed them under "compiler experiments stopped being toy demos." The accurate version: they stopped being demos and never started being product.

### April: two weeks that made the analysis engine

April was 152 commits, and every one of them landed between the 2nd and the 15th.

**Transport history as change data.** The most useful thing April built wasn't a feature, it was a reframing: *the transport system is a change log that nobody reads as one.*

```bash
vsp changelog '$ZDEMO' --since 20260101      # what changed here, and when
vsp changes   '$ZDEMO' --attribute SAPNOTE   # which transports were one logical change
```

`changelog` joins E071 (object → transport), E070 (headers) and E07T (descriptions). `changes` goes a layer up: it reads E070A transport *attributes* and groups every transport sharing a value. Put your change-request id in a custom attribute in SE01 and you get CR-level correlation across the landscape, from a terminal, no SE03, no SAP GUI. The bug that taught us E07T exists is the kind only a live system gives you: the command worked perfectly on local packages and returned HTTP 400 on the first transportable one.

**Dead code, health, Clean Core.** `vsp slim` traces reverse references through WBCROSSGT and CROSS, resolves hierarchy through TDEVC, and classifies each method LIVE / INTERNAL_ONLY / DEAD. `vsp health` reports tests, ATC findings, boundary violations and staleness, and deliberately doesn't say what to fix — only where to look. `vsp api-surface` inventories every standard SAP API your code touches and checks its release state, because the Clean Core question is never "are we clean" but "how unclean, exactly, and where."

The three MVP specs behind those were the most disciplined documents of the month, because each led with **Non-Goals**. The rule the whole set was written under, endorsed by two reviewing models: *"Do not try to make the MVP too smart."* And the graph knowledge design carried a code comment that ought to be framed:

> `// WARNING: REQUIRES_AUTH could mean SU24 default, runtime AUTHORITY-CHECK, or role assignment. These are different things. Do not implement until the intended semantic is chosen and documented.`

**The idea I'd still point a new reader at first: direction and effect.**

A dependency between two packages isn't one fact, it's eight. `pkg/graph/crossing.go` classifies each crossing through the package hierarchy — because a child calling its parent and a parent reaching into a child's internals are opposite architectural sins, and "cross-package dependencies: 47" tells you about neither:

| direction | verdict | why |
|---|---|---|
| UPWARD (child → parent) | OK | dependency flows toward the root |
| UPWARD_SKIP | WARN | skips a level — missing abstraction? |
| COMMON (→ the shared `_00`) | OK | that's what `_00` is for |
| **SIBLING** | **BAD** | *the most actionable finding — it names exactly what to extract* |
| **DOWNWARD (parent → child)** | **BAD** | *makes the parent impossible to understand without reading the child* |
| EXTERNAL | INFO | outside your control, but track it |

What makes that design document good is that a third of it is headed **"What This Model Cannot Do"** — it doesn't resolve interfaces, so an implementation reached through an interface still reads as a concrete sibling reference; test packages blur the lines by design. And it closes by voting against itself, recommending the simpler binary version ship first.

Then the second question: what does a unit *do to the world*. Here the design found something genuinely SAP-native, which I'd argue is the best single insight in the archive:

> The LUW patterns are the most dangerous for transitive analysis. A method that calls `CALL FUNCTION … IN UPDATE TASK` is not writing to the database *yet* — it's deferring. But whoever calls `COMMIT WORK` higher up will trigger all deferred writes. **This is invisible coupling.**

Out of which falls a classification more useful than pure/impure: **LUW-safe** (no commits, no registrations), **LUW-participant** (registers deferred work, doesn't commit), **LUW-owner** (contains `COMMIT WORK` — owns the transaction boundary), **LUW-unsafe** (mixes both). `COMMIT WORK` inside a utility method is rated *"HIGH — breaks caller's LUW"*, and that is a real bug class that nothing in SAP's toolchain will tell you about.

Hold that one. I'm coming back to it, and not in the way you'd expect.

**Fourteen PRs to zero.** 3 merged clean, 5 cherry-picked with fixes, 4 reimplemented, 2 closed. Contributors 5 → 15, across Germany, South Africa and Brazil. The one to remember is PR #82, refactoring tools: the triage note reads *"Endpoints likely fabricated — ADT discovery marks Refactoring as 'NOT Exposed'."* The endpoints turned out to be **real**; it was the implementation that was invented. Hold that one too.

### April audited itself. Two days before the article.

This is the part I didn't expect to find when I went back through the history.

On **April 5**, the project ran a claims audit on its own status table, asking three questions of every row — *is it real, is the label honest, do we need it in the table at all* — and marked its own homework down:

> - LLVM IR→ABAP: ~~Complete~~ → Advanced prototype
> - WASM Block-as-METHOD: ~~Complete~~ → Proven on large outputs
> - TS→ABAP Pipeline: ~~Proven~~ → Experimental
> - Graph Engine: ~~Slice 1+2 done~~ → In progress

with the instruction *"stop mixing production tools with compiler experiments"*, and a companion report whose closing sentence is the best of the month:

> **The goal is not to make the project look smaller. The goal is to make the docs trustworthy.**

That report also caught the Graph Engine being marked shipped *on the same day it was still being designed*, and caught the README telling Codex users to configure a project-local `.mcp.json` when Codex actually wants TOML — with a rule I've used ever since: **do not guess in docs**; separate `Verified:` from `Unverified in this repo docs:`.

**Two days later I wrote an article claiming 147 tools.**

And here is the sharp bit. The audit *checked* that number. It appears in the audit's own table as:

> `**Tools** (147/100) | ✅ | KEEP | Verifiable: count tools in register`

Verifiable. Nobody counted. August counted: **1 / 101 / 146.** The audit's proposed *replacement* wording for another row — "91 statement types + 8 lint rules" — was itself wrong twice over (95 patterns; 13 rules, 8 on by default).

An audit of claims that contained claims which failed a later audit. That's the whole article in one artifact.

### April also found the defect class, named it, and shipped it anyway

**April 7** — *"slim reverse ref queries: ADT freestyle doesn't support OR with LIKE."* SAP's freestyle SQL parser rejects more than one `LIKE` per `WHERE` and returns 400.

**April 9** — a commit whose entire subject line is the rule:

> **fix: never fail silently — add WARN stderr logging for all resolve/query errors**

**April 13** — the same OR-LIKE optimisation, reintroduced in a different file:

> the OR-LIKE batching […] **silently failed on every query** and dropped the entire code-side scan to zero results. The downstream buckets cascaded: 0 code tables → 0 Covered/Missing, 31 Orphan (= the whole data side), and a DDIC metadata walk starved of scope tables (246 reachable instead of ~2000). **Every number in the audit was wrong.**
>
> […] 6 workers hit WBCROSSGT/CROSS for 227 scope objects in roughly the time **the OR-LIKE attempt pretended to take**.

Learned, reintroduced, re-learned, in six days.

Three more from the same fortnight, all the same shape:

> Install handler checks `err != nil` but not `result.Success` — logs "OK" even when syntax/activation/lock failures return nil error with `Success=false`. **Critical: installer lies about success.**

Nine objects deployed, nine "OK"s printed, an empty package, exit code zero. Underneath it: `WriteSource` returns `(result, nil)` on syntax errors, activation failures, object conflicts and lock failures alike. A Go error and a SAP failure are different objects, and the code only knew about one.

> DeleteObject sent the HTTP DELETE without `Stateful: true`, so on a real SAP system that enforces lock-handle session affinity the cleanup DELETE would be rejected. **The mock tests passed only because the mock does not model session affinity.**

And my favourite piece of causality in the whole archive. On April 4 the quality sprint fixed a cluster of write failures by making sessions **stateless by default**. On April 7 that fix produced this:

> **The paradox:** Go's CookieJar sends `sap-contextid` cookies automatically, but the `stateless` header overrides and forces SAP to treat each request as independent.

Lock acquired in session A; write arrives as session B; lock handle not found. The fix on April 7 was itself incomplete — it missed the class-include paths, where the bug reproduced **100% of the time** — and the real close came on April 15, taking four issue numbers with it. Eleven days, three fixes, one root cause, introduced by a fix.

There was one more, and it's the one that should worry you, because this is a tool whose entire premise is letting an AI write to a live SAP system:

> The safety check was applied at create-time (where the package is an explicit parameter) but **never at edit-time** (where the package must be resolved from the existing object's metadata).

`SAP_ALLOWED_PACKAGES` — the guardrail — was enforced on four functions, none of which an agent uses. `EditSource`, `WriteSource`, `WriteProgram`, `WriteClass`: unrestricted, on any object in any package. The April 1 review had specifically praised this subsystem: *"one of the stronger 'proven' parts of the repo… without a serious safety layer, that thesis would not be credible."*

It was fixed in 55 minutes, at night, from a user report — and then fixed *properly*, because the request that came back was for architecture rather than a patch:

> «давайте сделайте всё через один auth/filter gate — потому что у нас гейты есть по CRUD, по TR (по CR!) по пакетам»
> *("do it all through one auth/filter gate — we have gates for CRUD, for transports, for change requests, for packages")*

One `checkMutation()` entry point, 15 mutators migrated, and the decision that makes it a security fix rather than a patch: when neither the package nor the resolution path is available under an active whitelist, **fail closed.** Applied honestly even where it hurt — UI5 mutations now refuse outright, with an error that says why:

> `operation 'UI5UploadFile' on UI5 surface is blocked: UI5 app→package resolution not yet implemented, cannot verify package against SAP_ALLOWED_PACKAGES`

Remember these five. August found ten more of exactly the same animal.

---

## Part Two: the four months

The last April commit was on the 15th. The next commit of any kind was a documentation triage on **June 15**. The next line of code was on **August 20**.

I could dress that up as a deliberate pause. It wasn't. The project went quiet, the draft stayed in its folder, and the PR queue April had emptied filled back up — **19 open pull requests and 30 open issues** today.

Two casualties worth naming, because they're what an interrupted project actually looks like from the inside.

**The cache.** April designed a SQLite cache in six milestones, with a headline number: `vsp boundaries` from ~60 s to ~2 s. Milestone one — configuration — landed. Milestones two through six, including the one the plan itself annotates *"← main value"*, did not. `pkg/cache` is 1,188 lines with **zero importers**, last touched in December. Four days after the plan was written, one command that needed caching hand-rolled its own. So today there's a config flag you can set, that is read, resolved from environment variables, and printed by `vsp systems` — and caches nothing. `boundaries` still takes 60 seconds.

**D010INC.** The graph engine's design had one genuinely original idea, and it explained itself well: CROSS and WBCROSSGT record what is **called**; `D010INC` records what is **loaded**. A program can load a class pool for a type definition and never call a method on it, and only one of those two tables knows the difference. It exists today as two constants — `EdgeLoads`, `SourceD010INC` — and no builder, no query, no caller.

---

## Part Three: five days

Between August 20 and August 24: **234 commits, six releases** (v2.40.0 → v2.45.0), and — measured tag to tag from where April stopped — **315 files, +58,948 / −3,087 lines.** All of April was 152 commits and +22,175 / −811. The ratio between those two deletion columns is the honest summary: April added, August refactored.

### Classic RFC, in pure Go, both directions

`vsp rfc` calls any SAP function module with **no NetWeaver RFC SDK, no native library, no cgo**, speaking classic Type-3 on top of `open-rfc-go`. Every scalar type including STRING/XSTRING, packed decimals and UTCLONG; flat and deep structures and tables; classic and fast serialization.

And it speaks the protocol **as the server**. A live SAP system ran an ABAP program of six parametrized calls — `RFC_PING`, `RFC_SYSTEM_INFO`, `STFC_CONNECTION`, `STFC_STRUCTURE`, `RFC_READ_TABLE`, `STFC_STRING` — against a Go endpoint, and every one returned `rc=0`. In SM59, all three test buttons are green against it.

It's labelled a research preview and deserves the label: classic RFC has no transport encryption, and the API isn't stable. But "you need SAP's closed-source library to speak RFC" is no longer true.

### The debugger that never needed our ABAP

For eight months this project's own documentation said: *REST breakpoints return 403 on newer SAP; use the WebSocket path; that needs our ABAP package installed on the server.* The April draft repeats it under "What's Still Hard". The April *design document* for the GUI debugger states it as a fact about SAP releases.

It was wrong. The 403 was **our own stateless HTTP client**. ADT's debugger wants a held session; we were sending it requests that had none. A client bug that had been living for eight months as a version-compatibility myth.

Fixed, the debugger runs over plain HTTPS against a stock system with nothing installed: breakpoints, stepping, variables read *and written*, movement between stack frames, statement-level traces with values, batch capture. A dozen smaller truths came with it — a breakpoint inside a function module needs its include; attach must activate external debugging for its own session; a closed conversation *is* how detach succeeds; `Accept` must default to `*/*`; and on releases with no stack resource, the dispatcher answers instead.

### AMDP: from "impossible" to a breakpoint

AMDP debugging — stepping through the SQLScript that ABAP generates for a HANA-side method — had been attempted here since December through an installed class over a WebSocket, and abandoned with the conclusion that breakpoints are accepted and then never fire.

ADT exposes the whole thing natively. `POST /sap/bc/adt/amdp/debugger/main`, then `/main/{mainId}/breakpoints`, `/debuggees/{id}?step=over`, `/variables/{name}`. It's in the system's own discovery document as template links. It never had to be guessed.

Today a breakpoint fires, steps, and reports its variables and its call stack — with both the ABAP and the native line — over plain ADT, nothing installed. Table *contents* are still open: the address is right, and HANA's own `INIT` refuses.

The mistake I made getting there is worth keeping. The start response carries a field called `HANA_SESSION_ID`, so I took it for the id the rest of the API wants. It isn't. The handler sets two different fields from two different sources; the body returns one and the **`Location` header** returns the other. I published the wrong version and corrected it the same day.

### A debugger you can test with no SAP

The debugger had, by measurement, **zero tests that ran by default**. Its only cross-transport guarantee sat behind an integration tag and needed both a live system and an installed ABAP facade, so it never exercised the "no Z code needed" claim at all. There was also a test asserting that the stateless client's debug calls *return errors* — it pinned the broken behaviour, and in a test listing it read like coverage.

`vsp adt debug --record` now writes a cassette from a live session: every request and response, with cookies, session ids, server names and instance names redacted at record time. The tests replay it, so `go test ./...` drives the real debugger with the wire substituted and nothing else.

**Four defects surfaced the first time a recording ran**, and a fifth from replaying it across releases. One of them: every recorded trace had been coming out with no values in it.

### From a dump to the log

The debugger helps when you can reproduce the failure. Usually you can't: there's a dump from Tuesday and a user who has moved on.

`vsp dumps` reads ST22 over plain ADT and groups runtime errors by what keeps failing. `vsp applog` reads the application log — and this is the part I like — **correlated with a dump by the call stack and the call graph rather than by the clock.**

Two wrong turns are in that design note. `/sap/bc/adt/applicationlog/objects` looks like the business log and isn't: it answers 400, and its discovery siblings are the signature of a repository *object type editor* — it edits SLG0 definitions. Then I decided the log would need the `BAL_*` function modules over RFC, and that a system with no gateway would need a tunnel to reach them. Both halves wrong, and the second hid the first: **`BAL_DB_SEARCH` is not remote-enabled at all.** No transport helps. A transport question was the wrong question. The log's header table is an ordinary table, and free SQL reads it — verified on 7.58 and 7.50.

### And the load-bearing boring things

Browser SSO that repairs its own expired session — which matters because an expired SSO session **does not return 401**: ICF forwards to the identity provider and the logon page arrives under a 200, so detection is by origin and by a missing CSRF token, never by status code. (Compare April's BTP bug, which was the same genre: Go's `http.Client` strips the `Authorization` header on every redirect per RFC 7235 §4.2, and BTP authenticates through redirects — which is why `curl` worked and we didn't.) Port detection that sweeps, prefers TLS and follows the name on the certificate. `vsp compat`, which asks a system what it supports and how each capability should route. Landscape discovery that reads what SAP GUI already wrote — after a fix titled *"stop inventing hosts, and address systems the way they answer."*

That title is foreshadowing.

---

## Part Four: the turn

Somewhere in that week I stopped adding and ran an audit.

Eleven agents read the README in full, both published articles, the whole MCP tool surface, every package, the debugger and the standing agenda. They inventoried **181 promises**, verified **134** against the code, and found **68** overstated, wrong, or unverifiable.

The first to fall was a number from the article you have been reading.

**147 tools.** In the April draft's scoreboard. In the README in twelve places. The `--mode` help said 100/147; the long usage said 81/122. Three published numbers, none right. Measured by asking the server what it actually registers: **1 in hyperfocused mode, 101 in focused, 146 in expert.** Pinned now by a test that asserts each count, requires focused to be a subset of expert, and requires every whitelisted name to be a tool that exists.

That last assertion failed on its first run and found ten gCTS tools whitelisted in every mode, behind 884 lines of handler code, with the registration function called from nowhere. Dead in all three modes since the day they were merged — as the closing win of April's PR sprint.

That's where the week changed shape.

---

## Part Five: ten features that were advertised and never worked

Once you start pulling, it doesn't stop. Every one of these was in the product, in the docs, reachable by a user, and had never returned a correct answer:

- **The dump filters.** All six of MCP's filter parameters were passed to a resource that ignores them. Decoration.
- **The dump feed parser.** Read fields that are empty on every row.
- **`EXEC_RESULT`.** Matched with a prefix against a message that arrives wrapped. It never matched — meaning **"no output captured" described every run there had ever been.**
- **`usage_examples`** for function modules, SUBMIT and programs. Asked CROSS for the two-letter code `'FU'` in a `C(1)` column. SAP returns 400; the caller read 400 as "nothing found". That path had never returned a row.
- **`analyze type=callers`, `callees`, `call_graph`, `object_structure`.** Four features built on the `/sap/bc/adt/cai/` namespace, which does not exist on any release we can test.
- **`where_used_config`.** Filtered `TYPE='DA'` — not a value of that column at all; it's an OTYPE from a different table.
- **A correlation rung I'd shipped the day before**, which could not fire, because the resource beneath it exists nowhere we can reach. I found my own.

Plus six places where an **activation refusal arrives inside an HTTP 200** and was read as success — including one where `RenameObject` deleted the original after the activation had been refused. Which is the April installer bug again, four months later, in a different file. *"Checks `err != nil` but not `result.Success`."*

And three that weren't missing caveats but wrong *numbers*: a health report saying GOOD over a sweep that could not run; a transport holding nothing reported `SELF-CONSISTENT`; `trace unit` exiting zero while stating that nobody had run.

**Not one of them was visible by reading the code.** Every single one needed a live system. That's what makes it a defect class rather than a list of bugs: the codebase is full of places where a failed call quietly becomes an empty list, and an empty list is indistinguishable from an honest "nothing here".

The commit subjects from that sweep read like a confession, which is why they're written that way:

> *a search that skipped objects said it had searched them*
> *checks that never ran were reported as checks that found nothing*
> *half of a callee list was reported as all of it*
> *a program that will not compile is not a program that ran*
> *reports that could not look everywhere said so nowhere*

### The one that was worse

Then, at midnight on the last day, this.

`WBCROSSGT.NAME` is `CHAR(120)`. A reference whose full name doesn't fit is **not truncated** — SAP stores a **SHA-1 of it** and keeps the real name in a side table, `WBCROSSGTX`, whose own description reads *"Index for Global Types – Management of Long Names"*.

Read the main table alone and that hash arrives looking exactly like a name. Forty hex characters carry no backslash, so the name splitter leaves them whole. They match no object's own name, so the self-reference filter keeps them. The row is marked DIRECT, so the indirect filter keeps them.

Asking a stock system what `/IWBEP/CL_MGW_CACHED_REQUEST` references returned twelve things, two of which were:

```
7B8B998F59381D55C977F2F55C1C061D25D9885E
A15C4C18AE006254E67B73937D5149766FD922C9
```

presented as data references. As object names. To an AI agent that would then go looking for them.

Everything else that week was **silence** — an answer withheld, which a careful reader can notice. This was **invention** — an answer supplied, which nobody can. A hash that can't be decoded is now dropped and reported as a gap, because leaving it in would put the invented name back. The same query returns ten references now and no hashes; the two decoded to the class's own interface-method parameters and were dropped as the self-references they were.

### The eleventh, found while writing this

Remember the LUW analysis — invisible coupling, `COMMIT WORK` in a utility method breaking every caller's transaction, the four-way classification I called the best insight in the archive?

`ExtractEffects` has **zero callers.** No CLI command reaches it. No MCP action routes to it. It is 250 lines of correct, well-tested Go that nothing in the product can invoke — and it has been described as a shipped capability in the README since April 8, in two places.

The steering plan written **the same day** as the implementation had a risk register. Risk 1 was titled **"Semantic overclaiming."** Its acceptance criterion was a sentence about honesty rather than capability:

> Success signal: transitive output reads like a careful summary, **not a theorem we cannot prove.**

The README now says *library only — no command reaches it yet*, corrected in the same commit that added this article. Wiring it is on the board. It is a good feature. It just wasn't one.

---

## What actually found all this

Four things, and I'd rather hand these over than the fixes.

**1. Ask the system; don't read its catalogue.** Discovery lies in both directions. Resources absent from it answer 200. Resources present in it answer 400. The dump resources that became `vsp dumps` are listed nowhere at all. And April's PR #82 was rejected because *ADT discovery marks Refactoring as "NOT Exposed"* — the endpoints were real.

**2. Read the handler.** Five times out of five, opening the ABAP class that serves a resource answered in one request what inference hadn't answered in several. Sharper: *when SAP does something in the kernel, look at what the same class reads from a table.* That's how `TMDIR` was found, after `GET_METHOD_BY_INCLUDE` turned out to be a `SYSTEM-CALL` with nothing to read. The SHA-1 bug came from the same move — one grep for `WBCROSSGT` lands in the class that **hashes** any name longer than 120 characters before searching. The same fact, from the writing side.

**3. Read what the system already sent.** The AMDP stop event carried the position, then the variables, then the call stack. Three separate discoveries, all inside one document I'd had since the first trace.

**4. Measure; don't reason.** Every rule I inferred from the examples in front of me was wrong about the first case nobody had tried. `'FU'` in a one-character column. A section-prefix list covering `U01` and missing `U27`. A pool-section matcher that accepted `LEGACY_REPORT` and confidently produced a function group called `EGACY_REP`.

---

## The scoreboard, updated

| | Apr 7 | Aug 24 |
|---|:--:|:--:|
| Stars | 257 | **444** |
| Forks | 58 | **106** |
| Commits | 455 | **766** |
| Releases | 55 | **61** |
| Contributors | 15 | **19** |
| Open PRs | **0** | 19 |
| Test functions | 821 | **1,066** (17 packages, all green) |
| MCP tools | "147" | **1 / 101 / 146**, pinned by a test |
| Latest | v2.38.1 | **v2.45.0** |

The rows worth reading aren't the stars. They're the tools row and the open-PR row: one number got smaller because it got true, and one got worse because I stopped paying attention for four months.

---

## The honest assessment, updated

**What works, and is now checked**
- ADT debugging over plain HTTPS with nothing installed — and testable with no system at all
- AMDP debugging: breakpoints, stepping, variables, call stack. Natively.
- Classic RFC in pure Go, client and server, no SDK
- Transport history as change data; directional boundary crossings
- Post-mortem: dumps grouped, application log correlated by call stack
- A mutation gate that fails closed

**What's still hard, or simply not wired**
- AMDP table contents. The address is right; HANA's `INIT` refuses.
- Side effects / LUW: implemented, tested, **unreachable**. Now labelled as such.
- `pkg/cache`: designed in six milestones, one shipped, zero importers. `boundaries` still takes 60 seconds.
- D010INC — the compile-time load graph — is two constants.
- UI5/BSP writes: still read-only, and now refusing loudly rather than proceeding.
- ADT freestyle SQL: no JOINs, no OR+LIKE, no subqueries. Every query is a workaround — twice over, as April proved.
- The universal `SAP()` tool exists **only** in hyperfocused mode, so agents in the other two modes can't reach any of this month's analysis work. Same disease. A day of work, not yet done.
- gCTS: connect-or-delete, undecided. 200 on two systems, 404 on 7.50.
- 19 open PRs.

**What surprised me**
- That the hardest thing on the roadmap — AMDP — was a documented REST API the whole time, and months of failure were entirely self-inflicted.
- That auditing our own claims was more productive than any feature shipped that week.
- That the worst defect in the codebase wasn't a crash. It was a tool answering confidently and making the answer up.

---

## The actual lesson, which isn't the one I expected

I went into the archive expecting the story to be *April overclaimed, August measured.*

It isn't. **April measured too.** April ran a claims audit and downgraded five of its own status rows. April wrote *"the goal is to make the docs trustworthy."* April wrote a commit titled *"never fail silently."* April wrote a risk register naming *"semantic overclaiming"* — and shipped an overclaimed feature the same day. April found a silent failure, fixed it, reintroduced it four days later in a different file, and only then discovered it had zeroed an entire analysis and made every published number in it wrong.

So the lesson isn't "be more careful." April *was* careful, in writing, on a Tuesday.

**A rule stated in a commit message is not enforced by anything.**

The tool count only became true when a test started asserting it — and that test failed on its first run and found ten more dead tools. The SHA-1 hashes only stopped appearing when the decoder began *dropping* what it couldn't resolve. The mutation gate only became a guardrail when it started failing closed. In every case, the thing that fixed it was a mechanism that can fail out loud, not an intention recorded in prose.

And ten dead features were found by calling everything the product advertises against a live system and seeing what answered — by hand, over one week, by a person who happened to be looking. That is not a mechanism. That's why the next thing here isn't a feature: it's a command that walks the entire advertised surface and reports what didn't answer, so the twelfth dead feature is found by the build and not by an article.

---

## Why I'm glad April didn't ship

Because I'd have published "147 tools", "AMDP breakpoints are unreliable" and "REST returns 403 on newer SAP" — three things I believed, had written down, and had never once checked against a running system.

The April draft's headline was **VSP IS ONLY 5% EXPLORED**. Still true. Here's the part I didn't know then:

**Some of that 95% wasn't unexplored. It was mapped, labelled, published — and empty.**

The difference between a tool you can trust and a tool you can't isn't how many features it has. It's whether, when it can't answer, it says so.

---

**GitHub**: [oisee/vibing-steampunk](https://github.com/oisee/vibing-steampunk) · **v2.45.0** · 444 stars

*Previous: "Agentic ABAP: Why I Built a Bridge for Claude Code" (Dec 2025) · "Agentic ABAP at 100 Stars" (Feb 2026) · "VSP Is Only 5% Explored" (Apr 2026 — unpublished, folded into this one)*

#ABAP #SAP #MCP #ClaudeCode #GoLang #OpenSource #AI #S4HANA #Debugging #RFC #StaticAnalysis #VSP
