# Contributing to the Augusto42 VSP distribution

Thank you for helping make VSP safer and more useful for ABAP developers.

## Scope

This distribution accepts generic fixes, portability improvements, documentation, tests,
and features that can benefit more than one SAP environment. Environment-specific behavior
must be configurable and disabled by default.

## Confidentiality boundary

All contributions must be reproducible with synthetic fixtures. Do not include:

- credentials, cookies, tokens, certificates, or secret values;
- private hostnames, IP addresses, SAP system identifiers, client numbers, or usernames;
- customer source code, transports, dumps, screenshots, traces, or production logs;
- personal, clinical, financial, or other regulated data;
- internal documents or business rules copied from a private environment.

If sensitive information is discovered, do not open a public issue. Follow
[SECURITY.md](SECURITY.md) and rotate or revoke affected credentials immediately.

## Development workflow

1. Create a branch from `main`.
2. Keep the change focused and add a regression test for bug fixes.
3. Use synthetic names such as `ZSYNTHETIC`, `$TMP`, and `sap.example.com`.
4. Run the relevant Go tests, `go vet ./...`, and formatting checks.
5. Open a pull request using the repository template.

At least one review is required for ordinary contributors. The repository owner retains an
administrative bypass for urgent maintenance and independent releases.

## Upstream relationship

Changes may be offered to the original project when they are broadly useful, but acceptance
upstream is not required for inclusion here. Upstream updates enter this repository through a
reviewable synchronization pull request; they are never merged automatically.

## License

By contributing, you agree that your contribution is provided under the repository's MIT
License. Original copyright and license notices must be preserved.
