# Upward tracing: what the tables give, and where it stops

Investigated against a live 7.58 system. This is the piece ZRAY never
built either — `BUILD_UP` exists there as a method and was never really
done — so there is nothing to copy and the ground has to be mapped.

## What is settled

**Both cross-reference tables are readable over plain ADT free SQL.** No
RFC, no Z code, no gateway. That was the precondition for doing this in
Go at all and it holds.

**Downward is done.** `pkg/adt/callees.go` resolves an object to its
includes (classes by their `=` padding, function modules through TFDIR),
reads `CROSS` and `WBCROSSGT`, filters each row back to its owner so a
prefix-sharing sibling cannot leak in, drops `INDIRECT` and
self-references, and marks invocation versus type reference.

**Upward, at object level, works** through the where-used list.

## What the columns actually hold

Checked rather than assumed, because two of these are misleading:

| | |
|---|---|
| `CROSS.TYPE` | `C(1)`. Invocations: F, R, T, U, P, D. A two-character value is rejected with 400, and that 400 has twice been read as "nothing found". |
| `WBCROSSGT.OTYPE` | `C(2)`. `TY` type references, `DA` data objects. |
| `WBCROSSGT.COMPONENT` | **A flag, not a name.** `C(1)`, holds `X` on the row where an object describes itself. The component is packed into `NAME` with a backslash — `ZCL_X\DA:GT_SERVICES`. The column name invites the opposite reading. |
| `WBCROSSGT.INCLUDE` | Where the reference sits. For a class this is a *section* — `…===CI` for the definition — or a method include, `…===CM001`. |
| `CROSS.PROG` | Present, and empty in every row sampled. Not a shortcut to the owner. |

## Correction: the wall was never there

Everything below about decoding a method include is true and was not
needed. **ADT's where-used already returns the method**, as child rows,
and this codebase already read them: `ExposedCaller.Component`, with the
comment "the method or routine holding the reference". Live, it is the
"References in" column — `PUSH_AUTO`, `CREATE_OBJECTSET`,
`GOTO_SOURCE_CODE`.

The code that reads it landed at 01:16. The analysis below, concluding
that the method was "not available from anything ADT exposes", was
written at 19:37 the same day, in the same repository, eighteen hours
later.

Every attempt listed below asked the same question — how to get from an
include to a method — and none asked whether the resource already being
called answered it. That is today's defect class one floor up: not
"broken", but "nobody checked whether it already works".

`DecodeMethodIncludes` stays. It is correct, it is tested, and it answers
a question upward tracing does not have to ask — a dump stack frame gives
an include and no method, so it earns its place there. It is simply not
the missing piece this note called it.

## What the wall looked like

`TMDIR` — `(CLASSNAME, METHODINDX, METHODNAME)` — and the `CM` suffix is
that index in hexadecimal. `adt.DecodeMethodIncludes` does it, verified
live. The route to it is recorded in
[2026-08-23-002](../reports/2026-08-23-002-reading-the-handler.md); what
follows is what was tried first and did not work, kept because each
attempt looked reasonable.

## The wall as it looked: decoding a method include

Upward tracing at object level needs `INCLUDE → object`, and that is
`NormalizeInclude`, now fixed — it used to resolve `LZDEMO_FGU27` to a
*program* named after its own include, because it matched a fixed list of
section prefixes that covers U01 and F15 and misses U27.

Upward tracing at **method** level needs `INCLUDE → method`, and that is
not available from anything ADT exposes.

What was tried and did not work:

- The class object structure lists methods with visibility, level and
  redefinition, and **no include**.
- `/sap/bc/adt/programs/includes/{include}/source/main` answers 500 for a
  class-pool include; the class's own `…/includes/` path answers 404. A
  method include is not addressable as a program.
- No `SEO*` table pairs `CMPNAME` with an include. `SEOCOMPOSRC` has
  three columns and none of them is one.

And the numbering does not carry it either. For one class the includes
present were `CM001, CM003, CM009, CM00A` — **hexadecimal**, and sparse,
because only methods that reference something appear at all. `CM001` is
that class's `class_constructor`, which is also first in the source, but
that is a single coincidence: the number is assigned when a method is
created, not by its position in the text, so a method written later and
inserted earlier gets a higher number. Ordering is not a mapping.

## Two routes, and their honest cost

**Content matching.** For each `CM` include we know what it references,
from the rows themselves. Parse the class with `pkg/abaplint` — it has a
real ABAP parser — collect the names each method mentions, and match. A
unique match decodes the include; an ambiguous one says so rather than
guessing. This needs no server-side anything and degrades honestly.
Cost: a parse per class, and the matching logic.

**Accept object level.** ADT's own where-used ignores a method-level URI
and resolves it to the class — SAP works at object granularity here too.
Saying "this class calls it, and here is the include" is already more
than the tools had yesterday.

The second is available now. The first is worth doing only if
method-level upward tracing turns out to be something anyone asks for
twice.

## Not to be repeated

The include mapping was heuristic in both directions and wrong in both.
Downward it guessed a section prefix from a list; upward it would have
guessed a method from a number. Both look like they work on the examples
that were to hand — `U01`, `CM001` — and both fail on the second example
nobody tried. A test written five minutes after the fix caught the
replacement being loose in a different way, accepting `LEGACY_REPORT` as
a function pool named `EGACY_REP`.

If a mapping cannot be read from the system, it should fail visibly
rather than resolve to something plausible.

## An operational hazard found alongside, worth stating plainly

A second SAP user was created so two sessions could debug without
clobbering each other. It worked for what it was meant to: external
breakpoints key on `USERNAME`, and `im_max_dbg_contexts` is per user, so
both separate.

It was not a clean win. **The AMDP debug work-process pool is not
partitioned by user** — it is a system-wide resource, and A4H appears to
have very few. So isolation bought a new kind of deadlock: a session that
dies without calling `astop` leaves a work process held, that orphan is
**invisible to the other user**, and `stopExisting` only ever reaches
your own. The one who took it is the only one who can give it back.

Two consequences:

- `astop` is mandatory on the failure path, not only the happy one. A
  `-c` script that is interrupted mid-`aresume` never reaches it.
- Before planning around parallel AMDP debugging, someone has to find
  out **how many of those processes exist**. If it is one, a second user
  buys nothing for AMDP however well it separates everything else.

The general shape is worth remembering: separating identity separates
what is keyed by identity, and leaves everything underneath shared —
while removing the ability to clean up across the boundary you just
drew.

## `--user` should stay local, and this is a decision rather than a gap

`--system` is a persistent flag and `--user` is not, so `vsp -u X <cmd>`
fails with an unknown-shorthand error while `-s` works. The obvious fix
is wrong.

Nine subcommands declare their own `--user`, and in most of them it means
something else: a *filter* in `dumps`, `applog` and `transport list`, and
"whose debuggees to listen for" in the two debug shells. The root one is
the logon account. Making it persistent would put `-u, --user "SAP
username"` in the Global Flags of every subcommand that already has a
local `--user` meaning something different — and the resulting mistake
would be silent, because filtering by the wrong user returns an empty
result rather than an error.

A named profile in `.vsp.json` is the right way to run as a different
account. What could fairly be improved is the error message, which is
confusing precisely because `-u` does exist at the root.
