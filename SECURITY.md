# Security policy

## Supported versions

Security fixes are provided for the latest tagged release.

## Reporting a vulnerability

Do not open a public issue containing a vulnerability, credential, token, cookie, database, HAR file, member identity or attendance record. Use GitHub's private security-advisory flow for this repository.

Include a minimal synthetic reproducer, affected version and expected impact. Replace all account and person data with invented values.

## Operator responsibilities

- Use only an account and data you are authorized to access.
- Run the CLI on a trusted, patched host with full-disk encryption.
- Restrict and encrypt backups of the local SQLite database.
- Keep exported CSV/JSON reports out of shared folders and source control.
- Remove a local profile with `myyolo auth logout --profile NAME` when access ends. This removes the CLI's keyring entries, not the remote account.

The CLI intentionally has no telemetry, update beacon, cloud storage or remote write route.
