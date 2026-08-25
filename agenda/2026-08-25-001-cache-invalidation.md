# The cache: measured, and not built

**Status:** `boundaries` is 11× faster without one. The cache stays unwired, and
this says why, so nobody re-derives it.

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

### Why REPOSRC is not the answer

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

## What was shipped instead

The fetches were serial. Six workers, results assembled **in input order** so
concurrency cannot reach the output:

    SBRF: 18.8 s → 1.6 s

Eleven times faster, byte-identical JSON across three runs, no staleness risk of
any kind, and better than the cache plan's own target. The parse was never the
problem.

## What is left, for whoever picks it up

- **Find what the ETag actually tracks.** If it is a per-object version the
  repository records somewhere queryable in bulk, the cache becomes sound and
  worth building. `CL_HTTP_CLIENT` is the case to explain: what happened to it
  on 2024-10-10 that moved the ETag and no source record?
- **The other scans are still serial.** `health`, `slim`, `api-surface` and
  `cr-config-audit` all walk objects one at a time. The same six workers apply,
  and the same rule with them: assemble in input order.
- **`pkg/cache` still has no importers.** It is 977 lines describing exactly the
  right model — `SourceHash`, `LastModifiedADT`, `Valid`. Adopt it when the
  signal exists, or delete it, but do not leave it looking wired.
