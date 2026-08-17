# Testing VSP without customer SAP data

VSP includes a local SAP protocol simulator for repeatable integration tests.
It implements only the ADT and ZADT_VSP routes required by the object-type
expansion. All identities, source code, credentials, and responses are
synthetic.

The simulator is not an ABAP compiler, database, application server, or
substitute for release-specific SAP validation. It proves the VSP client
contract and network workflow; a real non-production SAP system must still
prove the backend behavior.

## Automated end-to-end test

On Windows PowerShell:

```powershell
./scripts/test-object-expansion-mock.ps1
```

The script builds both binaries in an isolated temporary directory, starts the
simulator on localhost, and exercises the real VSP CLI:

1. reads `ENHO` source through ADT search and the enhancement source endpoint;
2. reads `DYNP` metadata and flow logic through the ZADT_VSP WebSocket and
   `RPY_DYNPRO_READ` contract;
3. updates an existing `INCL` through syntax check, stateful lock, PUT, unlock,
   and activation;
4. reads the include back and verifies the change;
5. stops the server and removes the temporary files.

The Go integration suite also checks merged include rendering and fail-closed
behavior for syntax errors and nonzero Dynpro RFC return codes:

```powershell
go test ./internal/mocksap -count=1
```

## Running the simulator manually

```powershell
go run ./cmd/vsp-mock-sap -listen 127.0.0.1:50080
```

Synthetic connection parameters:

| Setting | Value |
|---|---|
| URL | `http://127.0.0.1:50080` |
| Client | `001` |
| User | `SYNTHETIC_USER` |
| Password | `synthetic-password` |
| Package | `$TMP` |
| ENHO | `ZSYNTHETIC_ENHO` |
| INCL | `ZSYNTHETIC_INCLUDE` |
| DYNP | `ZSYNTHETIC_APP/0100` |

Useful deterministic failure modes:

```powershell
go run ./cmd/vsp-mock-sap -syntax-error
go run ./cmd/vsp-mock-sap -activation-error
go run ./cmd/vsp-mock-sap -dynpro-subrc 4
```

The server binds to localhost by default, requires synthetic Basic Auth for
all SAP-shaped routes, enforces CSRF on modifications, and never logs
Authorization headers or request bodies.

## Official SAP environments evaluated

Status checked on 15 August 2026:

| Option | Suitability for VSP | Current limitation |
|---|---|---|
| SAP BTP ABAP Environment trial | Good for modern ADT/RAP checks | ABAP Cloud does not support classic Dynpro/SAP GUI development and is not suitable for the full ENHO/DYNP test |
| SAP Learning Hub practice systems | Potentially useful for ordinary ADT exercises | Requires a valid subscription; permissions and custom APC installation depend on the assigned course system |
| ABAP Cloud Developer Trial Docker image | Best local architecture for classic tests | SAP announced that all official Docker versions have been unavailable since February 2026 while a 2025 replacement is prepared |
| SAP ABAP Platform 2023 Developer Edition on CAL | Best currently available full-system candidate | SAP software is offered for test/development, but the AWS/Azure/GCP infrastructure is paid and requires an account |
| SAP S/4HANA Fully-Activated Appliance on CAL | Full classic system with administrative access | Large paid infrastructure; trial terms and cost make it excessive for the first VSP validation |

Recommended validation ladder:

1. run the local simulator and complete repository tests;
2. use SAP BTP trial only for modern ADT-compatible object types;
3. provision the smaller 64 GB / 10-core ABAP Platform 2023 Developer Edition
   in SAP Cloud Appliance Library for a short, controlled classic-system test;
4. install only synthetic VSP objects in a local package and delete or suspend
   the appliance after the test window.

Official references:

- [SAP BTP ABAP Environment trial onboarding](https://developers.sap.com/tutorials/abap-environment-trial-onboarding.html)
- [SAP Help: differences between ABAP Cloud and classic ABAP](https://help.sap.com/docs/abap-cloud/developer-guide-from-classic-abap-to-abap-cloud/main-differences-between-classic-abap-and-abap-cloud)
- [SAP Learning practice systems](https://learning.sap.com/practice-systems)
- [SAP announcement: ABAP Cloud Developer Trial 2023](https://community.sap.com/t5/technology-blog-posts-by-sap/abap-cloud-developer-trial-2023-available-now/ba-p/14057183)
- [Official trial image requirements and setup](https://github.com/SAP-docs/abap-platform-trial-image)
- [SAP ABAP Platform 2023 Developer Edition on CAL](https://community.sap.com/t5/technology-blog-posts-by-sap/abap-platform-2023-developer-edition-on-cloud-appliance-library-cal-now/ba-p/14185861)
- [SAP Cloud Appliance Library infrastructure costs](https://help.sap.com/docs/SAP_CLOUD_APPLIANCE_LIBRARY/43df7ec18b5241f7bf9a8c9de5ba3361/bbd5aa62b7b44ce98bd512ee6638feee.html)
