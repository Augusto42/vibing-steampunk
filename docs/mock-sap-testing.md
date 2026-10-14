# Synthetic SAP protocol simulator

`cmd/vsp-mock-sap` serves a small, deterministic ADT and ZADT_VSP WebSocket
surface for development tests. It is **not** an SAP runtime and cannot prove
that ABAP code compiles, activates, or behaves on any SAP release. All object
names, credentials, and source in `internal/mocksap` are invented test fixtures.

Run the Go integration test:

```sh
go test ./internal/mocksap -count=1
```

On Windows, the end-to-end CLI smoke script builds both binaries and launches
the simulator on loopback only. Use a work root with adequate free space:

```powershell
pwsh -NoProfile -File scripts/test-object-expansion-mock.ps1 -WorkRoot 'D:\' -Port 50080
```

The script creates a temporary, uniquely named directory, uses only synthetic
configuration, and removes that directory when it finishes. It checks ENHO
read, read-only Dynpro through WebSocket, and INCL write/read-back. The unit
suite separately covers rejected writes and bridge error handling.

Do not point the simulator script at a real SAP host or use it to infer that
transportable-package writes work. That requires the dedicated non-production
SAP validation described in [the integration matrix](upstream-integration-matrix-2026-09-25.md).
