# Is the catalogue-versus-address mismatch the only one?

`PROG` that is really an include was found by accident: an honest caveat said a
quarter of a package could not be read, somebody asked why, and it turned out
the objects were being addressed as executable programs when ADT keeps them
under `/programs/includes`. Fixed in `4dff03f`.

The question this note answers is whether that was the only such pair. Mostly
yes, within what was measured — and the boundary of "what was measured" is the
part worth reading.

## What was measured

**Every program type SAP records, against what the catalogue calls it.** 295
`R3TR PROG` entries sampled from TADIR, looked up in REPOSRC by name, 192
resolved:

| `SUBC` | what it is | count | reads at `/programs/programs` |
|---|---|---|---|
| `1` | executable | 98 | yes |
| `I` | include | 54 | **no — 404** |
| `S` | subroutine pool | 37 | yes |
| `M` | module pool | 3 | yes |

Only `I` is a mismatch. Subroutine and module pools were checked **against a
build with the include retry removed**, because the retry would otherwise have
made them look fine whatever the truth was: the same run reads a known include
as 404, which is the control that says the test could have failed.

**What is still unreadable after the fix.** Twelve standard packages scanned:
one object, and it is a 500 rather than a 404 —
`VIM_CHECKS====================VC`, an include by REPOSRC, which answers
"could not be successfully read" at *both* addresses. Not a third address: an
object ADT does not serve. The retry is deliberately not widened to 500, which
is a real server error and would be turned into two by retrying it.

## What was not measured, and why that matters

The scan reads what the package scanners read: `CLAS`, `PROG`, `INTF`, `FUGR`.
So this says nothing about four families where catalogue and address diverge by
construction, and a mismatch there would be invisible to this method:

- **Function group members.** Modules and group includes live under the group,
  not beside it.
- **Interface pools**, addressed differently from the interface.
- **`DDLS` / `BDEF` / `SRVD`**, each with its own root.
- **Local classes** — `CCIMP`, `CCDEF` and the rest are not in TADIR at all, so
  nothing can list them and no package scan can miss them. A dependency reader
  that wants them has to ask on purpose, and none does.

The last is the one to watch. A missing object shows up as a gap; an object
nothing ever asks for shows up as nothing.

## The lesson, which is not the same as "silent failure"

> An honest caveat **covers** a defect. It makes the report truthful and by
> doing exactly that removes the pressure that would have made somebody look at
> why a quarter of the package could not be read.

The caveat was right and worth having — without it nobody would have seen the
number at all. The mistake was treating it as the end of the work. **A caveat is
where an investigation starts.**
