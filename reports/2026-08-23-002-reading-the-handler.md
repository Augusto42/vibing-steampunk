# Read the handler

A technique, written down because it worked three times in one day and
each time it beat the alternative badly.

## The shape

When a system does something you cannot reproduce, the instinct is to
infer the rule from its behaviour: send a request, read the error, adjust,
send again. That works, slowly, and it fails in a specific way — you end
up with a rule that fits every example you tried and is wrong about the
first one you did not.

ABAP systems are unusually generous here, because **the thing that
answers you is readable**. The handler, its transformation, the class it
delegates to — all of it is source you can fetch over the same connection
you are already using.

## Three times today

**The AMDP breakpoint document.** Three rounds of guessing got the media
type and the root element from error messages, then stalled: the position
attribute was invented and wrong. `CL_AMDP_DBG_ADT_RES_BPS` names its
transformation in one line, transformations are readable at
`/sap/bc/adt/xslt/transformations/{name}/source/main`, and the template
states every element and attribute — including that the position is a
plain `adtcore` reference and nothing AMDP-specific. One request against
three rounds and a wrong answer.

**The AMDP session identifier.** The start response carries
`HANA_SESSION_ID`, so I took that for the session id and built on it. The
handler sets `me->main_id` from `debugger_id` and `me->session_id` from
`db_dbg_session_id` — two different fields, and the body returns the
second. The one I needed comes back in the `Location` header. Reading
twenty lines of `CL_AMDP_DBG_ADT_RES_MAIN` would have saved an afternoon
and a published claim that had to be corrected.

**The method include.** `CM001` had to become a method name. The class
object structure does not carry the include, a class-pool include is not
addressable as a program, no `SEO*` table pairs the two, and the
numbering is assigned at creation rather than by position — so ordering
is not a mapping however well it fits one example.

`CL_OO_CLASSNAME_SERVICE=>GET_METHOD_BY_INCLUDE` is SAP's own resolver,
and reading it looked like a dead end: it issues `SYSTEM-CALL QUERY
METHOD INCLUDE`, a kernel call we cannot make. But **the whole class
contains exactly one `SELECT`**, and it is on `TMDIR` —
`(CLASSNAME, METHODINDX, METHODNAME)`. The `CM` suffix is that index in
hexadecimal, which one live class settles: `CM001, CM003, CM009, CM00A`
decode to methods 1, 3, 9 and 10, and decimal has no `A` in it.

That last one generalises into the sharper rule:

> When SAP does something in the kernel, look at what the same class
> reads from a table. The kernel call is the fast path; the table is
> usually the same knowledge, and the table is readable.

## Why it matters more than it sounds

Every one of today's silent failures had the same origin: a rule inferred
from behaviour that fit the examples to hand. `'FU'` in a `C(1)` column,
because two-letter type codes are what one remembers. A section prefix
list covering `U01` and `F15`, missing `U27`. A media type guessed from
its neighbours. Each looked right, each was checked against exactly the
cases that had prompted it, and each failed silently on the next one —
returning an empty result rather than an error, so nothing ever said so.

Reading the handler does not merely answer faster. It answers with the
whole rule, including the parts you would not have thought to test.

## What it costs

One request, usually. `vsp source read CLAS <name>`, or the ADT source
resource for a transformation. The classes are large — 1700 lines for
`ZCL_XRAY_GRAPH`, 900 for `CL_OO_CLASSNAME_SERVICE` — but the question is
almost always answerable by grepping for `SELECT`, for the transformation
name, or for the one method you care about.

The failure mode to avoid is reading the whole thing into a conversation.
Grep it.
