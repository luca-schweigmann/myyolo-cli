# Daten- und Metrik-Wörterbuch

## Quell-Namespaces

- `mysign`: rotierender JSON-Snapshot von `sign.azh-myyolo.info`.
- `admin_page`: strukturierte HTML-Tabellenbeobachtungen von
  `www.azh-myyolo.info`.

Zugangsdaten können über ein autorisiertes Operator-Profil geteilt werden,
Sitzungen und gespeicherte Datensätze werden jedoch nie implizit zusammengeführt.

## mySIGN-Metriken

| Metrik | Definition |
|---|---|
| `bookings` | Gespeicherte Mitglied-/Kurs-Beziehungszeilen |
| `attended` | Quell-Flag `Teilgenommen` ist wahr |
| `signed` | Quell-Flag `HatUnterschrift` ist wahr |
| `cancelled` | Quell-Flag `Storniert` ist wahr |
| `missing_signatures` | Teilgenommen, ohne Unterschrift und nicht storniert |
| `no_shows` | Nicht teilgenommen, nicht storniert, und das geparste Kursende liegt vor dem Report-`as-of` |
| `pending` | Nicht teilgenommen, nicht storniert, und Kursende liegt in der Zukunft, aktuell oder ist unbekannt |
| `distinct_participants` | Verschiedene mySIGN-Mitglieds-IDs in der Anwesenheit |

Der Schalter `--as-of` akzeptiert RFC3339 und macht zeitgebundene Reports
reproduzierbar. Zum exakten End-Zeitstempel bleibt eine Zeile ausstehend
(`pending`); sie wird erst danach zum No-Show (`no_shows`).

## Admin-Metriken

| Report-Befehl | Definition |
|---|---|
| `admin-reha-hours` | Aktuelle Zeilen gruppiert nach freigegebenem Reha-Zeitfenster; inkl. Dauer und Teilnehmer-Minuten |
| `admin-course-months` | Summe numerischer monatlicher Teilnahmewerte plus Anzahl Kurse mit numerischen Daten |
| `admin-missing-signatures` | Anzahl aktueller Mitgliedszeilen und Summe des freigegebenen Felds `Menge` |

Admin-Domain-Reports nutzen nur Datensätze, deren `last_observed_at` der
aktuellen Capability-Beobachtung entspricht. Ältere unterschiedliche
Zeilenversionen bleiben in SQLite für die Historie, blähen aktuelle Reports
aber nicht auf.

`report capability NAME` wendet dieselbe Regel der neuesten Beobachtung auf
jede typisierte Capability an. Die gespeicherte Route ist `capability:NAME`;
dynamische Mitglieds-IDs, Suchtext, Daten und andere übermittelte Filterwerte
sind in diesem Route-Key und in der Befehlsausgabe nicht enthalten.

## Herkunft und Aufbewahrung

- `sync_runs` speichert Quelle, normalisierten/Schema-Fingerprint, Start/Abschluss,
  Status und aggregierte Zeilenzahlen.
- mySIGN-Datensätze behalten stabile Remote-IDs plus `source='mysign'`.
- Admin-Datensätze behalten einen exakten Route- oder parameterfreien Capability-Key,
  Leaf-Table-Index, deterministischen Zeilen-Hash, strukturierte JSON-Werte, erste
  Beobachtung und letzte Beobachtung.
- Admin-Capability-Fingerprints umfassen Routen, Titel, Überschriften, Header und
  bereinigte Formular-Metadaten; Zeilenwerte beeinflussen den Fingerprint nicht.
- Rohe HTTP-Bodies, Passwörter, Cookies, Tokens, HTML-Dateien und Unterschriftsbilder
  werden nicht in SQLite gespeichert.
