# myyolo-cli

An unofficial, read-only CLI for locally collecting and analyzing mySIGN data from myYOLO. It authenticates with an account you are authorized to use, stores credentials and its cached session in the operating-system keyring, imports the rolling snapshot into private SQLite, and reports from local history.

This project is not affiliated with, endorsed by, or supported by myYOLO, azh or NOVENTI. Use it only with explicit authorization and under the agreement applicable to your account.

## What it can do

| Capability | Command | Remote requests |
|---|---|---:|
| Validate and securely save a login | `myyolo auth login` | 1 |
| Show whether a local profile is configured | `myyolo auth status` | 0 |
| Remove a local credential/session profile | `myyolo auth logout` | 0 |
| Pull the current mySIGN snapshot into SQLite | `myyolo sync` | 1–3 |
| Initialize an empty local database | `myyolo db init` | 0 |
| Import a local snapshot for offline use | `myyolo import mysign` | 0 |
| Show overall collection and attendance counts | `myyolo report summary` | 0 |
| Analyze attendance by course | `myyolo report courses` | 0 |
| Analyze attendance by calendar day | `myyolo report days` | 0 |
| Analyze attendance by starting hour | `myyolo report hours` | 0 |
| Analyze individual course sessions | `myyolo report sessions` | 0 |
| Analyze participation per member | `myyolo report members` | 0 |

The snapshot currently includes course sessions, course/member attendance
relationships, short member records and prescription summaries. The CLI keeps
source identifiers intact and builds history over repeated syncs.

## Safety model

- Only `POST /Home/LoginUser` and the semantically read-only `POST /Home/GetListData` are allowed; all other routes are blocked.
- One sync uses one request with a working cached session.
- An expired session causes exactly one login and one retry: three requests max.
- Requests are serial with a fixed one-second gap. There is no polling, browser fingerprint imitation, proxy rotation or CAPTCHA bypass.
- Raw responses are not retained. SQLite and WAL files use mode `0600`.
- Aggregate reports are default. Member output needs `--include-personal-data`.

## Install

With Go 1.26.5 or newer:

```bash
go install github.com/luca-schweigmann/myyolo-cli/cmd/myyolo@latest
```

From a checkout:

```bash
make check
make build
```

The binary is written to `bin/myyolo`.

## Quick start

```bash
# 1. Authenticate once. The password is prompted securely.
myyolo auth login \
  --profile default \
  --partner YOUR_PARTNER_NUMBER \
  --username YOUR_USERNAME

# 2. Pull one snapshot into the default local database.
myyolo sync --profile default

# 3. Read reports locally without another server request.
myyolo report summary
myyolo report courses
myyolo report days
```

## Configure one or more accounts

The password prompt has no terminal echo. Passwords are never accepted as command-line arguments:

```bash
myyolo auth login \
  --profile studio-a \
  --partner YOUR_PARTNER_NUMBER \
  --username YOUR_USERNAME
```

Use another profile for the second authorized login. For automation, `--password-stdin` is preferred. `MYYOLO_PASSWORD` is supported as a fallback but may be visible to privileged local processes.

```bash
printf '%s\n' "$PASSWORD" | myyolo auth login \
  --profile studio-b \
  --partner "$PARTNER" \
  --username "$USER" \
  --password-stdin
```

```bash
myyolo auth status --profile studio-a
myyolo auth logout --profile studio-a
```

Logout removes only the local keyring profile, not the remote account.

### Authentication commands

#### `myyolo auth login`

Authenticates against mySIGN, then stores the credential envelope and returned session in the operating-system keyring.

| Flag | Required | Default | Meaning |
|---|---:|---|---|
| `--profile NAME` | no | `default` | Local name for one isolated login |
| `--partner NUMBER` | yes | — | myYOLO partner number |
| `--username USER` | yes | — | myYOLO username |
| `--password-stdin` | no | false | Read the password from standard input |

Profile names may contain letters, numbers, `.`, `_` and `-`, are limited to 64 characters and cannot start with punctuation. The CLI deliberately has no `--password` flag because process arguments may be visible to other software.

#### `myyolo auth status`

Checks only the local keyring. It does not test the remote account and makes no network request.

```bash
myyolo auth status --profile studio-a
```

The JSON response contains `configured` and `session_cached`.

