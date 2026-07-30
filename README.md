# myyolo-cli

An unofficial, read-only CLI for locally collecting and analyzing data from
mySIGN and the classic myYOLO administration system. One authorized credential
profile can serve both systems, while their rotating-token and ASP-cookie
sessions remain isolated in the operating-system keyring. Remote reads are
small and serial; reports run from private local SQLite.

This project is not affiliated with, endorsed by, or supported by myYOLO, azh or NOVENTI. Use it only with explicit authorization and under the agreement applicable to your account.

## What it can do

| Capability | Command | Remote requests |
|---|---|---:|
| Validate and securely save both logins | `myyolo auth login --source all` | 4 |
| Check both sessions, including one autonomous relogin | `myyolo auth check` | mySIGN 1–3, admin 1–5 |
| Deliberately verify password-only autonomous relogin | `myyolo auth check --force-relogin` | mySIGN 2, admin 2–4 |
| Show whether a local profile is configured | `myyolo auth status` | 0 |
| Remove a local credential/session profile | `myyolo auth logout` | 0 |
| Pull the current mySIGN snapshot into SQLite | `myyolo sync` | 1–3 |
| Pull the admin attendance landing page | `myyolo sync admin` | 1–5 |
| Read six allowlisted admin pages | `myyolo discover admin` | 7–10 |
| Inspect all 99 typed read capabilities locally | `myyolo catalog` | 0 |
| Collect exactly one selected admin capability | `myyolo collect CAPABILITY` | 2–5 |
| Read the latest collected capability snapshot | `myyolo report capability CAPABILITY` | 0 |
| Initialize an empty local database | `myyolo db init` | 0 |
| Check database integrity and aggregate state | `myyolo db status` | 0 |
| Check Keychain and database readiness | `myyolo doctor` | 0 |
| Import a local snapshot for offline use | `myyolo import mysign` | 0 |
| Show overall collection and attendance counts | `myyolo report summary` | 0 |
| Analyze attendance by course | `myyolo report courses` | 0 |
| Analyze attendance by calendar day | `myyolo report days` | 0 |
| Analyze attendance by starting hour | `myyolo report hours` | 0 |
| Analyze individual course sessions | `myyolo report sessions` | 0 |
| Analyze participation per member | `myyolo report members` | 0 |
| Inspect collected admin route/schema metadata | `myyolo report admin-capabilities` | 0 |
| Analyze Reha time windows and duration | `myyolo report admin-reha-hours` | 0 |
| Analyze monthly course participation | `myyolo report admin-course-months` | 0 |
| Count missing Reha signatures | `myyolo report admin-missing-signatures` | 0 |

Personal, health and financial reads are isolated behind separate explicit
flags. See the complete [read catalogue](docs/read-catalog.md),
[capability map](docs/capabilities.md) and
[data dictionary](docs/data-dictionary.md).

## Safety model

- Every request must match an exact HTTPS host, method, path, and classified
  query. Unknown routes and query strings are blocked.
- One sync uses one request with a working cached session.
- An expired mySIGN session causes exactly one login and final read: three
  requests maximum.
- Admin redirects are followed manually and budgeted. Sync uses at most five
  requests; discovery uses at most ten. Admin calls are serial and at least two
  seconds apart.
- `collect` accepts exactly one named capability per run, never an `all` mode,
  and has a hard five-request ceiling including session probe and relogin.
  Parameters are type-checked before credentials or network access.
- HTTP 429, CAPTCHA, unexpected login flow, schema drift, and unclassified
  routes stop immediately. There is no polling, retry loop, browser
  fingerprint imitation, proxy rotation, or stealth behavior.
- Raw JSON/HTML responses are not retained. SQLite and WAL files use mode
  `0600`.
- Aggregate reads are default. Personal data needs
  `--include-personal-data`; Reha/prescription data additionally needs
  `--include-health-data`; bank/billing data additionally needs
  `--include-financial-data`.

## Install

