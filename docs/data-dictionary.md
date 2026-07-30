# Data and metric dictionary

## Source namespaces

- `mysign`: rotating JSON snapshot from `sign.azh-myyolo.info`.
- `admin_page`: structured HTML table observations from
  `www.azh-myyolo.info`.

Credentials may be shared by an authorized operator profile, but sessions and
stored records are never merged implicitly.

## mySIGN metrics

| Metric | Definition |
|---|---|
| `bookings` | Stored member/course relationship rows |
| `attended` | Source flag `Teilgenommen` is true |
| `signed` | Source flag `HatUnterschrift` is true |
| `cancelled` | Source flag `Storniert` is true |
| `missing_signatures` | Attended, unsigned, and not cancelled |
| `no_shows` | Unattended, not cancelled, and the parsed course end is before the report `as-of` |
| `pending` | Unattended, not cancelled, and course end is future, current, or unknown |
| `distinct_participants` | Distinct mySIGN member IDs represented in attendance |

The `--as-of` flag accepts RFC3339 and makes time-bound reports reproducible.
At the exact end timestamp a row remains pending; it becomes a no-show only
after that timestamp.

## Admin metrics

| Report | Definition |
|---|---|
| `admin-reha-hours` | Current rows grouped by exposed Reha time window; includes duration and attendee-minutes |
| `admin-course-months` | Sum of numeric monthly participation values plus number of courses with numeric data |
| `admin-missing-signatures` | Count of current member rows and sum of the exposed `Menge` field |

Admin domain reports use only records whose `last_observed_at` equals the
current capability observation. Older distinct row versions remain in SQLite
for history but do not inflate current reports.

`report capability NAME` applies the same latest-observation rule to every
typed capability. The stored route is `capability:NAME`; dynamic member IDs,
search text, dates and other submitted filter values are not included in that
route key or command output.

## Provenance and retention

- `sync_runs` records source, normalized/schema fingerprint, start/completion,
  status, and aggregate row counts.
- mySIGN records retain stable remote IDs plus `source='mysign'`.
- admin records retain an exact route or parameter-free capability key,
  leaf-table index, deterministic row hash, structured JSON values, first
  observation, and last observation.
- admin capability fingerprints include routes, titles, headings, headers, and
  sanitized form metadata; row values do not affect the fingerprint.
- raw HTTP bodies, passwords, cookies, tokens, HTML files, and signature images
  are not stored in SQLite.
