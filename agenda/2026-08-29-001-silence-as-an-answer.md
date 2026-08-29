# Silence, returned as an answer

**Author:** vsp-bug-fix
**Date:** 2026-08-29
**Status:** planned, nothing started
**Scope:** tier 1 — four defects and one stalled deploy. Target release v2.55.0.

## The thing they have in common

A session working in another repo spent a day on the question "why does the
test runner not see the test class in this function group". It filed two
reports. Reading them against our code turned up four defects, and three of the
four are the same defect wearing different clothes:

> The tool could not answer, so it answered anyway.

- A file type it could not determine, determined forever, until the stack ran out.
- An object type it does not know, silently asked about as a class of the same name.
- A response it could not parse, reported as "no tests found".

The fourth — `action="test"` ignoring its `target` — is a plain routing bug, but
it belongs in the same release because it is what the reporting session hit
first and it is what sent them looking in the wrong place.

The report puts the cost better than a bug tracker would: *a test that never ran
reports exactly what a passing test reports*. Every item below is an instance of
that sentence.

None of this needs a SAP system. All four are checked by unit tests, which
matters because the systems in the reports are not ones this session may touch.

---

## 1. `ParseABAPFile` ↔ `parseFromContent` recurse until the stack ends

**Where:** `pkg/adt/fileparser.go` — dispatcher at `:131`, detector at `:269`.

**What happens.** The extension switch falls through to `case ext == ".abap"`
and calls `parseFromContent`. The detector scans for `CLASS … DEFINITION`,
`REPORT`, `PROGRAM`, `INTERFACE`, `FUNCTION-POOL` or `FUNCTION`, and on a hit
calls `ParseABAPFile` back with the same path. Neither the path nor any state
changes, so the recursion is unconditional. Reproduced in isolation:
`fatal error: stack overflow`, exit 2, the two frames alternating for the whole
dump.

The comment on the recursive call reads `// Re-parse with known type`. The
knowledge it describes is never carried anywhere.

**Blast radius.** Every bare `.abap` file whose first meaningful line is one of
those six statements — which is precisely the set the content branch exists to
serve. `vsp deploy` is gone for all of them. A file starting with a comment is
fine, because it reaches the end of the detector without matching, which is why
this survived to a release.

**Fix.** Split the dispatcher so the detector can hand its finding forward
rather than re-entering:

```go
func ParseABAPFile(path string) (*ABAPFileInfo, error) {
        return parseABAPFile(path, "")           // "" = detect from extension
}
func parseABAPFile(path string, hint CreatableObjectType) (*ABAPFileInfo, error)
```

The `.abap` arm calls the detector, the detector calls `parseABAPFile(path, t)`
with the type it found, and the hinted call skips the extension switch. Any
shape works as long as the argument changes between the two calls; today
nothing does.

**Test that proves it.** `fileparser_test.go` has twelve tests and **not one of
them uses a bare `.abap` file** — the branch has never had coverage, which is
the whole story of how this shipped. Add one case per detected statement, each
asserting the resulting `ObjectType`, plus the comment-only file as the control
that must still return a clean error.

**Done when:** the six statements each parse to the right type, the control case
still errors cleanly, and `go test ./pkg/adt/` passes with no new skips.

---

## 2. `graph --direction callers` asks about a class when it does not recognise the type

**Where:** `cmd/vsp/cli_extra.go` — the `objURI` switch in `runGraph`, `:829`.

**What happens.** `CLAS`, `PROG`, `INTF`, `FUGR` are mapped. Everything else
hits `default:` and is built as `/sap/bc/adt/oo/classes/{name}`. So
`vsp graph TABL ZDEMO_POSTINGS --direction callers` asks the where-used service
about a *class* named `ZDEMO_POSTINGS`, gets 200 and an empty list, and prints
"nobody calls this — or the name does not exist". Both halves of that sentence
are true about the class it asked about. Neither is about the table.

This is the same failure the `FUNC` case was fixed for, and the comment above
that fix already says why it is bad: *a 404 that then looks like a call graph
the system declined to give*. The `default` arm turns the same mistake into a
200 and an empty list, which is worse, because there is nothing to notice.

**Blast radius.** Every type not in the switch: `TABL`, `INCL`, `DDLS`, `TRAN`
before its own resolution, `STRUCT`, `DTEL`, `MSAG`. A false negative delivered
in the tool's ordinary voice.

**Fix.** Delete the `default` fallback. Map the types we can address —
`/sap/bc/adt/ddic/tables/{name}` is already formed in `pkg/adt/crud.go:952` and
`/sap/bc/adt/programs/includes/{name}` in `pkg/adt/revisions.go:149`, so most of
this is moving strings that exist. For anything still unmapped, refuse by name:
name the type, say it is not addressable here, and list what is. An error is a
worse user experience than an answer and a better one than a wrong answer.

**Test that proves it.** A table test over the type→URI mapping, plus one case
asserting that an unmapped type returns an error rather than a class URI.

