# Feature freeze, and what a clean release means

**Opened:** 2026-08-25, after v2.50.0.
**Purpose:** finalise the tool as it stands, and be able to say so with evidence.

## Why now

Five releases in two days, and every one of them fixed something found while
fixing something else. That is the right way to run a week of repair and the
wrong way to run a month: it never reaches a state anyone can point at and call
finished. This sprint reaches one.

The claim at the end should be narrow and true: **on these three systems, this
build does what it says, and here is the record.** Not "it works" — that is what
the ten dead features said.

## The freeze, operationally

Until this sprint closes:

**Allowed.** Fixing what exists. Tests for what exists. Documentation that
corrects a claim. Making a capability reachable that was already advertised.

**Not allowed.** New capabilities. New commands. New `analyze` types. New
surface of any kind — including "small" ones, because the whole point is that
the surface stops moving long enough to be verified.

**The exception was argued and refused.** `vsp sweep` cannot compare two
systems and the whole sprint is a three-system comparison, so `--against`
looked like tooling rather than capability. Three arguments against it, and the
third is the one that decides:

1. Nothing is missing. `sweep --json` exists and its own flag help says "for
   diffing or for a record". `--against` would add the join, which is the
   cheapest part.
2. It would be the only capability not covered by the run it was added for. A
   comparison tool that is itself unverified is a report saying GOOD over a
   check that did not run — the shape this very tool was built to catch.
3. **`SweepReport` carries a `system` field.** A tool that by construction holds
   two systems in one process makes anonymisation a step somebody has to
   remember. A manual step makes it structural: the raw reports stay in
   `.local/` and the tracked artefact is a separate, deliberate act. This
   document says a three-way diff tempts an exception because the real names
   read better — and then proposed giving the product the ability to put them
   side by side.

The replacement lives at `.local/freeze/compare-sweeps.py`, untracked. It eats
what `sweep --json` already emits and is anonymised **by construction**: the
`system` field is dropped at read time, once, so nothing downstream can print
it; inputs are A, B, C by command-line order; the release comes from the
report's own build line. If the comparison turns out to be needed permanently,
it enters the product afterwards, with tests and a probe, and not as an
exception.

## What "clean" has to mean

A release nobody can check is a claim. These are checkable:

1. **`go test ./...` green**, all 17 packages, on a clean tree.
2. **`vsp sweep` on each of the three systems**, with the build stamped in each
   report — the report names the binary it exercised, and that rule was earned
   twice.
3. **Every finding classified**, and this is the work: on 7.50 many capabilities
   are genuinely absent, and `absent` is a fact about the system while `broken`
   is a fact about us. A run that cannot tell them apart proves nothing.
4. **`vsp compat` on each**, diffed pairwise, so a difference in the sweep has a
   routing explanation rather than a shrug.
5. **Published counts pinned and correct** — 1 / 102 / 147, and every copy.
6. **No claim in README or CLAUDE.md that the sweep contradicts.**

## The three systems, and the hard rule about two of them

The tool is verified against **a4h, d15 and ms1** — three releases, three
shapes, which is the point.

> **Nothing from d15 or ms1 enters the repository.** Not a hostname, not a
> user, not an object name, not a transport, not a dump, not a JSON report with
> a system field in it. Only a4h may be named in tracked files.

This sprint is exactly the activity that tempts an exception — a three-way diff
is so much more legible with the real names in it. The tracked artefact is
therefore **shaped counts and verdicts, with the systems as `A`, `B`, `C` and a
release number**, and the raw reports live under `.local/`. Anyone who needs to
map them back has the untracked copy.

The check before every commit in this sprint is the one already in CLAUDE.md,
and it should be run rather than remembered.

## Order of work

1. ~~Decide the `--against` question.~~ **Refused — see above.**
2. **Baseline on a4h.** The known-good, so a difference elsewhere has something
   to be a difference from.
3. **Run d15 and ms1.** Expect absences: 7.50 has no dump detail resource, no
   AMDP, a different debugger surface. Expect at least one real defect — three
   systems have never been swept.
4. **Triage every non-`answered` verdict** into: absent by release, refused by
   authorisation, our defect. The third list is the sprint's work.
5. **Fix the third list.** This is where the freeze earns itself: no new
   capability, only what is already claimed made true.
6. **Re-run all three.** A fix verified on one system is not verified.
7. **Correct the documentation** to whatever the three runs actually showed,
   including release qualifiers the README currently does not carry.
8. **Release, with the record attached.**

## What would make this fail

- **Finding something interesting and building it.** The freeze exists because
  this is what happened five times this week, each time correctly.
- **A clean sweep read as proof.** It covers 39 advertised capabilities. The
  CLI has commands the sweep does not probe, and the coverage line says so —
  the claim at the end must be as narrow as the evidence.
- **A run against a stale binary.** It has cost this project two evenings, in
  both directions. The build stamp is in every report for this reason.