#### `myyolo auth logout`

Deletes that profile's credentials and cached session from the local keyring. It does not call a remote logout route and does not remove the SQLite database.

## Sync and reports

```bash
myyolo sync --profile studio-a
myyolo report summary
myyolo report courses
myyolo report days
myyolo report hours
myyolo report sessions
myyolo report members --include-personal-data --format csv
```

Reports support table, JSON and CSV. The default database is `~/.local/share/myyolo-cli/myyolo.sqlite`; override it with `--db` or `MYYOLO_DB_PATH`. Give each profile a separate database when source accounts must remain isolated.

### `myyolo sync`

```bash
myyolo sync [--profile NAME] [--db PATH]
```

The command loads the selected profile, tries the cached session, fetches one snapshot, validates its graph, and imports it in one SQLite transaction.

Request sequence:

1. With a working cached session: one snapshot request.
2. Without a cached session: login, then one snapshot request.
3. With an expired cached session: failed read, exactly one login, then exactly one final read.

There is no fourth request, generic retry loop or partial database import. Authentication failure, HTTP errors, oversized responses, unknown schema and broken cross-references return a non-zero exit status.

### Database commands

```bash
myyolo db init [--db PATH]
myyolo import mysign --file PATH [--db PATH]
```

`db init` creates or migrates an empty local database. `import mysign` parses a local `GetListData` JSON response and uses the same validation and transactional import path as live sync. Offline import exists for development and recovery; raw live responses should not normally be retained.

### Report command reference

All reports read SQLite only:

```bash
myyolo report REPORT [--db PATH] [--format table|json|csv]
```

| Report | Grouping | Fields |
|---|---|---|
| `summary` | Entire database | Members, sessions, attendance rows, attended, signed, cancelled, missing signatures, no-shows, distinct participants, last completed sync |
| `courses` | Course description | Sessions, bookings and every attendance metric |
| `days` | Calendar day | Sessions, bookings and every attendance metric |
| `hours` | Course starting hour | Sessions, bookings and every attendance metric |
| `sessions` | Course occurrence | Date, time, course, room and every attendance metric |
| `members` | Member | Source ID, member number, name and every attendance metric |

The member report can expose personal data and therefore requires the explicit gate:

```bash
myyolo report members --include-personal-data
```

Output formats:

- `table` is the human-readable default.
- `json` is suitable for scripts and middleware.
- `csv` is suitable for local spreadsheet analysis.

Redirect output as usual, but remember that member-level files contain personal data:

```bash
myyolo report courses --format csv > course-report.csv
myyolo report summary --format json > summary.json
```

## Metric definitions

The reports use the stored mySIGN flags; they do not infer physical presence from a course time window.

| Metric | Definition |
|---|---|
| `bookings` | Stored attendance/course-member relationship rows |
| `attended` | `Teilgenommen = true` |
| `signed` | `HatUnterschrift = true` |
| `cancelled` | `Storniert = true` |
| `missing_signatures` | Attended, not signed and not cancelled |
| `no_shows` | Not attended and not cancelled |
| `distinct_participants` | Unique myYOLO member IDs represented in attendance rows |

These are technical definitions. Validate them against the operational meaning used by your organization before treating them as billing, compliance or management KPIs.

## Local history and idempotency

mySIGN returns a rolling snapshot rather than a complete historical export. Each sync upserts records by their stable source IDs:

- existing records are updated;
- repeated imports do not duplicate records;
- older rows remain in SQLite when they leave a later remote snapshot;
- every attempted import receives a `sync_runs` audit row;
- failed imports roll back their data transaction and are marked failed.

This means history becomes more useful over time while report commands remain remote-request-free. It does not reconstruct periods that were never captured.

## Multiple logins and databases

Credential profiles are isolated in the operating-system keyring. Database selection is independent, so choose an explicit database per account when datasets must not mix:

```bash
myyolo sync --profile studio-a --db ~/.local/share/myyolo-cli/studio-a.sqlite
myyolo sync --profile studio-b --db ~/.local/share/myyolo-cli/studio-b.sqlite

myyolo report summary --db ~/.local/share/myyolo-cli/studio-a.sqlite
```

Do not point unrelated profiles at the same database unless combining those records is explicitly intended and authorized.

## Data model

