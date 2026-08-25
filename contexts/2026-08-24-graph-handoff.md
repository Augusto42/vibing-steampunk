# Handoff: the graph, after the sweep

> **Superseded 2026-08-25** by
> [2026-08-25-freeze-handover.md](2026-08-25-freeze-handover.md). Everything in
> "Where to resume" below is done, including the item marked *not yet* — the
> sweep exists as a command, the fifteen are probed, and the traversal question
> was answered by building the load graph instead. The findings and the two
> lessons at the end still hold; the plan does not. Left in place rather than
> edited, because what a plan looked like before the work is worth more than a
> tidy record of it.

Entry point for whoever picks this up — including me, later. Read `CLAUDE.md`
first, then this. Everything else is linked from here rather than repeated.

## Where things live, because they are deliberately not in one place

| | |
|---|---|
| **This file** | the single entry point: state, where to resume, and my view of the direction |
| `agenda/AGENDA.md` | the living board — what is open, what was decided, one line each |
| `agenda/2026-08-24-001-graph-surface-sweep.md` | the sweep: all fifteen capabilities, what each answered, what was done |
| `agenda/2026-08-23-004`, `-005` | the previous audit and the upward-tracing investigation. **`-005` was corrected**: what it called impossible already worked |
| `contexts/2026-08-23-graph-handoff.md` | the handoff I received. Still accurate on data and traps; its "order I would keep" is superseded below |
| `reports/2026-08-23-002-reading-the-handler.md` | the technique that answered five times out of five |
| the code | the facts that must not be lost live in comments beside what they describe, not in reports — see `graphFromCross` |

Rule of thumb: **a fact goes where the person who needs it will be standing.**
Something the next editor of a function must know goes in that function. Something
the next person choosing work must know goes on the board. Only the shape of the
whole goes here.

## State

Branch `feat/graph-forward`, **ten commits ahead of `main`**, tree clean,
`go test ./...` green, no conflicts with `main` (which moved five commits under
me and was rebased onto cleanly).

```
3a9d538 docs(graph): what the fallback could do, and why it was not done
e678848 fix(graph): trace_execution went quiet in four places…
eb2dc44 docs(agenda): the graph surface, exercised rather than counted
b3a3bbc fix(graph): check_boundaries called a package clean without opening a file
b1b4f29 fix(graph): two nodes for twenty-seven edges…
84487ae fix(graph): the references answer was too large to be read
62c4c8e fix(graph): the parser invented a function module out of a variable name
bdde5e4 docs(agenda): outcomes of the sweep, and four gaps from a neighbour
e58097d fix(deploy): a function module could be created from a file but never updated
438c3d8 feat(edit): a DDIC table can be edited like any other source
```

Five earlier commits are already in `main` and shipped in **v2.45.0** — the
callee gaps, the SHA-1 decode, the inactive index. The release notes name the
`Callees` signature change as breaking.

**Two of the ten unblock somebody now.** The ABAP-IRC project is working around
`e58097d` and `438c3d8` by hand. Landing them is the first thing worth doing.

## Vision — what this area is for

The graph is not a graph. It is **a set of primitives that answer questions
about a live system, each honest about what it cannot see.** The traversal on
top is the part that keeps being wrong; the primitives are the product. That is
not my idea — it is in the handoff I received, and everything since has agreed
with it.

What today added to that:

**Two tiers of defect, and we had only been fixing the lower one.** A silence
withholds an answer and can at least be noticed — an empty list is visible. An
invented answer cannot: a SHA-1 reported as an object name, a function module
conjured out of a variable, a CLEAN verdict on a package nobody opened. *Silence
is loss; invention is corruption.* Rank work by that.

**The protection is not documentation.** Description without verification rots
silently — that is exactly how ten undocumented capabilities decayed. But
verification without description does not know what a right answer is. The pair
is what holds: **claims a machine can falsify.** Not "shows callers", but "on an
object with callers, answers non-empty; on one without, answers empty and says
the query ran".

**And every claim about the surface must be made against the surface.** Three
times today a belief about what exists went unchecked: a list two entries short,
a capability declared impossible that had shipped eighteen hours earlier, and a
whole sweep nearly recorded against a server running yesterday's binary.

## Where to resume

In order. The first is minutes; the rest are choices.

**1. Land the ten.** Either the release session takes them as before, or merge to
`main` and tell them. Until then two projects are blocked on work that is done.

**2. `edit` should say what it knows after activating a function group.** An RFC
session keeps the loaded group, so a call after activation runs the *old* code —
successfully, with a well-formed answer. The symptom is indistinguishable from
"my edit was wrong", which is what makes it expensive. Either reopen the
connection or put one sentence in the answer. Small, and it saves somebody an
hour. Reported by ABAP-IRC, reproducible on a4h.

**3. The sweep as a command.** Proposed by the release session; I agree, and
tonight is the argument for it — fifteen capabilities called by hand found five
defects and two more nobody was looking for. `vsp compat` already has the shape:
checks, report, JSON, two-system comparison. **It must name the build it
exercised.** That rule cost time twice today, in both directions.

**4. Then describe the fifteen, in the falsifiable form above.** With (3) holding
it, description stops being a claim and becomes a test. Without (3) it is the
thing that rotted last time.

**Not yet:** the traversal layer. It needs decisions about snapshots and what
counts as a node, with ZRAY's model in front of you — `agenda/2026-08-23-004`
has that reading. It is the wrong work for the end of a long night.

**Not mine:** ABAP Channels in `create`. The recipe is complete on the board,
read out of abapGit's own handlers, and it closes the whole family in one go —
but it is hours, and it belongs to whoever owns object creation.

## Open, needing somebody's decision rather than somebody's time

- **`graph_stats` is narrower than its name.** It analyses source handed to it
  and cannot be asked about a repository object. Widen it or rename it; either
  is defensible, leaving it undocumented is what was not.
- **`WBCROSSGTI` is used to explain an empty answer, not to fill it.** Active and
  inactive references are disjoint per object. Merging them into one list needs
  somebody to decide what a reader of that list expects; I deliberately did not.
- **Two reports from ABAP-IRC await a fresh binary**: `create TABL` silently
  producing two client fields, and ABAP Channels in `create`. Both were observed
  on a stale image. Check the build, then check the code anyway — that corollary
  is on the board and cost us the evening in both directions.

## Two things not to relearn

**`Unsearched` means "could not look", and nothing else.** I stretched it to
carry a fact I had looked up, and produced output saying "1 of 2 tables could not
be searched" directly above a sentence proving one had been. Caught on the live
output, not on the tests — the tests asserted a number, not what the number
*said*. Test the sentence.

**Read the handler.** When SAP does something in the kernel, look at what the
same class reads from a table. Five for five today, and twice it answered more
than was asked — that is how `WBCROSSGTX` and the whole long-name defect were
found, from twenty lines of somebody else's ABAP.
