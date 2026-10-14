# Debugger: replay recorded conversations

Status: first cassette recorded and replaying in CI. Coverage is narrow
and the way to widen it is known.

## Why this was first

The debugger is the feature we most want to claim, and it was the one
with no test that ran by default. The single cross-transport check sat
behind `-tags=integration` and needed a live SAP, an RFC channel *and*
the `ZADT_DEBUG` facade — so it could not exercise the thing worth
proving, that the ADT path needs no Z code at all.

## What was built

`pkg/saprfc/cassette.go`. The ADT transport is a one-method interface,
which makes record and replay a small thing:

- `Recorder(inner)` wraps a live transport and captures every
  request/answer pair.
- `LoadCassette(path)` answers from the file and nothing else. Strict by
  default: an unrecorded request is an error, never an invented answer.
- Keying is method + URI + body together, because the debugger sends one
  URI with different payloads (`getChildVariables` for two parents), and
  repeats advance in order — the second `getStack` of a run is a
  different stack — then hold on the last recorded answer.

`vsp adt debug --record <path>` takes one from a live run.

## Keeping a live recording publishable

A cassette is a tracked fixture taken from a real system, so the
recorder scrubs by default rather than by list:

- Headers that carry a session or name a box are dropped outright —
  `Set-Cookie` gives away both at once, since `sap-contextid` embeds the
  application server's hostname.
- The logon name is substituted on the way to disk. Request and answer
  are rewritten together, so a scrubbed cassette replays exactly like
  the run it came from.
- `TestCommittedCassettesCarryNoLiveIdentifiers` re-checks the committed
  files on every run, so a careless re-record cannot quietly undo this.

`.gitignore` blanket-ignores `*.jsonl`; the exception is narrow, one
directory.

## What the recording found immediately

Both were live bugs, not test artefacts:

1. **Refusals lost their reason.** `adtError` preferred the machine
   category (`notAuthorized`) over the sentence (`User X does not
   exist`), so every distinct refusal reached the caller as the same
   word. It also matched `<message lang="EN">` literally, finding
   nothing on a system logged on in any other language.
2. **`vsp adt debug` had no SSO branch.** It handled cookie files and
   cookie strings and ignored `auth: sso` entirely, so it sent no
   cookies and got the Entra sign-in page back under a 200 — the exact
   failure shape we documented and then failed to wire here. This is
   the longest-lived session vsp holds; it needs the refresh hook more
   than anything else does.

Also renamed `TestIntegration_DebugSessionAPIs` to
`…StatelessClientRefusesDebugCallsWithoutASession`. It asserts every
debugger call fails without a session — a fair check, the opposite of
debugger coverage, and misread as the latter for as long as it was
named that way.

## What is covered now

One cassette, recorded live.

`a4h-step.jsonl` (7.58): a whole debug session, plus listing breakpoints
over SAP's own ADT resource and a refusal reported with the server's own
sentence. `ZVSP_DEBUG_DEMO` stops
on its `PERFORM`; the replay catches the debuggee, reads the stack, reads
locals with values (`LV_COUNTER` = 7, the table described by its shape),
steps into the `FORM`, sees `IV_IN`/`CV_OUT` alongside the caller's
variables, and steps back out to line 28. No RFC channel, no
`ZADT_DEBUG`, no Z code — the claim, finally under test.

The demo program is committed at
`pkg/saprfc/testdata/fixtures/zvsp_debug_demo.prog.abap`. Deploy it to
`$TMP`, then:

```
vsp adt debug -s a4h --user <you> --record pkg/saprfc/testdata/cassettes/a4h-step.jsonl \
  -c "ebp ZVSP_DEBUG_DEMO 26; eclipse 120; estack; elocals; estep into; estack; elocals; estep out; estack"
# in a second process, while the listener waits:
vsp execute -s a4h 'SUBMIT zvsp_debug_demo AND RETURN.'
```

## What the second recording found

Three, all on 7.58, none of them test artefacts:

3. **`method=detach` is not a method SAP knows.** Every close burns a
   400 before falling back to `stepContinue`, which does work. Probed
   directly: `detach`, `detachDebuggee`, `POST …/debugger/detach` and
   `DELETE …/debugger` all fail, while `stepContinue` on the same
   attached session succeeds. SAP's message renders the *matched*
   method rather than the requested one, so it reads "Unknown method
   ''" — an empty name — which is why this looked like a malformed
   request rather than an unsupported one. The fallback the code
   already had is the real path, not the exception.
