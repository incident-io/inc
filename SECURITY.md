# Security Policy

## Supported Versions

`inc` follows a rolling release model: security fixes are shipped in a new release rather than backported. Only the [latest release](https://github.com/incident-io/inc/releases/latest) is supported — please update to the newest version before reporting an issue, in case it has already been fixed.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, use one of these private channels:

- **GitHub private vulnerability reporting** — [open a draft security advisory](https://github.com/incident-io/inc/security/advisories/new) on this repository.
- **Email** — [security@incident.io](mailto:security@incident.io).

Our full disclosure policy is published at [incident.io/legal/vulnerability-disclosure](https://incident.io/legal/vulnerability-disclosure), and our security contact details at [incident.io/.well-known/security.txt](https://incident.io/.well-known/security.txt).

### What to include

To help us triage and fix the issue quickly, please include where possible:

- The type of issue (e.g. credential exposure, command injection, insecure file permissions)
- The version of `inc` affected (`inc --version`) and your OS/platform
- Step-by-step instructions or a proof of concept to reproduce the issue
- The impact you believe the issue has, and how an attacker might exploit it

### What to expect

- We will acknowledge your report within **2 business days**.
- We will keep you informed as we investigate, and let you know when a fix is released.
- Please give us a reasonable opportunity to remediate before any public disclosure — we're happy to coordinate timing with you.
- We do not operate a paid bug bounty, but we're glad to credit reporters in release notes if you'd like.

## Scope

This policy covers the `inc` CLI in this repository, including how it stores and transmits API keys. Vulnerabilities in the incident.io product or API itself are also welcome via the channels above and are covered by the [vulnerability disclosure policy](https://incident.io/legal/vulnerability-disclosure).
