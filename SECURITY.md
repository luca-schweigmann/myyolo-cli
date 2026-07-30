# Security policy

## Supported versions

Security fixes are provided for the latest tagged release.

## Reporting a vulnerability

Do not open a public issue containing a vulnerability, credential, token, cookie, database, HAR file, member identity or attendance record. Use GitHub's private security-advisory flow for this repository.

Include a minimal synthetic reproducer, affected version and expected impact. Replace all account and person data with invented values.

## Operator responsibilities

- Use only an account and data you are authorized to access.
- Run the CLI on a trusted, patched host with full-disk encryption.
- Treat the local database as health-adjacent personal data even when only
  aggregate reports are normally displayed.
- Restrict and encrypt backups of the local SQLite database.
- Keep exported CSV/JSON reports out of shared folders and source control.
- Do not weaken the two-second admin delay or increase the compiled request
  ceilings. Stop after rate limits, CAPTCHA, auth anomalies, or schema drift.
- Remove a local profile with `myyolo auth logout --profile NAME` when access
  ends. This removes credentials plus the separate mySIGN/admin sessions from
  the keyring, not the remote account.

## Local-data retention and deletion

The default database is
`~/.local/share/myyolo-cli/myyolo.sqlite`. `MYYOLO_DB_PATH` or `--db` can select
another path. Reports may create additional files wherever shell output is
redirected.

Before deleting a database, stop all CLI processes using it. Delete the exact
database file and, when present beside it, its `-wal` and `-shm` companions.
Delete exported CSV/JSON files and protected backups separately. This action is
local and irreversible; it does not delete any source-system record.

Choose a documented retention period suitable for the organization's
authorization and legal basis. This project does not provide a background
scheduler or automatic retention job.

The CLI intentionally has no telemetry, update beacon, cloud storage,
background service, remote logout, or remote write route.