| Table | Contents |
|---|---|
| `members` | Source member ID, member number, first and last name |
| `course_sessions` | Date, course, room, time window and source counts |
| `attendance` | Member/session/prescription relationship and attendance flags |
| `prescriptions` | Treatment, weekly-treatment and visit counters |
| `sync_runs` | Source, normalized payload hash, timestamps, status and row counts |

Raw HTTP response bodies, passwords, cookies and rotating request tokens are not written to SQLite.

## Complete command index

```text
myyolo help
myyolo version
myyolo auth login [--profile NAME] --partner NUMBER --username USER [--password-stdin]
myyolo auth status [--profile NAME]
myyolo auth logout [--profile NAME]
myyolo sync [--profile NAME] [--db PATH]
myyolo db init [--db PATH]
myyolo import mysign --file PATH [--db PATH]
myyolo report summary [--format table|json|csv] [--db PATH]
myyolo report courses [--format table|json|csv] [--db PATH]
myyolo report days [--format table|json|csv] [--db PATH]
myyolo report hours [--format table|json|csv] [--db PATH]
myyolo report sessions [--format table|json|csv] [--db PATH]
myyolo report members --include-personal-data [--format table|json|csv] [--db PATH]
```

## Platform behavior

- macOS uses Keychain.
- Windows uses Credential Manager.
- Linux uses Secret Service through the desktop keyring/D-Bus session.
- SQLite files use mode `0600` on POSIX systems. On Windows, protect the user profile and database directory with appropriate account ACLs.
- The CLI has no telemetry, cloud storage, background service or update beacon.

## Troubleshooting

### `profile "NAME" is not configured`

Run `myyolo auth login --profile NAME ...` first and verify with `auth status`.

### Keyring errors on Linux

Ensure a Secret Service provider such as GNOME Keyring or KWallet and a D-Bus user session are available. Headless servers often do not provide one by default.

### Authentication failed

Check partner number, username and password. The CLI does not keep rejected credentials and never retries invalid login in a loop.

### Session expired

A normal sync handles this once automatically. If the final read still reports an expired session, the command stops. Re-run `auth login` deliberately rather than looping sync.

### Response schema changed

The parser fails closed so changed or incomplete attendance fields cannot silently corrupt reports. Open an issue with a fully synthetic reproducer—never attach the live response.

### Database is locked

Do not run overlapping sync processes against the same database. Wait for the other process to finish or use separate database paths.

### Where is my data?

The default path is `~/.local/share/myyolo-cli/myyolo.sqlite`. Run commands with an explicit `--db` path when portability matters.

## Offline development

```bash
myyolo import mysign \
  --file ./internal/mysign/testdata/get_list_data.synthetic.json \
  --db ./data/synthetic.sqlite
myyolo report summary --db ./data/synthetic.sqlite
```

Only synthetic fixtures are committed. Never attach credentials, tokens, screenshots, HAR files, databases or member exports to an issue.

## Development and release checks

```bash
make format  # format Go source
make test    # race-enabled test suite
make check   # format, module, vet, race, and build gates
make build   # local binary in bin/myyolo
```

The tests cover strict snapshot parsing, idempotent imports, failed-import
auditing, file permissions, every report, secret-store behavior, write-route
denial, response-size limits, password redaction, schema drift, cached sessions,
the exact re-login request sequence and the three-request ceiling.

CI runs the same quality gates plus `govulncheck`. Tagged `v*` pushes use
GoReleaser to build checksummed archives for macOS, Linux and Windows on AMD64
and ARM64.

The checked-in Printing Press contract can be parsed without credentials or
live traffic:

```bash
cli-printing-press generate \
  --spec ./printing-press/myyolo-pp-spec.yaml \
  --spec-source browser-sniffed \
  --transport standard \
  --dry-run
```

The generated client is reference-only. The production runtime stays
hand-written because its exact allowlist, keyring, request-budget and re-login
invariants are stricter than the generated transport.

## Scope

Version 1 has no Magicline integration, scheduler, server, cloud upload, MCP or myYOLO write command. Identical names are not a safe cross-system key: names can change and collide. A future integration should use a stable member number or source identifier and needs a separate privacy review.

See [architecture](docs/architecture.md), [security policy](SECURITY.md) and the [Printing Press contract](printing-press/contract.md).
