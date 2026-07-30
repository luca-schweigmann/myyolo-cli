# Capability map

This map separates what the CLI has verified from what merely appears as a link
or possible future integration. It contains no account values, member records,
captured responses, cookies, or live counts.

The current typed catalogue contains 99 read-only capabilities across
analytics, courses, members, prescriptions, compliance and isolated financial
reads. See [the complete read catalogue](read-catalog.md) or run
`myyolo catalog --format json`.

## Status vocabulary

- **Supported**: implemented, covered by synthetic tests, and verified against
  the authorized read-only surface.
- **Structured**: collected into private SQLite with a stable schema fingerprint,
  but not yet promoted into every possible domain-specific report.
- **Classified**: implemented in the typed allowlist with synthetic validation;
  the source may still return no rows, a changed schema, or a non-HTML export
  on a particular installation, in which case collection fails closed.
- **Linked candidate**: visible from an authorized navigation page but not
  allowlisted or fetched by the CLI.
- **Out of scope**: deliberately unavailable in the read-only CLI.

## mySIGN

| Capability | Status | Local data / command |
|---|---|---|
| Partner-scoped login | Supported | `auth login`, separate mySIGN keyring session |
| Expired-session recovery | Supported | one login and one final read, three requests maximum |
| Course occurrences | Supported | `course_sessions`; `report sessions`, `courses`, `days`, `hours` |
| Member references | Supported | `members`; PII-gated `report members` |
| Attendance flags | Supported | attended, signed, manually signed, cancelled |
| Prescriptions | Supported | treatment, weekly-treatment, visit, remaining-unit fields |
| Missing signatures | Supported | attended, unsigned, not cancelled |
| No-shows | Supported | only after the course end; future/running/unknown-end rows are pending |
| Physical facility dwell time | Not exposed | course windows must not be represented as physical check-in duration |

mySIGN returns a rolling snapshot. The CLI builds local history from repeated
captures but cannot reconstruct periods that were never collected.

## Classic myYOLO administration

The original six-page `discover admin` command remains a bounded compatibility
and schema-discovery path. New operational reads use
`myyolo collect CAPABILITY`, which validates one typed capability and its
parameters, makes at most five requests including session recovery, and stores
the result under a parameter-free capability key.

All routes below are exact-host, exact-method, exact-path GET reads. Login uses
the observed form POST plus its two observed redirect hops. Discovery never
submits member IDs, signatures, edits, billing actions, or date-search forms.

| Capability | Route | Status | Local report |
|---|---|---|---|
| Attendance navigation/session probe | `/start_Anwesenheit.asp` | Supported | `sync admin`, `auth check --source admin` |
| Currently present page | `/Anwesenheit/Anwesend_Aktuell_Liste.asp` | Structured | `report admin-capabilities`; PII-gated generic records |
| Present-today page | `/Anwesenheit/Anwesend_Heute_Liste.asp` | Structured | `report admin-capabilities`; PII-gated generic records |
| Attendance-by-date page | `/Anwesenheit/Anwesend_Datum_Liste.asp` | Structured | GET snapshot only; POST date filter is not submitted |
| Reha attendance/time windows | `/Anwesenheit/Reha_Anwesend_Datum_Liste.asp` | Supported | `admin-reha-hours`; PII-gated `admin-reha-attendance` |
| Monthly course participation | `/Statistiken/Kursplaner/Kurs_Teilnehmer_anwesend_monatlich.asp` | Supported | `admin-course-months` |
| Missing Reha signatures | `/Vertrag_Reha/Unterschrift/Fehlende_Reha_Unterschriften_Liste.asp` | Supported | aggregate `admin-missing-signatures`; PII-gated member detail |

The Reha time-window report provides scheduled or recorded windows exposed by
that page. It calculates minutes per window and attendee-minutes. It does not
claim that these windows are equivalent to Magicline entry/exit events.

### Typed capabilities

The classified catalogue adds 99 individually selectable reads. It includes
historical aggregate attendance, course and participant controlling,
person-level check-in/Reha history, bounded lists and exports,
billing-readiness checks, and separately gated financial archives. It does not
crawl navigation or automatically fan out from search results. Each live run
selects exactly one named capability.

Catalogue membership means the route and parameter shape are classified; it
does not mean all 99 routes were bulk-tested against a live account. Doing so
would conflict with the server-load policy. Representative live verification
should remain small and operator-directed.

### Deliberately blocked

- logout routes, because remote logout is a state change and local session
  removal is sufficient;
- bulk member-detail loops or automatic traversal from a member list;
- login-time, signature, attendance, contract, billing, note, or course edits;
- any form POST not explicitly classified as a semantic read with typed fields;
- broad crawling, guessed paths, automatic form submission, retries after
  schema drift, CAPTCHA handling, or rate-limit bypass.

## Magicline seam

There is no Magicline transport in this version. Identical names are not a safe
join key. A future integration should prefer stable IDs or member numbers,
retain source namespaces, record match confidence and provenance, and require a
separate privacy and reconciliation review.

## Capability discovery budget

`myyolo discover admin` reads the six supported/structured aggregate pages
serially. With a valid cached session it uses seven requests including the
session probe. With an expired session it uses exactly ten: one failed probe,
three login/redirect requests, and six reads. No partial remote result is
imported if any page fails.

`myyolo collect` is the safer operational default: one session probe plus one
selected read with a valid cache, or one bounded login plus one selected read.
Its shared hard ceiling is five requests and its minimum inter-request delay is
two seconds. There is no `collect all`.
