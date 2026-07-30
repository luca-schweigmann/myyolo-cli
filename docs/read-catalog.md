# Typed read catalogue

The CLI contains 99 explicitly classified, read-only admin capabilities. The
authoritative machine-readable view is always available locally:

```bash
myyolo catalog --format json
myyolo catalog --group analytics
myyolo collect CAPABILITY [required filters] --dry-run [data-scope flags]
```

`collect` fetches exactly one capability, never follows catalogue links, never
submits an edit/billing/signature action, and uses at most five serial HTTP
requests including session probe and one autonomous login. The minimum delay
between admin requests is two seconds. All reports after collection read
SQLite and make zero HTTP requests.

Sensitivity gates:

| Scope | Required flags |
|---|---|
| `aggregate` | none |
| `personal` | `--include-personal-data` |
| `health` | `--include-personal-data --include-health-data` |
| `financial` | `--include-personal-data --include-financial-data` |

The catalogue records the exact HTTP method and static path in
`myyolo catalog`; this document focuses on operator-facing names and filters.
Descriptions are source-surface descriptions, not a promise that every
installation exposes rows for every capability. HTML and HTML-based Excel
responses are supported; a binary export fails closed without being stored.

## Analytics (17)

- `absences` — personal: recorded absence list
- `age-structure` — aggregate: member age distribution
- `attendance-current` — personal: currently present people
- `attendance-date` — personal: attendance page for the server-selected date
- `attendance-monthly` — aggregate: historic active-member attendance by month
- `attendance-today` — personal: people present today
- `attendance-week-range` — aggregate; `--year --week-from --week-to`
- `attendance-weekly` — aggregate: historic facility attendance by week
- `first-contact-active` — personal: first-contact evaluation for active people
- `first-contact-passive` — personal: first-contact evaluation for passive people
- `prevention-attendance` — health; `--from --to`
- `prevention-attendance-monthly` — aggregate: prevention attendance by month
- `prevention-week-range` — aggregate; `--year --week-from --week-to`
- `reha-attendance` — health: Reha attendance and exposed time windows
- `reha-attendance-monthly` — aggregate: Reha attendance by month
- `reha-week-range` — aggregate; `--year --week-from --week-to`
- `studio-hourly-load` — aggregate; `--from --to`

## Courses (18)

- `course-attended` — health; `--from --to`
- `course-cancelled` — health; `--from --to`
- `course-day-list` — personal; `--date`
- `course-definitions` — aggregate: course, schedule, room and active status
- `course-history` — aggregate; `--course-id`
- `course-last-appointment-export` — personal
- `course-monthly` — aggregate: monthly course attendance
- `course-not-attended` — health; `--from --to`
- `course-planner-day` — personal
- `course-planner-week` — personal
- `course-session` — health; `--course-id --date --planner`
- `members-not-attending` — personal
- `members-with-appointment` — personal
- `members-with-appointment-7-days` — personal
- `members-with-appointment-14-days` — personal
- `members-without-appointment` — personal
- `missed-appointment-contacts` — personal
- `repeated-course-no-shows` — personal

## Members and contracts (31)

- `birthdays` — personal
- `booked-contract-pauses` — personal
- `card-contracts` — personal
- `contract-pauses` — personal
- `contracts-complete-archive` — personal
- `contracts-with-addons` — personal
- `contracts-with-address` — personal
- `extensions` — personal
- `member-checkins` — personal; `--member-id`
- `member-detail` — personal; `--member-id`
- `member-expired-prescriptions` — health; `--member-id`
- `member-missing-signatures` — health; `--member-id`
- `member-reha-history` — health; `--member-id`
- `member-reha-planner` — health; `--member-id`
- `member-search` — personal; `--population --search`
- `member-sport-planner` — health; `--member-id`
- `memberships` — personal
- `notes` — personal
- `people` — personal
- `people-active-long` — personal
- `people-active-short` — personal
- `people-former-long` — personal
- `people-former-short` — personal
- `people-staff-long` — personal
- `people-staff-short` — personal
- `people-with-prevention` — health
- `people-with-reha` — health
- `people-without-reha-prevention` — personal
- `prevention-members` — health
- `prevention-pauses` — health
- `reha-members` — health

The individual attendance-history endpoints expose the server-default history
window. The CLI does not claim a custom historical range where the source does
not expose one through the classified request.

## Prescriptions (21)

- `duplicate-reha-times` — health
- `prescription-expiry` — health; `--from --to`
- `prescription-last-attendance` — health; `--from`
- `prescription-remaining-units` — health; `--threshold`
- `prevention-prescription-summary` — health; `--from --to [--referrer-id]`
- `prevention-prescriptions-by-referrer` — health; `--from --to [--referrer-id]`
- `reha-multiple-prescriptions` — health
- `reha-plus` — health
- `reha-plus-without-visit` — health
- `reha-prescription-summary` — health; `--from --to [--referrer-id]`
- `reha-prescriptions` — health
- `reha-prescriptions-archived` — health
- `reha-prescriptions-by-referrer` — health; `--from --to [--referrer-id]`
- `reha-time-visit-counts` — health
- `reha-times-active` — health
- `reha-times-archived` — health
- `reha-times-inconsistent` — health
- `reha-without-prescription` — health
- `reha-without-visit` — health
- `trena-prescriptions` — health
- `trena-prescriptions-archived` — health

## Billing-readiness compliance checks (7)

These are read-only validation pages. They do not create, edit, submit, close,
or transmit a billing:

- `billing-number-mismatches` — health
- `duplicate-daily-units` — health
- `missing-attendee-signatures` — health
- `missing-instructor-signatures` — health
- `missing-insurance-data` — health
- `missing-reha-signatures` — health
- `prescription-unit-overruns` — health

## Isolated financial reads (5)

Financial capabilities never appear in a normal aggregate collection and
require the dedicated financial gate:

- `digital-billing-complete-archive` — financial; `--ik`
- `digital-billing-final-archive` — financial; `--ik`
- `member-bank-export` — financial
- `private-billing-digital-archive` — financial
- `private-billing-manual-archive` — financial

## Local reporting

```bash
myyolo report capability CAPABILITY [data-scope flags] \
  [--db PATH] [--format table|json|csv]
```

This command returns only rows from the newest complete observation for that
capability. Older row versions remain in SQLite for local time-series work.
Neither this command nor `catalog` contacts myYOLO.
