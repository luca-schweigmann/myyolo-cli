# Architecture

The CLI turns bounded, authorized reads from two related systems into durable
local history and runs all analytics against that history.

```text
one operator credential profile in the OS keyring
                    |
          +---------+---------+
          |                   |
          v                   v
  mySIGN JSON client     classic ASP admin client
  rotating token         isolated cookie jar
  <=3 requests           <=5 sync / <=10 discovery
  >=1s serial            >=2s serial
          |                   |
  strict JSON graph      99-route typed read catalogue
                         bounded leaf-table parser
          |                   |
          +---------+---------+
                    v
          private SQLite database
                    |
          local-only reports
          sensitivity details behind explicit flags
```

## Trust boundaries

1. `internal/transport` is the final host/method/path/query gate. A POST is
   never assumed to be a write or read from its verb; every exact route is
   classified.
2. `internal/readcatalog` is the typed source contract for one-capability
   collections. It validates dates, IDs, weeks, enums, search length, IK
   numbers, fixed query values and sensitivity before network access.
3. `internal/secrets` owns one credential envelope and distinct `mysign` and
   `admin` session envelopes in the platform keyring. Secret values never enter
   logs, configuration files, command-line arguments, or SQLite.
4. `internal/mysign` parses a strict graph. Missing attendance metrics,
   duplicate identifiers, and unknown references fail closed.
5. `internal/admin` disables automatic redirects, counts each ASP auth hop,
   owns an isolated cookie jar, decodes the declared response charset, rejects
   CAPTCHA/rate-limit/schema anomalies and non-HTML binary exports, and parses
   bounded leaf tables.
6. `internal/store` imports mySIGN graphs and structured admin observations.
   Current admin reports join only the latest observation; older row versions
   remain available as local history.
7. `internal/output` renders local table, JSON, or CSV. Personal, health and
   financial rows require distinct explicit data-scope flags.

## mySIGN session state machine

```text
cached session?
  +-- yes -> read
  |          +-- success -> rotate token, persist, stop (1 request)
  |          +-- expired -> login -> read once, stop (3 max)
  |          +-- other error/schema drift -> stop
  +-- no  -> login -> read once, stop (2 requests)
```

There is no generic retry loop. A second failed read never causes another login. Concurrent callers in one process are serialized. The CLI does not schedule itself; users or an external scheduler control frequency.

## Admin session state machine

```text
cached admin cookies?
  +-- yes -> exact session probe
  |          +-- success -> read requested pages
  |          +-- redirect to observed unauthenticated landing
  |                        -> login once -> read once
  |          +-- any other result -> stop
  +-- no  -> login once -> read once
```

The login flow is counted explicitly:

```text
POST /Anmelden.asp?vw=
  -> GET /LoginHandler.asp
  -> GET /Start.asp (or observed lowercase alias)
```

`http.Client` never follows redirects automatically. Every hop passes the same
allowlist, delay, and shared budget. A normal admin sync uses at most five
requests; full discovery uses at most ten, including an expired-session probe.
A typed collection reads exactly one capability with a maximum of five
requests. All calls inside one operation are serial and at least two seconds
apart.

## Persistence

- `members`: source namespace, source ID, member number, local display identity.
- `course_sessions`: dated course occurrences, room and time.
- `prescriptions`: local prescription counters.
- `attendance`: member/session/prescription links and attendance flags.
- `admin_capabilities`: route/capability key, sanitized schema metadata and latest observation.
- `admin_records`: route/table/row hash, structured values and observation range.
- `sync_runs`: source fingerprint, status, timestamps and aggregate row counts.

mySIGN exposes a rolling snapshot. Rows are retained locally when they later
disappear from that window. Admin row hashes preserve changed/removed versions
while current reports filter to the latest capability observation. SQLite and
WAL files are mode `0600`; operators should also use full-disk encryption, host
access controls, and encrypted backups.

## Printing Press

Two sanitized Printing Press specs record the observed mySIGN and representative
admin contracts. The complete executable admin contract lives in the typed Go
catalogue and is exportable with `myyolo catalog --format json`. Generated
clients are not the runtime because the generator cannot encode rotating
tokens, cookie isolation, manual redirect budgets, semantic-read POSTs,
sensitivity gates, or fail-closed schema boundaries as one generated contract.

## Non-goals for version 1

- no Magicline integration or name-based cross-system matching;
- no polling, daemon, cloud sync, hosted dashboard, or MCP server;
- no myYOLO writes, signatures, course edits, documents or billing operations;
- no CAPTCHA bypass, browser fingerprinting, proxy rotation or stealth behavior;
- no member-detail crawling or fan-out collection;
- no unclassified query or form submission;
- no claim that course/Reha windows equal physical facility dwell time.
