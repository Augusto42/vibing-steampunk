# Security policy

## Supported versions

Security fixes target the latest Augusto42 community release and the current `main` branch.
Older releases may receive fixes when practical, but are not guaranteed maintenance.

## Reporting a vulnerability

Use GitHub private vulnerability reporting for security issues. Do not disclose a vulnerability
in a public issue, discussion, pull request, commit, or log.

Report the smallest useful reproduction and use only synthetic data. Never attach credentials,
private SAP endpoints, customer source, production traces, personal data, or confidential
documents. If a credential may have been exposed, revoke or rotate it before reporting.

The maintainer will triage reports on a best-effort basis, coordinate a remediation, and publish
only sanitized technical details.

## Operational safety

VSP can perform mutating SAP ADT operations. Keep safety policies enabled, restrict allowed
packages and transports, use least-privilege technical users, and validate changes in an
authorized disposable development environment before broader use.
