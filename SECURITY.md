# Security policy

velocity.report runs on a Raspberry Pi beside a street. By design it records no images, licence
plates or personal data, but the device, its database and its admin surfaces still need
protecting.

## Reporting a vulnerability

Report it privately through GitHub's private vulnerability reporting on this repository: the
**Security** tab, then **Report a vulnerability**. Do not open a public issue, pull request or
discussion about it.

Include what you found, the version (`velocity version`), how to reproduce it, and what an
attacker would need: network position, credentials or physical access. A maintainer replies in the
private report, and the fix is discussed there until it ships.

## Supported versions

Fixes land on `main` and ship in the next release. Only the latest release receives them.

## Scope

In scope: the `velocity` binary and its HTTP, gRPC and debug surfaces, the web app, the Raspberry
Pi image and its installer, and the macOS visualiser. Reports that need physical access to the
device are out of scope.

## Identifiers

Findings are tracked with internal identifiers of the form `VR-SEC-NNN`, recorded in the
[security checklist's register](.github/knowledge/security-checklist.md#identifier-register). They
are not CVE identifiers. A CVE is requested only through a GitHub security advisory, when one is
warranted, and a live weakness is listed only once its fix has shipped.
