# Three systems, forty-two probes, and what the differences mean

The freeze run. Three systems, named A, B and C by release rather than by name,
because a verdict about a release is a statement and a verdict about a system is
an anecdote — and because nothing that identifies a system belongs in a tracked
file.

| | release | probes | build |
|---|---|---|---|
| A | 758 | 42 | `v2.50.0-19-g4e33a6c` |
| B | 757 | 42 | `v2.50.0-17-g1e2f7eb` |
| C | 750 | 42 | `v2.50.0-17-g1e2f7eb` |

**No `broken` verdict on any system.** That is the headline: not one defect of
ours survived into the release run.

The two build strings differ by two commits and neither touches `.go` — checked
with a diff rather than asserted, because "that commit was only documentation"
is a rule that holds until somebody puts a help string a probe matches into a
documentation commit. The runs are therefore comparable, and saying *why* they
are is the part worth keeping: three of the four re-runs this record cost were
caused by comparing across builds that were not.

Cut as **v2.51.0**.

## The eight differences, and which of them are about us

None. Every difference is a fact about a release, a system's contents, or a
target — which is what the sprint set out to establish and is not what it found
on the first attempt.

### Attributable to the release: two

`analyze type=sql_trace_state` and `analyze type=list_sql_traces` answer on 758
and 757 and return **404** on 750, at `/sap/bc/adt/st05/trace/…`. Two
independent signals:

- the status is 404 rather than 406, 500 or a timeout — the resource is not
  there, as against there and refusing;
- the same code and the same build get a real answer from the two newer
  releases.

**A third signal was wanted and is not available.** The plan was to require
"missing from discovery **and** 404", because discovery lies in both directions.
Neither `compat` nor the discovery document mentions ST05 at all on any of the
three, so discovery cannot testify here either way. Two of three, and the third
is named rather than assumed.

### Attributable to a system's contents: four

`core.grep`, `pm.dump_impact`, `pm.list_traces` and `pm.get_trace` differ
because one system has dumps or traces and another does not. A quiet system is
quiet; none of this is about the code.

`pm.get_trace` reads `absent` on one and `skipped` on the others, and the
difference is worth keeping rather than smoothing: `skipped` means no target was
found before the probe ran, `absent` means the resource did not answer. The
second is honest about a system, the first is honest about the sweep. They are
not interchangeable.

### Attributable to the target: two

`analyze type=callers` and `analyze type=call_graph` come back `timed-out` on
750 — the verdict that exists so that a slow answer is not filed as a broken
capability.

The cause is the probe's default target, an object whose where-used list is
enormous, against the slowest of the three systems. Discriminated by
measurement rather than argued: two ordinary classes on that same system answer
the same request without difficulty. The resource is alive; the budget is not
enough for that one target.

The target was deliberately not made lighter. A heavy target is the test.

## What the run cost, and why

Four restarts, and each one is worth naming because only one of them was
avoidable.

1. A build with no ldflags — `build: dev`, a baseline that cannot be dated
   against a fix. Both of us did this, independently, within an hour.
2. A probe asserting a target's type. On 758 the target happened to be a class
   and the assumption held; on 757 it did not, and the sweep reported `absent`,
   which reads as a fact about the release.
3. A resolver that filtered after the query, so a row limit could starve it —
   and then reported "no package found" as a fact about the system.
4. The report not carrying the SAP release, which made every absence
   unattributable.

Only the first was avoidable by care. The other three were defects that a
single-system run could not have surfaced: each one needed a second system to
disagree with the first.

## The coverage figure, stated because it is easy to misread

The run reports **39 of 51 capabilities probed**, and names the twelve it did
not. It said "39 of 39" until the denominator was taught about four capabilities
that had become reachable and had entered no count — not probed, not named as
unprobed, not counted. That figure was arithmetically true and read as complete.

Probes for the remainder are deliberately **not** written yet: adding them
during the freeze would move the surface the freeze exists to hold still, and
they would be the only capabilities unverified by the run they joined.