4. **Expanding an internal table returns nothing.** `echildren LT_ROWS`
   on a table holding two rows comes back with no children. The request
   is not obviously wrong: SAP reports `ID` equal to `NAME` for that
   variable, so the parent we send is the one it named. Tables likely
   need a different call than `getChildVariables`; worth probing
   against `canAdvancedTableFeatures`, which the step document
   advertises as true.
5. **`vsp execute` never returns output.** `WRITE`, `cl_demo_output`
   and a deliberate conversion dump all report "Executed successfully
   (no output captured)" — on every release tried. The code runs (the
   `SUBMIT` used to trigger these recordings really does reach the
   breakpoint), so this is the capture, not the execution.

## What is still not covered

`SetVariable`, `GoToFrame`, batch capture, `Record`, non-line
breakpoints, `SystemDebugging`, post-mortem. All are now cheap: the
choreography above is scripted and takes about a minute, so each needs
a script line and an assertion, not new machinery.

The conformance diff — the same script recorded on two releases and the
parsed results compared — needs a second system whose recordings may be
published. Only A4H qualifies today, so this waits on one.

The blocker it *used* to have is gone: under single sign-on nothing here
knew the logon name, so `--user` had to be typed. The transport
organizer answers it over plain ADT — asking for your own requests
returns a tree named after you, even when you own none — so
`vsp adt debug` now resolves it itself. See `pkg/saprfc/adtwhoami.go`.

## Cross-release conformance, and what it found

Run against three releases: 7.50, 7.57, 7.58. Only the 7.58 recordings
may be committed, so the comparison lives in `vsp compat` — the probe is
in the repository, the answers are not.

`vsp compat -s <a> --against <b>` now also diffs the debugger. Two
additions:

- The five non-line breakpoint resources — statements, conditions,
  messagetypes, validations, vit — are checks now. They are the only
  part of the debugger that answers with no session held, so they are
  the only part comparable without stopping a program.
- The report carries the debugger resources each release advertises in
  its own discovery document, and the diff shows what one has and the
  other does not.

Differences found, by release:

| resource | 7.50 | 7.57 | 7.58 |
|---|---|---|---|
| `/debugger/stack` | — | yes | yes |
| `/debugger/breakpoints/vit` | — | yes | yes |
| `/debugger/memorysizes` | — | — | yes |

Discovery under-reports: 7.50 does not advertise
`/debugger/breakpoints/vit` and yet answers it with 200. So absence from
discovery is a hint, not a verdict — the only proof is asking.

### The one that mattered

**7.50 has no `/sap/bc/adt/debugger/stack`.** The listener catches the
debuggee and the attach succeeds; the first stack read returns 404 "No
suitable resource found". Since the catch itself reads the stack, every
caught debuggee was thrown away, and the ADT debugger was simply
unusable on that release.

The stack is there — the release serves it from the dispatcher instead,
`POST /sap/bc/adt/debugger?method=getStack`, as the same `dbg:stack`
document the existing parser already reads. `ADTStack` now tries the
resource, falls back once on a 404, and remembers which shape answered,
so the discovery costs one request per session rather than one per step.
A refusal is not retried: a 403 or a 500 means something the caller
needs to hear, and trying another shape would hide it.

Verified end to end on 7.50 afterwards: stack, locals with values, step
into the FORM, step back out — the same script that passes on 7.58.

Two more reliability fixes came out of the same run:

- **A failed stack read no longer discards an attached session.** The
  catch reports the debuggee it caught and the stack problem separately.
  Without this, a release-specific gap in one read looked like "nobody
  stopped".
- **Every debugger request asked for `application/xml`.** ADT matches on
  URI *and* media type and reports a mismatch as 404 "No suitable
  resource found", not 406 — so a resource answering only its own vendor
  type reads as a resource that does not exist. All eight call sites now
  ask for `*/*`, the rule the transport layer already documents. This
  was not what broke 7.50's stack, but it is the same trap, and it did
  break the "who am I" lookup on 7.57 the first time it ran.
