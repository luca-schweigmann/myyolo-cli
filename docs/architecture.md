# Architecture

The CLI turns one bounded, authorized snapshot request into durable local history and runs all analytics against that history.

```text
mySIGN
  | exact allowlist, serial requests, request budget
  v
authenticated transport -- OS keyring (credentials + cached session)
  | parsed snapshot
  v
transactional collector
  | normalized rows
  v
private SQLite database
  |
  +-- aggregate reports: summary, courses, days, hours, sessions
  +-- explicit PII report: members
```

## Trust boundaries

1. `internal/transport` owns the only network client. It accepts only HTTPS, the exact production hostname, no query string, the two approved POST paths, and same-host HTTPS redirects.
2. `internal/secrets` owns credential profiles and cached sessions in the platform keyring. Secret values never enter logs, configuration files, command-line arguments or SQLite.
3. `internal/mysign` parses a response into a strict graph. Missing attendance metrics, duplicate identifiers and unknown references fail closed.
4. `internal/store` imports a complete snapshot transactionally. Re-importing the same source identifiers updates rows instead of duplicating them.
5. `internal/output` renders local table, JSON or CSV output. The CLI guards member names and identifiers behind an explicit personal-data flag.

## Session state machine

```text
cached session?
  +-- yes -> read
  |          +-- success -> rotate token, persist, stop (1 request)
  |          +-- expired -> login -> read once, stop (3 max)
  |          +-- other error/schema drift -> stop
  +-- no  -> login -> read once, stop (2 requests)
```

There is no generic retry loop. A second failed read never causes another login. Concurrent callers in one process are serialized. The CLI does not schedule itself; users or an external scheduler control frequency.

## Persistence

- `members`: source ID, member number and local display identity.
- `course_sessions`: dated course occurrences, room and time.
- `prescriptions`: local prescription counters.
- `attendance`: member/session/prescription links and attendance flags.
- `sync_runs`: payload fingerprint, status, timestamps and row counts.

The source currently exposes a rolling snapshot. Rows are retained locally when they later disappear from that window, building history without remote backfills. SQLite and WAL files are mode `0600`; operators should also use full-disk encryption, host access controls and encrypted backups.

## Printing Press

The sanitized Printing Press spec records the observed remote contract and passes a no-network dry run. The generated client is not the runtime because the current generator cannot encode the semantic-read POST, keyring handling, request budget, exact-one-relogin behavior and fail-closed schema boundary as one generated contract. The hand-written transport is smaller and directly tests those invariants.

## Non-goals for version 1

- no Magicline integration or name-based cross-system matching;
- no polling, daemon, cloud sync, hosted dashboard or MCP server;
- no myYOLO writes, signatures, course edits, documents or billing operations;
- no CAPTCHA bypass, browser fingerprinting, proxy rotation or stealth behavior;
- no claim that course windows equal physical facility dwell time.
