# Community fork: enhancement and Dynpro integration

This page describes the **integration branch**, not a released binary. It
combines the current upstream baseline with capabilities developed in the
community fork. Do not deploy it to a productive SAP system until the release
gates in [the integration matrix](upstream-integration-matrix-2026-09-25.md)
have been met. All examples below use invented object names.

## What is available in source

| Capability | Entry point | Important limit |
| --- | --- | --- |
| Dynpro read | `GetSource(object_type="DYNP", name="ZSYNTHETIC_PROGRAM/0100")` or `Client.GetDynpro` | Read-only, requires the optional ZADT_VSP bridge and `RPY_DYNPRO_READ` authorization. No screen creation or update. |
| ENHO read | `GetSource(object_type="ENHO", name="ZSYNTHETIC_ENHO")` | XH source uses ADT or bridge read-back; class/BAdI implementations return metadata when there is no single source include. |
| ENHO/XH update | `WriteSource` with `object_type="ENHO"`, `mode="update"` | Existing XH only. Explicit package/transport policy, CTS owner check, and active include read-back. No source-only creation or upsert. |
| ENHO/XH create | `CreateEnhancement(kind="XH", ...)` or `vsp enhancement create xh ...` | Requires explicit host, full anchor, package, and description. |
| Class enhancement create | `CreateEnhancement(kind="CLASS", ...)` | Requires the enhanced class; optional new method. |
| BAdI implementation create | `CreateEnhancement(kind="BADI", ...)` | Requires spot, definition, implementation name, and an **existing** implementation class. |

Creation and classic ENHO writes need the matching `ZCL_VSP_RFC_SERVICE`
installed and activated on a dedicated **development/test** SAP system. The
ABAP source is identical (apart from line endings) in
`embedded/abap/zcl_vsp_rfc_service.clas.abap` and
`src/zcl_vsp_rfc_service.clas.abap`, so the embedded installer and abapGit
package carry the same bridge implementation. The client reports success only
after reading back the expected active object/source; a scheduled background
worker by itself is **not** success.

## Synthetic CLI examples

Run `vsp enhancement create --help` for all metadata fields. Never put a real
password, source from a customer system, or transport log in an issue or PR.
Use your normal local connection configuration; keep credentials outside the
repository.

```powershell
# XH body comes from stdin. FULL_NAME must be obtained from the target host
# in the test system; this invented anchor is only a syntax illustration.
Get-Content .\synthetic-hook.abap -Raw | vsp enhancement create xh ZSYNTHETIC_ENHO `
  --host ZSYNTHETIC_PROGRAM --anchor '\PR:ZSYNTHETIC_PROGRAM\SE:END\EI' `
  --package '$TMP' --description 'Synthetic test enhancement'

vsp enhancement create class ZSYNTHETIC_CLASS_ENHO `
  --class ZCL_SYNTHETIC_HOST --method ZSYNTHETIC_METHOD `
  --package '$TMP' --description 'Synthetic class enhancement'

vsp enhancement create badi ZSYNTHETIC_BADI_ENHO `
  --spot ZSYNTHETIC_SPOT --badi ZBADI_SYNTHETIC `
  --implementation ZIM_SYNTHETIC --implementation-class ZCL_IM_SYNTHETIC `
  --package '$TMP' --description 'Synthetic BAdI implementation'
```

For a transportable package, add an explicit `--transport` and enable the
transportable-edit safety policy. The package and request must belong to the
same test system. The client blocks mutation if package metadata cannot be
verified, if a required transport is absent, or if an existing ENHO's CTS
owner disagrees with the requested transport.

## Validation still required before a release

The Go unit tests and mock responses establish routing, validation and
fail-closed behavior. They **do not** establish that the ABAP bridge compiles
or that Enhancement Framework APIs work on every SAP release. On a dedicated
non-production system, test each create subtype and XH update, `$TMP` and a
transportable package, activation, read-back, rollback after failure, and the
actual bridge deployment. Record the SAP release and any unsupported subtype.
Do not use any real customer objects or data for this validation.

The `vsp update` command on this branch targets the community fork, not the
upstream repository. GitHub's `releases/latest` endpoint excludes pre-releases;
for a community pre-release, use `vsp update --version <tag>` after reviewing
its checksums and release notes. No new community release has been published
for this integration branch.
