# Typisierter Read-Katalog

Die CLI enthält 99 explizit klassifizierte, schreibgeschützte Admin-Capabilities. Die
maßgebliche maschinenlesbare Ansicht ist lokal immer verfügbar:

```bash
myyolo catalog --format json
myyolo catalog --group analytics
myyolo collect CAPABILITY [required filters] --dry-run [data-scope flags]
```

`collect` holt genau eine Capability, folgt nie Katalog-Links, sendet nie eine
Edit-/Abrechnungs-/Unterschrifts-Aktion und nutzt höchstens fünf serielle HTTP-
Anfragen inklusive Session-Probe und einem autonomen Login. Die Mindestverzögerung
zwischen Admin-Anfragen beträgt zwei Sekunden. Alle Reports nach dem Collect lesen
SQLite und stellen keine HTTP-Anfragen.

Sensitivitäts-Gates:

| Scope | Erforderliche Flags |
|---|---|
| `aggregate` | keine |
| `personal` | `--include-personal-data` |
| `health` | `--include-personal-data --include-health-data` |
| `financial` | `--include-personal-data --include-financial-data` |

Der Katalog speichert die exakte HTTP-Methode und den statischen Pfad in
`myyolo catalog`; dieses Dokument konzentriert sich auf operatorseitige Namen und
Filter. Beschreibungen sind Oberflächenbeschreibungen der Quelle, kein Versprechen,
dass jede Installation für jede Capability Zeilen liefert. HTML- und HTML-basierte
Excel-Antworten werden unterstützt; ein binärer Export wird fail-closed abgelehnt
und nicht gespeichert.

## Analytics (17)

- `absences` - personal: erfasste Abwesenheitsliste
- `age-structure` - aggregate: Altersverteilung der Mitglieder
- `attendance-current` - personal: aktuell anwesende Personen
- `attendance-date` - personal: Anwesenheitsseite für das vom Server gewählte Datum
- `attendance-monthly` - aggregate: historische Anwesenheit aktiver Mitglieder nach Monat
- `attendance-today` - personal: heute anwesende Personen
- `attendance-week-range` - aggregate; `--year --week-from --week-to`
- `attendance-weekly` - aggregate: historische Einrichtungs-Anwesenheit nach Woche
- `first-contact-active` - personal: Erstkontakt-Auswertung für aktive Personen
- `first-contact-passive` - personal: Erstkontakt-Auswertung für passive Personen
- `prevention-attendance` - health; `--from --to`
- `prevention-attendance-monthly` - aggregate: Präventions-Anwesenheit nach Monat
- `prevention-week-range` - aggregate; `--year --week-from --week-to`
- `reha-attendance` - health: Reha-Anwesenheit und freigegebene Zeitfenster
- `reha-attendance-monthly` - aggregate: Reha-Anwesenheit nach Monat
- `reha-week-range` - aggregate; `--year --week-from --week-to`
- `studio-hourly-load` - aggregate; `--from --to`

## Kurse (18)

- `course-attended` - health; `--from --to`
- `course-cancelled` - health; `--from --to`
- `course-day-list` - personal; `--date`
- `course-definitions` - aggregate: Kurs, Zeitplan, Raum und aktiver Status
- `course-history` - aggregate; `--course-id`
- `course-last-appointment-export` - personal
- `course-monthly` - aggregate: monatliche Kursanwesenheit
- `course-not-attended` - health; `--from --to`
- `course-planner-day` - personal
- `course-planner-week` - personal
- `course-session` - health; `--course-id --date --planner`
- `members-not-attending` - personal
- `members-with-appointment` - personal
- `members-with-appointment-7-days` - personal
- `members-with-appointment-14-days` - personal
- `members-without-appointment` - personal
- `missed-appointment-contacts` - personal
- `repeated-course-no-shows` - personal

## Mitglieder und Verträge (31)

- `birthdays` - personal
- `booked-contract-pauses` - personal
- `card-contracts` - personal
- `contract-pauses` - personal
- `contracts-complete-archive` - personal
- `contracts-with-addons` - personal
- `contracts-with-address` - personal
- `extensions` - personal
- `member-checkins` - personal; `--member-id`
- `member-detail` - personal; `--member-id`
- `member-expired-prescriptions` - health; `--member-id`
- `member-missing-signatures` - health; `--member-id`
- `member-reha-history` - health; `--member-id`
- `member-reha-planner` - health; `--member-id`
- `member-search` - personal; `--population --search`
- `member-sport-planner` - health; `--member-id`
- `memberships` - personal
- `notes` - personal
- `people` - personal
- `people-active-long` - personal
- `people-active-short` - personal
- `people-former-long` - personal
- `people-former-short` - personal
- `people-staff-long` - personal
- `people-staff-short` - personal
- `people-with-prevention` - health
- `people-with-reha` - health
- `people-without-reha-prevention` - personal
- `prevention-members` - health
- `prevention-pauses` - health
- `reha-members` - health

Die individuellen Anwesenheitsverlauf-Endpunkte liefern das serverseitige
Standard-Verlaufsfenster. Die CLI beansprucht keinen benutzerdefinierten
historischen Zeitraum, wenn die Quelle keinen über die klassifizierte Anfrage
bereitstellt.

## Verordnungen (21)

- `duplicate-reha-times` - health
- `prescription-expiry` - health; `--from --to`
- `prescription-last-attendance` - health; `--from`
- `prescription-remaining-units` - health; `--threshold`
- `prevention-prescription-summary` - health; `--from --to [--referrer-id]`
- `prevention-prescriptions-by-referrer` - health; `--from --to [--referrer-id]`
- `reha-multiple-prescriptions` - health
- `reha-plus` - health
- `reha-plus-without-visit` - health
- `reha-prescription-summary` - health; `--from --to [--referrer-id]`
- `reha-prescriptions` - health
- `reha-prescriptions-archived` - health
- `reha-prescriptions-by-referrer` - health; `--from --to [--referrer-id]`
- `reha-time-visit-counts` - health
- `reha-times-active` - health
- `reha-times-archived` - health
- `reha-times-inconsistent` - health
- `reha-without-prescription` - health
- `reha-without-visit` - health
- `trena-prescriptions` - health
- `trena-prescriptions-archived` - health

## Abrechnungsbereitschaft: Compliance-Prüfungen (7)

Das sind schreibgeschützte Validierungsseiten. Sie erstellen, bearbeiten, senden,
schließen oder übermitteln keine Abrechnung:

- `billing-number-mismatches` - health
- `duplicate-daily-units` - health
- `missing-attendee-signatures` - health
- `missing-instructor-signatures` - health
- `missing-insurance-data` - health
- `missing-reha-signatures` - health
- `prescription-unit-overruns` - health

## Isolierte Finanz-Reads (5)

Finanzielle Capabilities erscheinen nie in einer normalen Aggregat-Collection und
erfordern das dedizierte Finanz-Gate:

- `digital-billing-complete-archive` - financial; `--ik`
- `digital-billing-final-archive` - financial; `--ik`
- `member-bank-export` - financial
- `private-billing-digital-archive` - financial
- `private-billing-manual-archive` - financial

## Lokales Reporting

```bash
myyolo report capability CAPABILITY [data-scope flags] \
  [--db PATH] [--format table|json|csv]
```

Dieser Befehl liefert nur Zeilen aus der neuesten vollständigen Beobachtung für
diese Capability. Ältere Zeilenversionen bleiben in SQLite für lokale
Zeitreihenarbeit. Weder dieser Befehl noch `catalog` kontaktiert myYOLO.
