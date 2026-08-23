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

## The wall: decoding a method include

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
