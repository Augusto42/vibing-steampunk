# ABAP object support

This document describes the operation-level contract of the unified
`GetSource` and `WriteSource` APIs. A type is listed as supported only when VSP
has a concrete backend path and automated tests for the relevant behavior.

## Capability matrix

| Object type | Read | Create | Update | Notes |
|-------------|:----:|:------:|:------:|-------|
| `PROG` | Yes | Yes | Yes | Executable programs |
| `INCL` | Yes | Yes | Yes | First-class `PROG/I`; syntax check runs before the stateful write sequence |
| `CLAS` | Yes | Yes | Yes | Full source, includes, and method-level operations |
| `INTF` | Yes | Yes | Yes | Interface source |
| `FUNC` | Yes | No | No | `parent` must identify the function group; specialized function tools may provide additional operations |
| `FUGR` | Metadata | No | No | Returns function-group metadata through `GetSource` |
| `DDLS` | Yes | Yes | Yes | CDS DDL source |
| `VIEW` | Yes | No | No | Classic DDIC view metadata |
| `BDEF` | Yes | Yes | Yes | RAP behavior definition |
| `SRVD` | Yes | Yes | Yes | RAP service definition |
| `SRVB` | Metadata | Yes | Yes | JSON configuration rather than ABAP source |
| `MSAG` | Metadata | No | No | Message-class metadata; specialized message tools are separate |
| `ENHO` | Yes | No | `XH` | Existing classic source-code plug-ins can be updated through ZADT_VSP; other subtypes remain read-only |
| `DYNP` | Experimental | No | No | Read-only screen metadata, layout, and flow logic through ZADT_VSP and `RPY_DYNPRO_READ`; validate against your non-production SAP release |
| `ENHC` / `ENHS` | No | No | No | Explicitly unsupported; VSP does not pretend a safe mutation path exists |

`No` means the unified operation fails explicitly. It must not be interpreted
as permission to substitute a different object type or silently create a
standalone program.

## Enhancement implementations (`ENHO`)

VSP resolves an enhancement implementation by name and attempts these paths:

1. the enhancement source URI returned by ADT search;
2. the alternate plural-form ADT source endpoint used by some releases;
3. the optional ZADT_VSP bridge for classic systems where REST does not expose
   the implementation body.

Includes can also be read as an annotated, merged view. This result is for
analysis only and must not be written back to SAP.

```text
GetSource(object_type="ENHO", name="ZENH_SAMPLE")
GetSource(object_type="INCL", name="ZPROGRAM_SAMPLE_F01", merged=true, include_context=false)
WriteSource(object_type="ENHO", name="ZENH_SAMPLE", source="...", mode="update")
```

If no source path is available, VSP returns a structured error with navigation
metadata. It does not return empty source as a successful read.

Updating an existing `ENHO/XH` requires the current ZADT_VSP bridge. The bridge
uses SAP's Enhancement Framework API in an isolated worker so SAP owns locking,
saving, activation, and transport assignment. VSP reports success only after
the active generated include has been read back and matches the requested
source. Creation remains unavailable because source text alone does not define
the host object, enhancement anchor, subtype, or enhancement spot.

## Program includes (`INCL`)

Program includes are created and updated as `PROG/I` objects. The update
workflow is:

```text
SyntaxCheck -> Lock -> UpdateSource -> Unlock -> Activate
```

The syntax check uses `/programs/includes/<name>/source/main`. Only OO class
include URLs omit `/source/main`; treating every URL containing `/includes/`
as a class include is incorrect and is covered by regression tests.

```text
WriteSource(
  object_type="INCL",
  name="ZPROGRAM_SAMPLE_F01",
  source="INCLUDE zprogram_sample_f01.",
  mode="upsert",
  description="Sample include",
  package="$TMP"
)
```

For transportable packages, configure VSP's mutation policy and pass an
allowed transport exactly as required for the other writable object types.
VSP resolves package metadata before mutation and fails closed when a package
records changes but no explicit transport was supplied. It also aborts when
SAP locks the object in a request different from the caller's request.

## Screens (`DYNP`)

Dynpro reads require the optional ZADT_VSP WebSocket bridge. The native read
contract uses `RPY_DYNPRO_READ` and returns:

- header metadata;
- screen containers;
- field-to-container assignments;
- flow logic as ordered source lines.

Either call form is accepted:

```text
GetSource(object_type="DYNP", name="0100", parent="ZPROGRAM_SAMPLE")
GetSource(object_type="DYNP", name="ZPROGRAM_SAMPLE/0100")
```

Screen numbers are normalized to four digits. Invalid references, unavailable
bridges, unexpected RFC response shapes, and nonzero RFC return codes all fail
closed.

Dynpro create, update, and delete are intentionally unavailable in this
release. Those operations depend on classic RFC mutation APIs and must first be
validated end to end against a dedicated non-production SAP system, including
locking, transport ownership, syntax checks, activation, and rollback behavior.

## Next candidates

The next high-value expansion areas are:

1. sandbox-validated Dynpro mutation workflows;
2. Enhancement Framework mutation support and explicit `ENHC` / `ENHS`
   contracts;
3. first-class domain and data-element creation;
4. GUI status and other classic program assets.

Every new mutation path should preserve the same safety gates, transport
handling, synthetic test policy, and fail-closed behavior used by current VSP
workflows.

For a runnable ADT/ZADT_VSP protocol simulator and the real-system validation
ladder, see [Testing VSP without customer SAP data](mock-sap-testing.md).
