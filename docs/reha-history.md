# Lokale Reha-Historie: v1 und Management-v2

`report reha-history` liest ausschließlich eine private, hashgebundene SQLite-
Kopie und ihren Standort-Scope-Receipt. Es stellt keine Netzwerkanfragen.

```sh
myyolo report reha-history --db /private/snapshot.sqlite \
  --scope-file /private/snapshot.scope.json --location point-gerlingen \
  --from 2026-07-01 --to 2026-09-16 --schema-version v2
```

`--schema-version v1` bleibt der Standard mit unverändertem
`reha-history-observations.v1`-JSON. v2 ist ausdrücklich opt-in und liefert
`reha-history-observations.v2`; alle bisherigen Sitzungsfelder bleiben erhalten.
Der Zeitraum ist inklusive und wird als Kalendertage in Europe/Berlin ausgewertet.

Zusätzliche v2-Felder pro vorhandener Sitzung:

| Feld | Bedeutung |
|---|---|
| `course_label` | Beschreibung aus `KursBuchungen.Beschreibung`; Anzeige, kein Identitätsschlüssel |
| `stable_course_id` | `null`: mySIGN liefert hier keine separate Kursserien-ID |
| `registered` | Validiertes `TeilnehmerAnzahl` der gespeicherten Quellbeobachtung; sonst `null` |
| `registered_observed_at` | Beobachtungszeit genau dieses Eintragungswertes; sonst `null` |
| `participated` | Anzahl `HatUnterschrift && Teilgenommen && !Storniert` einer belegten vollständigen Snapshot-Collection; sonst `null` |
| `attendance_observed_at` / `attendance_status` | Eigener Beobachtungszeitpunkt und `complete_for_observed_snapshot`; bei fehlendem Beleg unbekannt |
| `registration_status` | `validated_observation` oder `unknown` |
| `capacity` | `null`: diese Quelle enthält keine belegte Kapazität |
| `completeness` | `observed_rows_only`, keine behauptete Vollständigkeit des angefragten Zeitraums |
| `provenance` | Explizite Quellen-/Unbekannt-Angaben je Kennzahl und Kursidentität |

`registered: 0` bedeutet einen tatsächlich vorhandenen, validierten Quellwert.
Ein früherer SQLite-Standardwert 0 genügt nicht. `null` bleibt unbekannt und darf
in einem Diagramm oder Durchschnitt nicht durch 0 ersetzt werden. Teilnahmen
zählen beobachtete Anwesenheitszeilen, keine eindeutigen Personen und keine
abrechenbaren Einheiten. Heutige und künftige lokale Tage sind vorläufig und nicht als abgeschlossene Tage
für Teilnahme-Durchschnitte zu verwenden. Die Ausgabe rekonstruiert keine
historischen Voranmeldestände; sie gibt die neueste gespeicherte Beobachtung des
historischen Termins wieder.

Die additive Tabelle `reha_session_observations` speichert den validierten
Eintragungswert samt Zeitpunkt und Quell-Sitzungs-ID privat neben der Sitzung.
Sie speichert zusätzlich die Zeilenzahl und qualifizierten Teilnahmen genau der eingehenden
validierten Attendance-Collection. Sie wird atomar mit dem Import aktualisiert.
Alte per UPSERT stehengebliebene Attendance-Zeilen werden deshalb in v2 nicht
unbelegt als aktuelle Teilnahme gezählt. Eine Migration erzeugt nur die
leere Tabelle und validiert keine Altzeilen nachträglich. So bleiben akkumulierte
Imports belegbar, auch wenn spätere Snapshots alte Termine nicht mehr enthalten.
Alte einzelne, streng validierte Imports können weiterhin ihren passenden
`reha_import_observation`-Beleg verwenden. Ein veralteter oder abweichender Beleg
ergibt unbekannt. Schema-5/6-Snapshots bleiben lesbar und werden nie migriert.

Sitzungs-Pseudonyme bleiben gegenüber v1 identisch. Gleichnamige Kurse oder
identische Uhrzeiten werden nie zusammengeführt. Mitglieder-, Verordnungs-,
Anwesenheits- und rohe Sitzungs-IDs sowie Raum- und Personendaten fehlen im
Export. Kurslabels müssen bei Darstellung wie jeder Quelltext escaped werden.

Die bereinigte Printing-Press-Spezifikation beschreibt zusätzlich die relevanten
mySIGN-Felder. Sie ändert keine Remote-Route und erzeugt keine neue API-Fähigkeit.

V2 weist in `coverage_detail` das beobachtete Termin-Inventar, die Verfügbarkeit
von Eintragungswerten und die belegten Attendance-Collections getrennt aus.
`expected_sessions` und `missing_sessions` bleiben ohne autoritativen Kalender
`null`. Ein vollständig gelesener Snapshot beweist nicht, dass seit Juli jeder
geplante Termin vorhanden ist. `requested_from`, `requested_to`, `as_of` und
`timezone` machen den Auswertungsrahmen explizit. V1-Zählfelder bleiben aus
Kompatibilitätsgründen erhalten; v2-Konsumenten verwenden die nullable
`registered`-/`participated`-Felder samt ihren jeweiligen Belegen.
