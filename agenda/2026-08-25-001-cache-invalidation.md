# The cache: the invalidation signal, settled

**Status:** `boundaries` is 11× faster without a cache. The signal a cache needs
is now **established and sound** — this document said the opposite for an hour,
and the correction is kept in full below because how it went wrong is the more
useful half.

## What was claimed

The April plan: `vsp boundaries '$ZDEMO'` goes from ~60s (227 source fetches) to
~2s with the SQLite cache wired. Milestone 1 landed — configuration, read and
printed by `vsp systems` — and milestones 2 through 6, including the one the
plan annotates *"← main value"*, did not. `pkg/cache` is 977 lines with zero
importers.

## What was measured

The timing claim holds. On a live 7.58 system:

| package | objects | before |
|---|---:|---:|
| SAI_PROXY_VERI | 11 | 0.7 s |
| SBRF | 167 read of 222 | **18.8 s** |

So 227 objects at about a minute is right, and the cost is round trips: the
parse of all 167 sources is milliseconds.

## The question a cache has to answer first

**What invalidates a cached source?** Caching without a sound answer serves
stale code and analyses something that is not there — which, for boundary and
dependency verdicts, is the confidently-wrong class this project has spent the
week removing. Three candidate signals, all probed:

| signal | cost | verdict |
|---|---|---|
| `ETag` / `Last-Modified` on the source read | one round trip **per object** | correct, and no use: the round trips *are* the cost |
| `REPOSRC.UDAT`+`UTIME` | one query per object prefix, 0.4 s | **unsound — see below** |
| `SEOCLASSDF.CHANGEDON` | one query, bulk | date only, and it agrees with REPOSRC, not with ADT |

### First conclusion: REPOSRC is not the answer — **and it was wrong**

The obvious construction is: one bulk query gives a change time per object,
compare against the cache, fetch only what moved. It fails on measurement.

`source/main`'s ETag and the repository's own timestamps **disagree in both
directions**:

| class | ETag | `…CU` include | max over all includes |
|---|---|---|---|
| CL_ABAP_TYPEDESCR | 20200316133949 | 20200316133949 | 2025-12-01 |
| CL_ABAP_ELEMDESCR | 20230517085041 | 20230517085039 | — |
| CL_HTTP_CLIENT | **20241010160847** | 20230517085038 | 20230517085038 |
| CL_SALV_TABLE | **20230615133422** | 20220519104316 | — |

The first line is why this looked promising: for one class the ETag *is* the
`CU` timestamp, exactly. On the next three it is out by two seconds, by a year,
and by a year. And `CL_HTTP_CLIENT`'s ETag is **later than every source record
the repository holds** — 2024 against 2023 everywhere, with `SEOCLASSDF` also
saying 2023.

So one of the two tracks the bytes ADT returns and I cannot tell which. Building
on REPOSRC would be building on the hypothesis that survived one example, which
is the failure mode named four times in this month's reports.

**A cross-run source cache is therefore not shipped.** Not "not yet worth it" —
not soundly possible on a signal this system has been shown to offer.

### The correction, from a controlled experiment

The paragraph above is wrong, and it was wrong for an avoidable reason: **the
probe that produced it read 40 rows of a class that has 63 includes.**
`CL_HTTP_CLIENT================CM00Q` carries `20241010160847` — which is the
ETag, exactly — and it was past the cut. A rule inferred from four unexplained
examples, when a fifth row would have explained them.

What settled it was creating a Z class and changing it, rather than inspecting
standard objects whose history nobody can reconstruct. `ZCL_VSP_00_STAMP_TEST`
in `$TMP`, created, method body changed twice, public section changed once, then
deleted:

| step | source sha | ETag | max REPOSRC | moved |
|---|---|---|---|---|
| create | e7107994 | …053314 | …053314 | all |
| edit 1 | 96e5a49c | …053316 | …053316 | CM001, CS |
| edit 2 | 8e429838 | …053318 | …053318 | CM001, CS |
| signature | 62988786 | …053321 | …053321 | CM002, CU, CP, CS |

Four changes, four distinct hashes, and the ETag equals the maximum every time.
Note that `CU` does **not** move on a method-body change and `CM001` does: no
single include is the signal, the maximum is.

### The rule

> **stamp = max(REPOSRC over the object's includes, excluding `CS`)**

`CS` is regenerated. It moves without the source changing — several standard
classes carry today's timestamp on it, hours after nobody edited them — and
including it drops agreement across ten standard classes from **eight to zero**.

Of those ten, eight match the ETag exactly and two are **later** than it:

| class | ETag | max excluding CS | |
|---|---|---|---|
| CL_ABAP_TYPEDESCR | 20200316133949 | 20200316133949 | = |
| CL_HTTP_CLIENT | 20241010160847 | 20241010160847 | = |
| CL_GUI_FRONTEND_SERVICES | 20221209144808 | 20221209144808 | = |
| CL_SALV_TABLE | 20230615133422 | 20241010160755 | later |
| CL_ABAP_UNIT_ASSERT | 20230210142543 | 20230404131642 | later |

Ten of ten are **≥ the ETag**, and that direction is the whole safety argument:
a stamp never older than what ADT serves can invalidate too often and never too
rarely. Over-invalidation costs a fetch. Under-invalidation serves code that is
not there, under a verdict somebody acts on.

## What was shipped instead

The fetches were serial. Six workers, results assembled **in input order** so
concurrency cannot reach the output:

    SBRF: 18.8 s → 1.6 s

Eleven times faster, byte-identical JSON across three runs, no staleness risk of
any kind, and better than the cache plan's own target. The parse was never the
problem.

## What is left, for whoever picks it up

- **The cache is now buildable, soundly.** `SourceStamps` implements the rule
  and its tests pin the two things that break it: the `=` padding that separates
  `ZCL_ORDER` from `ZCL_ORDER_ITEM`, and the `CS` exclusion. `pkg/cache` already
  has the right model — `SourceHash`, `LastModifiedADT`, `Valid`.
- **Whether it is worth building is now a different question.** `boundaries` is
  1.6 s without one. The case for a cache is the *other* scans, repeat runs, and
  large customer packages — not this command.
- **One cost to measure first:** the stamp itself is one query per class,
  because SAP freestyle rejects more than one LIKE per WHERE. For 137 classes
  that is 137 queries, which is what the cache was meant to avoid. Either find a
  bulk form, or accept that the cache pays off only when the sources are large
  relative to the stamps.
- **The other scans are still serial.** `health`, `slim`, `api-surface` and
  `cr-config-audit` all walk objects one at a time. The same six workers apply,
  and the same rule with them: assemble in input order.
- **`pkg/cache` still has no importers.** It is 977 lines describing exactly the
  right model — `SourceHash`, `LastModifiedADT`, `Valid`. Adopt it when the
  signal exists, or delete it, but do not leave it looking wired.
