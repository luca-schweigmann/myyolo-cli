# Capability map

This map separates what the CLI has verified from what merely appears as a link
or possible future integration. It contains no account values, member records,
captured responses, cookies, or live counts.

## Status vocabulary

- **Supported**: implemented, covered by synthetic tests, and verified against
  the authorized read-only surface.
- **Structured**: collected into private SQLite with a stable schema fingerprint,
  but not yet promoted into every possible domain-specific report.
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

### Linked candidates not fetched

Authorized navigation exposes additional areas, including prevention
attendance, people, course planners, management, marketing, and debit/billing
navigation. They are not allowlisted merely because a link exists. Each needs a
separate read-only classification, bounded parser, synthetic contract, and
operator-value justification before it can enter discovery.

### Deliberately blocked

- logout routes, because remote logout is a state change and local session
  removal is sufficient;
- member-detail loops and query URLs carrying member IDs;
- login-time, signature, attendance, contract, billing, note, or course edits;
- the date-filter POST until its response contract and operational need are
  independently validated;
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