Prebuilt archives and `checksums.txt` are published on the
[GitHub Releases page](https://github.com/luca-schweigmann/myyolo-cli/releases).
Choose the archive matching macOS, Linux or Windows and AMD64 or ARM64, extract
it, and place `myyolo` (`myyolo.exe` on Windows) on your `PATH`.

Release archives are checksummed but not currently code-signed or notarized.
Verify the checksum before use. Organizations requiring signed binaries should
build from the reviewed source until a signing pipeline is added.

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
# 1. Authenticate both systems once. The password is prompted securely.
myyolo auth login \
  --profile point \
  --source all \
  --partner YOUR_PARTNER_NUMBER \
  --username YOUR_USERNAME

# 2. Pull one mySIGN snapshot and selected admin statistics.
myyolo sync mysign --profile point
myyolo collect attendance-monthly --profile point
myyolo collect course-monthly --profile point

# 3. Check readiness and read reports locally without another server request.
myyolo doctor --profile point
myyolo report summary
myyolo report capability attendance-monthly
myyolo report capability course-monthly
```

## Agent-friendly installation

Authorized coding agents can install, configure, verify, and run the CLI
without source changes or passwords in process arguments. They need Go 1.26.5,
an operating-system keyring, authorized myYOLO credentials, and a private local
directory for SQLite and report exports.

```bash
git clone https://github.com/luca-schweigmann/myyolo-cli.git
cd myyolo-cli
make check
make build
./bin/myyolo version
```

The non-interactive authentication path consumes the password from standard
input:

```bash
printf '%s\n' "$AUTHORIZED_MYYOLO_PASSWORD" | ./bin/myyolo auth login \
  --profile point \
  --source all \
  --partner "$AUTHORIZED_MYYOLO_PARTNER" \
  --username "$AUTHORIZED_MYYOLO_USERNAME" \
  --password-stdin

./bin/myyolo auth check --profile point --source all
./bin/myyolo doctor --profile point
```

Agents can inspect the complete local contract, validate a command with zero
HTTP requests, perform one bounded collection, then use local JSON reports:

```bash
./bin/myyolo sync mysign --profile point
./bin/myyolo catalog --format json
./bin/myyolo collect studio-hourly-load \
  --from 2026-07-01 --to 2026-07-31 \
  --profile point --dry-run
./bin/myyolo collect studio-hourly-load \
  --from 2026-07-01 --to 2026-07-31 \
  --profile point
./bin/myyolo report summary --format json
./bin/myyolo report capability studio-hourly-load --format json
```

Agents must not expose credentials, cookies, tokens, member data, or databases;
parallelize source reads; lower the admin delay; continue after rate limits,
CAPTCHA, auth anomalies, or schema drift; or enable sensitive reads without
the corresponding explicit data-scope gates. The machine-readable catalogue is
available through `myyolo catalog --format json`; the documented scopes are in
[`docs/read-catalog.md`](docs/read-catalog.md), metric definitions in
[`docs/data-dictionary.md`](docs/data-dictionary.md), and trust boundaries in
[`docs/architecture.md`](docs/architecture.md).

## Configure one or more accounts

One profile contains one credential envelope, a separate mySIGN session, and a
separate admin cookie session. The password prompt has no terminal echo.
Passwords are never accepted as command-line arguments:

```bash
myyolo auth login \
  --profile studio-a \
  --source all \
  --partner YOUR_PARTNER_NUMBER \
  --username YOUR_USERNAME
```

Use another profile for a second authorized account. `--source mysign` or
`--source admin` validates only one system; `--source all` is the normal setup
and succeeds only when both logins succeed. For automation, `--password-stdin`
is preferred. `MYYOLO_PASSWORD` is supported as a fallback but may be visible
to privileged local processes.

```bash
printf '%s\n' "$PASSWORD" | myyolo auth login \
  --profile studio-b \
  --source all \
  --partner "$PARTNER" \
  --username "$USER" \
  --password-stdin
```

```bash
myyolo auth check --profile studio-a --source all
myyolo auth status --profile studio-a
myyolo auth logout --profile studio-a
```

Logout removes only the local keyring profile, not the remote account.

### Authentication commands

#### `myyolo auth login`

Authenticates against the selected source or both sources, then stores the
credential envelope and source-specific sessions in the operating-system
keyring.

| Flag | Required | Default | Meaning |
|---|---:|---|---|
| `--profile NAME` | no | `default` | Local name for one isolated login |
| `--source SOURCE` | no | `all` | `mysign`, `admin`, or both |
| `--partner NUMBER` | yes | — | myYOLO partner number |
| `--username USER` | yes | — | myYOLO username |
| `--password-stdin` | no | false | Read the password from standard input |

Profile names may contain letters, numbers, `.`, `_` and `-`, are limited to 64 characters and cannot start with punctuation. The CLI deliberately has no `--password` flag because process arguments may be visible to other software.

#### `myyolo auth check`

Performs the smallest remote read for the selected source. A working cached
session is reused. An expired session triggers exactly one login and one final
read; failure then stops. The repaired session is saved for the next command.

```bash
myyolo auth check --profile studio-a --source all
myyolo auth check --profile studio-a --source admin --force-relogin
```

`--force-relogin` ignores the cached session for that check, authenticates once
from the credential envelope already stored in the keyring, performs one final
read, and replaces the cache. It never asks for the password and is the
operator-facing proof that unattended session recovery is ready.

#### `myyolo auth status`

Checks only the local keyring. It does not test the remote account and makes no network request.

```bash
myyolo auth status --profile studio-a
```

The JSON response shows credential presence and both source-specific session
caches. It does not reveal usernames, passwords, cookies, or tokens.

#### `myyolo auth logout`

Deletes that profile's credentials and both cached sessions from the local
keyring. It does not call a remote logout route and does not remove the SQLite
database.

## Sync and reports

```bash
myyolo sync mysign --profile studio-a
myyolo sync admin --profile studio-a
myyolo discover admin --profile studio-a
myyolo report summary
myyolo report courses
myyolo report days
myyolo report hours
myyolo report sessions
myyolo report members --include-personal-data --format csv
myyolo report admin-capabilities
myyolo report admin-reha-hours
myyolo report admin-course-months
myyolo report admin-missing-signatures
```

Reports support table, JSON and CSV. The default database is `~/.local/share/myyolo-cli/myyolo.sqlite`; override it with `--db` or `MYYOLO_DB_PATH`. Give each profile a separate database when source accounts must remain isolated.

### mySIGN sync

```bash
myyolo sync mysign [--profile NAME] [--db PATH]
# Backward-compatible shorthand:
myyolo sync [--profile NAME] [--db PATH]
```

The command loads the selected profile, tries the cached session, fetches one snapshot, validates its graph, and imports it in one SQLite transaction.

Request sequence:

1. With a working cached session: one snapshot request.
2. Without a cached session: login, then one snapshot request.
3. With an expired cached session: failed read, exactly one login, then exactly one final read.

There is no fourth request, generic retry loop or partial database import. Authentication failure, HTTP errors, oversized responses, unknown schema and broken cross-references return a non-zero exit status.

### Admin sync and discovery

```bash
myyolo sync admin \
  [--profile NAME] [--db PATH] \
  [--delay 2s] [--request-budget 5]

myyolo discover admin \
  [--profile NAME] [--db PATH] \
  [--delay 2s] [--request-budget 10]
```

`sync admin` reads only the attendance landing page. `discover admin` reads the
six documented aggregate/list pages in
[the capability map](docs/capabilities.md). Both commands are serial. The CLI
rejects delays below two seconds and budgets above the hard limits. A valid
cached discovery needs seven requests including its session probe; the
worst-case expired-session flow needs exactly ten.

Remote HTTP 429, CAPTCHA, an unexpected redirect, login failure, request-budget
exhaustion, or schema drift ends the command immediately. Discovery imports
nothing unless all six remote reads succeeded.

### Typed read catalogue and collection

`catalog` is local-only and lists all 99 classified reads, their group,
sensitivity, HTTP method, static path and accepted filters:

```bash
myyolo catalog [--group GROUP] [--format table|json|csv]
myyolo collect CAPABILITY [filters] \
  [--profile NAME] [--db PATH] \
  [--delay 2s] [--request-budget 5] \
  [--dry-run] [--format table|json|csv]
```

The six groups are `analytics`, `courses`, `members`, `prescriptions`,
`compliance`, and `financial`. Use `catalog` instead of guessing paths or form
fields. `collect` permits one capability only; it has no bulk or wildcard
mode. Its accepted typed filters are:

| Flag | Validation |
|---|---|
| `--from`, `--to`, `--date` | `YYYY-MM-DD`, normalized for the source; ranges max. 366 days |
| `--year` | 2000–2100 |
| `--week-from`, `--week-to` | 1–53 |
| `--threshold` | 0–100000 |
| `--member-id`, `--course-id`, `--referrer-id` | Positive numeric identifier |
| `--planner`, `--population` | Capability-specific enum |
| `--search` | 1–128 characters, no control characters |
| `--ik` | Exactly nine digits |

Unneeded filters are rejected. `--dry-run` validates the complete command
without loading credentials, opening SQLite, or making HTTP requests; its
output deliberately omits supplied values. Every live collection:

1. validates the capability, parameters, data scope, delay and budget locally;
2. probes a cached session or performs exactly one bounded login;
3. fetches exactly one capability;
4. stores the structured observation under `capability:NAME`;
5. returns only import counts and a schema fingerprint, never row values.

Examples:

```bash
# Aggregate historical reports
myyolo collect attendance-monthly --profile point
myyolo collect studio-hourly-load \
  --from 2026-07-01 --to 2026-07-31 --profile point

# Person-level and Reha reads require explicit local authorization flags
myyolo collect member-checkins --member-id 123 \
  --include-personal-data --profile point
myyolo collect member-reha-history --member-id 123 \
  --include-personal-data --include-health-data --profile point

# Financial reads are isolated from normal collection
myyolo collect digital-billing-complete-archive --ik 123456789 \
  --include-personal-data --include-financial-data --profile point
```

Member IDs and filter values above are placeholders. Do not put real
credentials or health/member data into shell history, issues, logs, or public
repositories. The complete catalogue is in
[`docs/read-catalog.md`](docs/read-catalog.md).

### Database commands

```bash
myyolo db init [--db PATH]
myyolo db status [--format table|json|csv] [--db PATH]
myyolo doctor [--profile NAME] [--db PATH]
myyolo import mysign --file PATH [--db PATH]
```

`db init` creates or migrates an empty local database. `import mysign` parses a local `GetListData` JSON response and uses the same validation and transactional import path as live sync. Offline import exists for development and recovery; raw live responses should not normally be retained.

`db status` performs SQLite `quick_check` and returns schema version, source
counts, and last successful sync timestamps. `doctor` adds a local-only
keyring-readiness check; it never tests either remote session.

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
| `admin-capabilities` | Admin route/table | Route, title, headings, schema fingerprint, row count, observation time |
| `admin-reha-hours` | Reha time window | Attendees, duration and attendee-minutes |
| `admin-course-months` | Calendar month | Participants and courses with numeric values |
| `admin-missing-signatures` | Current aggregate | Member rows and exposed missing-signature quantity |
| `admin-reha-attendance` | Member row | Current Reha attendance details |
| `admin-missing-signature-members` | Member row | Current missing-signature details |
| `admin-records` | Route/table row | Generic structured values for current or historic observations |
| `capability CAPABILITY` | Latest observation for one typed capability | Generic structured row values; sensitivity-gated |

Member and admin-detail reports can expose personal data and therefore require
the explicit gate:

```bash
myyolo report members --include-personal-data
myyolo report admin-reha-attendance --include-personal-data
myyolo report admin-missing-signature-members --include-personal-data
myyolo report admin-records --route EXACT_PATH --include-personal-data
myyolo report capability member-checkins \
  --include-personal-data
myyolo report capability member-reha-history \
  --include-personal-data --include-health-data
myyolo report capability member-bank-export \
  --include-personal-data --include-financial-data
```

An unfiltered `admin-records` report can mix every sensitivity class and
therefore requires all three data-scope flags. A route classified as health or
financial requires its matching additional flag.

mySIGN reports accept `--as-of RFC3339`. This makes pending/no-show boundaries
reproducible. Admin reports always use the newest complete observation for
their route.

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
| `no_shows` | Not attended, not cancelled, and course end is strictly before report `as-of` |
| `pending` | Not attended, not cancelled, and course end is current, future, or unknown |
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

Admin discovery and typed collection retain schema/provenance metadata and
distinct structured row versions. Capability reports select only rows
belonging to the newest complete observation, so repeated collection is
idempotent and old row versions do not inflate the current view. Historical
versions remain in SQLite for local longitudinal analysis.

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
| `members` | Source namespace, member ID, member number and name |
| `course_sessions` | Source namespace, date, course, room, time window, parsed UTC end and source counts |
| `attendance` | Source namespace, member/session/prescription relationship and attendance flags |
| `prescriptions` | Source namespace, treatment, weekly-treatment and visit counters |
| `admin_capabilities` | Exact route, page/table metadata, schema fingerprint and latest observation |
| `admin_records` | Exact route/table, deterministic row hash, structured values and observation history |
| `sync_runs` | Source, normalized/schema fingerprint, timestamps, status and row counts |

Raw HTTP response bodies, passwords, cookies and rotating request tokens are not written to SQLite.

## Complete command index

```text
myyolo help
myyolo version
myyolo auth login [--profile NAME] [--source mysign|admin|all] --partner NUMBER --username USER [--password-stdin]
myyolo auth check [--profile NAME] [--source mysign|admin|all] [--force-relogin]
myyolo auth status [--profile NAME]
myyolo auth logout [--profile NAME]
myyolo sync [mysign] [--profile NAME] [--db PATH]
myyolo sync admin [--profile NAME] [--db PATH] [--delay DURATION] [--request-budget 1..5]
myyolo discover admin [--profile NAME] [--db PATH] [--delay DURATION] [--request-budget 1..10]
myyolo catalog [--group GROUP] [--format table|json|csv]
myyolo collect CAPABILITY [typed filters] [--profile NAME] [--db PATH] [--delay DURATION] [--request-budget 1..5] [--dry-run] [data-scope flags]
myyolo db init [--db PATH]
myyolo db status [--format table|json|csv] [--db PATH]
myyolo doctor [--profile NAME] [--db PATH]
myyolo import mysign --file PATH [--db PATH]
myyolo report summary|courses|days|hours|sessions [--as-of RFC3339] [--format table|json|csv] [--db PATH]
myyolo report members --include-personal-data [--as-of RFC3339] [--format table|json|csv] [--db PATH]
myyolo report admin-capabilities|admin-reha-hours|admin-course-months|admin-missing-signatures [--format table|json|csv] [--db PATH]
myyolo report capability CAPABILITY [data-scope flags] [--format table|json|csv] [--db PATH]
myyolo report admin-reha-attendance|admin-missing-signature-members|admin-records --include-personal-data [data-scope flags] [--route EXACT_PATH] [--format table|json|csv] [--db PATH]
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

A normal check, sync, or discovery handles this once automatically. If the
final read still reports an expired session, the command stops. Re-run
`auth login` deliberately rather than looping the command.

### Rate limit or CAPTCHA

The CLI stops without retrying. Do not lower the delay, rotate identities, or
automate around the block. Wait for the operator/provider-approved window and
retry deliberately.

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

The tests cover strict JSON and HTML parsing, migration/idempotency,
latest-snapshot selection, typed capability filters, dry-run redaction,
personal/health/financial gates, fixed-clock no-show boundaries, file
permissions, output formats, separate keyring sessions, exact route allowlists,
delays and budgets, rate-limit/CAPTCHA stops, response-size and content-type
limits, schema drift, cached sessions and exact one-time re-login sequences.

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

cli-printing-press generate \
  --spec ./printing-press/myyolo-admin-pp-spec.yaml \
  --spec-source browser-sniffed \
  --transport standard \
  --dry-run
```

The generated client is reference-only. The production runtime stays
hand-written because its exact allowlist, keyring, request-budget and re-login
invariants are stricter than the generated transport.

## Scope

This version has no Magicline integration, scheduler, server, cloud upload,
hosted dashboard, MCP or myYOLO write command. A local dashboard is a separate
step and should read only SQLite. Identical names are not a safe cross-system
key: names can change and collide. A future Magicline integration should use a
stable member number or source identifier and needs a separate privacy review.

See [architecture](docs/architecture.md), [security policy](SECURITY.md) and the [Printing Press contract](printing-press/contract.md).
