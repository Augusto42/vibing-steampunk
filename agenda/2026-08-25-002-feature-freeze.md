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

**The one exception to argue about, not to assume.** `vsp sweep` cannot compare
two systems; `vsp compat` can, and the whole sprint is a three-system
comparison. Either the sweep gains `--against`, or the comparison is done by
hand and by diffing JSON. That is tooling for the freeze rather than product
capability, and it is the only thing worth breaking the rule for. Decide before
starting, not halfway through.

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

1. **Decide the `--against` question.** Ten minutes, and everything downstream
   is shaped by it.
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