**Done when:** no input to `runGraph` can produce a URI for a different object
than the one named.

---

## 3. `action="test"` is the only action that ignores its target

**Where:** `internal/mcp/handlers_devtools.go:16–31`; message built in
`internal/mcp/handlers_help.go:530`.

**What happens.** The router reads `params.object_url` and nothing else. Given
none, it returns `nil, false, nil`, the chain runs out, and the caller gets:

```
No handler found for action="test" target="CLAS ZCL_DEMO_THING".

Valid actions: read, edit, create, delete, search, query, grep, test, analyze, …
```

`test` is in the list in the same breath as the claim that nothing handles it.
Verified by calling `getUnhandledErrorMessage` directly — the target-bearing
form fails identically to the bare one.

The reporting session read this and concluded the action was unimplemented.
Their proposed remedies were "wire it up" and "drop it from the list". Both are
wrong: it is wired up, and it is the documented way to run tests.

**Fix.** Two parts, and both are needed:

1. Build the URL from `target` when `object_url` is absent, using the same
   mapping the CLI has in `buildObjectURL` (`cmd/vsp/devops.go:3721`). That
   mapping is about to be reworked for item 2 — share one table rather than
   growing a third copy.
2. Put `test` in `actionsNeedingTarget` (`handlers_help.go:520`) so a genuinely
   under-specified call says what is missing instead of denying the action
   exists.

**Test that proves it.** Assert the message for `test` with no target names the
missing input; assert the router claims `test` when given only a target. The
existing `handlers_help_examples_test.go` executes every documented example —
extend it so the target-only form is documented and therefore covered.

**Done when:** `SAP(action="test", target="CLAS ZCL_DEMO_THING")` reaches the
handler, and no reachable input produces "No handler found" for a live action.

---

## 4. `parseUnitTestResult` reports every unparsed document as "no tests"

**Where:** `pkg/adt/devtools.go:693`, printed at `cmd/vsp/devops.go` in
`runTest`.

**What happens.** The response is unmarshalled into a struct keyed on
`<program>` elements. Go's XML decoder ignores what it does not recognise, so
any document without them — an error body under 200, a rejected URI, an
exception node — produces zero classes and no error. `runTest` then prints
`No test classes found.` So three different facts arrive as one sentence:

- the object has no tests,
- the URI was not one the service accepts,
- the service said something we did not parse.

This is the item that matters most and is easiest to skip, because unlike the
other three it produces no crash and no obviously wrong output. It is also the
one the whole reporting session was actually about.

**Fix.** Distinguish the cases before formatting. If the body is non-empty and
unmarshals to zero programs, keep it and say so — that the service answered,
that nothing in the answer was a test program, and (behind `--verbose` or the
existing `VSP_DEBUG_XML` path) what it did say. Check for an ADT exception
element explicitly and surface its message. Reserve "no test classes found" for
a well-formed run result that genuinely contains none.

**Also, one line while in there.** The run payload hardcodes
`<testDeterminationStrategy sameProgram="true" assignedTests="false"/>`
(`devtools.go:~660`). It is a plausible reason a function group's include is not
searched. Making it a flag costs nothing and turns an unanswerable question into
an experiment someone can run in one command. Do not claim it is the cause —
nobody has measured that.

**Test that proves it.** Feed `parseUnitTestResult` three bodies: a real run
result, an ADT exception, and a well-formed document with no programs. Assert
three distinguishable outcomes.

**Done when:** a caller can tell "ran, found nothing" from "did not run".

---

## 5. Redeploy `C:\bin` to v2.54.0

Not code. `make deploy-windows` has been blocked since the release by running
`vsp.exe` processes holding the file, so the released version is on GitHub and
not on the machine that uses it. Do this first if the processes are closed, or
last; it does not interact with anything above.

---

## Order, and why

**1 → 2 → 3 → 4.** One and two are independent and are the cheapest; doing them
first means the release has value even if it is cut short. Three depends on the
type→URI mapping that item two is already reworking, so it wants to come after.
Four is last because it is the largest and the only one where the fix shape is a
judgement call rather than a correction.

Items two and three both touch object-type→URI mapping and there are now three
copies of it (`runGraph`, `buildObjectURL`, `handlers_devtools`). Unifying them
is tempting and is not in this sprint — say so in the commit rather than
quietly leaving the reader to wonder.

## What this sprint does not settle

Whether the ADT test runner can see a `FOR TESTING` class inside a function
group's own include. That needs a system and a SAP GUI trace, and neither is
available from here. Item 4 does not answer the question; it makes the tool stop
pretending it already did.

## Release

v2.55.0 when items 1–4 are in. The honest headline is not "bug fixes" — it is
that the tool stopped presenting silence as an answer.

The sweep gate applies as it did for v2.52.0: the release gate must cover the
changed code. Items 1–4 are all unit-testable, so the coverage here is `go test`
rather than new sweep probes; the `SAP()` surface changes only in item 3, which
the examples test already walks.
