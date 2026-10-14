# Handover: the freeze run, and what it left

Written at the end of the triage half of the feature freeze, for the session
cutting the release and for whoever comes after. It supersedes
`contexts/2026-08-24-graph-handoff.md`, whose "where to resume" list is now
entirely done — including the item it marked *not yet*. A handoff that promises
work already finished is the same untruth this week has been spent removing, so
it is superseded here rather than left to be discovered.

## What state the sprint is in

Three systems swept on one build, releases **758, 757, 750**, forty-two probes
each. **No `broken` verdict on any system.** The triage is in
`agenda/2026-08-25-003-three-system-triage.md`: eight differences, none of them
about us — two belong to a release, four to what a system contains, two to a
target that is deliberately heavy.

Everything of mine is merged. Nothing is in flight on my side.

Outstanding before the cut, and not mine:

- **A needs re-taking.** The 758 report was captured on an older build and
  carries no release. Two of the eight differences involve it, so the record is
  short of one matched artefact.
- Probes for the eleven newly-routed capabilities. Agreed by both sessions to
  be **after** the release, not during: adding them now would move the surface
  the freeze exists to hold still, and they would be the only capabilities
  unverified by the run they joined.

## The tooling, and where it lives

`.local/freeze/compare-sweeps.py` — untracked on purpose. It reads sweep JSON
and prints only the differences.

It is anonymised **by construction, not by discipline**: it names what it keeps
— build, release, and per probe `{id, capability, verdict}` — and nothing else
enters the process. That is a whitelist because the first version was a
blacklist and the blacklist was wrong: it dropped the one field known to name a
system, and `targets.Dump` turned out to be a 150-character path with the
instance name and a user packed inside it, no dots and no word boundaries. My
own check for a leak looked for a hostname with dots and found nothing. **It
could not have failed.**

That is the third time in a week a check or a fixture modelled something the
data does not do. It is worth reading as a class: *a test that cannot fail
reports a guarantee nobody has.*

## What the run cost, and the one lesson worth carrying

Four restarts. Only the first — a build with no version stamp — was avoidable
by care. The other three were defects, and none of them could have been found
on one system:

- a probe asserting its target's type, which held on 758 by luck and broke on
  757;
- a resolver filtering after its query, so a row limit starved it and it
  reported "no package found" as a fact about the system;
- a report that named no SAP release, which left every absence unattributable.

> Each needed a second system to disagree with the first.

That is the argument for the three-system run itself, and it is stronger than
the argument the sprint was opened with. Sharpened by the release session into
the form worth keeping:

> **A second system is needed not for coverage but for disagreement.**

The distinction is the actionable half. Coverage says "run it in more places and
you will see more"; disagreement says *what kind* of defect only a second place
can show — the ones that hold because one system happens to agree with itself. A
probe asserting a target's type was right by luck. A resolver filtering after
its query starved only where local packages happened to fill the limit. A report
naming no release did no harm while there was nothing to compare it against.
None of the three is visible from inside.

## Two things to hold on to

**A caveat is where an investigation starts, not where it ends.** The honest
"55 of 222 objects could not be read" was correct and covered a defect: all 55
were readable, at an address nobody had asked. Being truthful removed the
pressure to ask why.

**The name is not the address.** Three times now the catalogue and ADT have
disagreed about where something lives — a `PROG` that is an include, a class
section addressed as its object, a target whose type was assumed. The shape
repeats and the symptom is always the same: everything answers, nothing fails,
and the answer is about something else.

## Open, and belonging to nobody yet

- **One registry every router registers into.** `advertisedCapabilities()` is
  assembled from the analyze tables plus a list of routers named by hand, and it
  was wrong once already — four capabilities became reachable and entered no
  count, so coverage read "39 of 39" while naming nothing as unprobed. It reads
  39 of 51 now. The durable fix is a registry, after the freeze.
- **`vsp sweep` printing a full dump identifier in `targets`.** It is there for
  reproducibility and it is the one place the report carries a host and a user.
  A product question, not a freeze question.
- The four families the address-mismatch measurement cannot see, listed in
  `agenda/2026-08-25-002`. Local classes are the one to watch: they are in no
  catalogue, so nothing lists them, nothing can miss them, and no dependency
  reader asks for them.
