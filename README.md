# myyolo-cli

Eine inoffizielle reine Lese-CLI, die Daten aus mySIGN und der klassischen
myYOLO-Verwaltung kontrolliert lokal sammelt und auswertet. Ein autorisiertes
Zugangsprofil kann für beide Systeme genutzt werden. Die Sitzungen mit
rotierendem Token und ASP-Cookies bleiben dabei getrennt im Schlüsselbund des
Betriebssystems. Serverabfragen erfolgen klein und nacheinander. Auswertungen
laufen anschließend aus einer privaten lokalen SQLite-Datenbank.

Dieses Projekt ist weder mit myYOLO, azh oder NOVENTI verbunden noch von diesen
Unternehmen freigegeben oder unterstützt. Nutze es nur mit ausdrücklicher
Berechtigung und im Rahmen der Vereinbarung, die für deinen Zugang gilt.

## Was die CLI kann

| Funktion | Befehl | Serverabfragen |
|---|---|---:|
| Beide Logins prüfen und sicher speichern | `myyolo auth login --source all` | 4 |
| Beide Sitzungen prüfen, inklusive einmaligem autonomem Login | `myyolo auth check` | mySIGN 1 bis 3, Admin 1 bis 5 |
| Autonomen Login gezielt ohne menschliche Eingabe testen | `myyolo auth check --force-relogin` | mySIGN 2, Admin 2 bis 4 |
| Prüfen, ob ein lokales Profil eingerichtet ist | `myyolo auth status` | 0 |
| Lokales Zugangs- und Sitzungsprofil entfernen | `myyolo auth logout` | 0 |
| Aktuellen mySIGN-Snapshot in SQLite übernehmen | `myyolo sync` | 1 bis 3 |
| Startseite der Admin-Anwesenheit übernehmen | `myyolo sync admin` | 1 bis 5 |
| Sechs freigegebene Admin-Seiten einlesen | `myyolo discover admin` | 7 bis 10 |
| Alle 99 typisierten Lesefunktionen lokal anzeigen | `myyolo catalog` | 0 |
| Genau eine ausgewählte Admin-Funktion abrufen | `myyolo collect CAPABILITY` | 2 bis 5 |
| Letzten Snapshot einer Funktion lokal ausgeben | `myyolo report capability CAPABILITY` | 0 |
| Leere lokale Datenbank anlegen | `myyolo db init` | 0 |
| Datenbankintegrität und Gesamtstatus prüfen | `myyolo db status` | 0 |
| Schlüsselbund und Datenbank prüfen | `myyolo doctor` | 0 |
| Lokalen Snapshot offline importieren | `myyolo import mysign` | 0 |
| Gesamtzahlen zu Sammlung und Anwesenheit anzeigen | `myyolo report summary` | 0 |
| Anwesenheit nach Kurs auswerten | `myyolo report courses` | 0 |
| Anwesenheit nach Kalendertag auswerten | `myyolo report days` | 0 |
| Anwesenheit nach Startzeit auswerten | `myyolo report hours` | 0 |
| Einzelne Kurstermine auswerten | `myyolo report sessions` | 0 |
| Teilnahme pro Mitglied auswerten | `myyolo report members` | 0 |
| Gesammelte Admin-Routen und Schemas prüfen | `myyolo report admin-capabilities` | 0 |
| Reha-Zeitfenster und Dauer auswerten | `myyolo report admin-reha-hours` | 0 |
| Monatliche Kursteilnahme auswerten | `myyolo report admin-course-months` | 0 |
| Fehlende Reha-Unterschriften zählen | `myyolo report admin-missing-signatures` | 0 |

Personenbezogene, gesundheitliche und finanzielle Abfragen sind durch getrennte
Freigabeschalter geschützt. Eine vollständige Übersicht steht im
[Lesekatalog](docs/read-catalog.md), in der
[Funktionsübersicht](docs/capabilities.md) und im
[Datenwörterbuch](docs/data-dictionary.md).

## Sicherheitsmodell

- Jede Abfrage muss exakt zu HTTPS-Host, Methode, Pfad und klassifizierten
  Parametern passen. Unbekannte Routen und Query-Strings werden blockiert.
- Mit einer gültigen gespeicherten Sitzung benötigt ein Sync genau eine
  Abfrage.
- Bei einer abgelaufenen mySIGN-Sitzung erfolgen genau ein Login und ein
  abschließender Leseversuch. Mehr als drei Abfragen gibt es nicht.
- Admin-Weiterleitungen werden manuell verfolgt und mitgezählt. Ein Sync nutzt
  höchstens fünf Abfragen, die Erkennung höchstens zehn. Admin-Abfragen laufen
  nacheinander und mit mindestens zwei Sekunden Abstand.
- `collect` akzeptiert pro Lauf genau eine benannte Funktion. Es gibt keinen
  `all`-Modus. Einschließlich Sitzungsprüfung und erneutem Login gelten maximal
  fünf Abfragen. Parameter werden vor dem Zugriff auf Zugangsdaten oder
  Netzwerk geprüft.
- HTTP 429, CAPTCHA, ein unerwarteter Login-Ablauf, Schemaänderungen und nicht
  klassifizierte Routen führen sofort zum Abbruch. Es gibt kein Polling, keine
  Wiederholungsschleife, keine Browser-Fingerabdruck-Imitation, keine
  Proxy-Rotation und kein Tarnverhalten.
- Rohe JSON- oder HTML-Antworten werden nicht gespeichert. SQLite- und
  WAL-Dateien erhalten den Modus `0600`.
- Standardmäßig sind nur aggregierte Abfragen erlaubt. Personenbezogene Daten
  brauchen `--include-personal-data`, Reha- und Verordnungsdaten zusätzlich
  `--include-health-data`, Bank- und Abrechnungsdaten zusätzlich
  `--include-financial-data`.

## Installation

Fertige Archive und `checksums.txt` liegen auf der
[GitHub-Releases-Seite](https://github.com/luca-schweigmann/myyolo-cli/releases).
Wähle das passende Archiv für macOS, Linux oder Windows sowie AMD64 oder ARM64,
entpacke es und lege `myyolo` beziehungsweise `myyolo.exe` unter Windows in
deinem `PATH` ab.

Die Archive besitzen Prüfsummen, sind derzeit aber noch nicht signiert oder
notarisiert. Prüfe die Prüfsumme vor der Nutzung. Organisationen, die signierte
Binärdateien benötigen, sollten bis zu einer Signatur-Pipeline aus dem geprüften
Quellcode bauen.

Mit Go 1.26.5 oder neuer:

```bash
go install github.com/luca-schweigmann/myyolo-cli/cmd/myyolo@latest
```

Aus einem lokalen Repository:

```bash
make check
make build
```

Die Binärdatei liegt anschließend unter `bin/myyolo`.

## Schnellstart

```bash
# 1. Beide Systeme einmal anmelden. Das Passwort wird sicher abgefragt.
myyolo auth login \
  --profile point \
  --source all \
  --partner YOUR_PARTNER_NUMBER \
  --username YOUR_USERNAME

# 2. Einen mySIGN-Snapshot und ausgewählte Admin-Statistiken abrufen.
myyolo sync mysign --profile point
myyolo collect attendance-monthly --profile point
myyolo collect course-monthly --profile point

# 3. Bereitschaft prüfen und Reports ohne weitere Serverabfrage lokal lesen.
myyolo doctor --profile point
myyolo report summary
myyolo report capability attendance-monthly
myyolo report capability course-monthly
```

## Agentenfreundliche Einrichtung

Autorisierte Coding-Agenten können die CLI ohne Änderungen am Quellcode und
ohne Passwörter in Prozessargumenten installieren, einrichten, prüfen und
ausführen. Benötigt werden Go 1.26.5, der Schlüsselbund des Betriebssystems,
autorisierte myYOLO-Zugangsdaten und ein privates lokales Verzeichnis für
SQLite und Report-Exporte.

```bash
git clone https://github.com/luca-schweigmann/myyolo-cli.git
cd myyolo-cli
make check
make build
./bin/myyolo version
```

Für eine nicht interaktive Anmeldung wird das Passwort über die
Standardeingabe übergeben:

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

Agenten können den vollständigen lokalen Vertrag prüfen, einen Befehl ohne
HTTP-Abfrage validieren, genau eine begrenzte Sammlung ausführen und danach
lokale JSON-Reports nutzen:

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

Agenten dürfen Zugangsdaten, Cookies, Tokens, Mitgliederdaten oder Datenbanken
nicht offenlegen, Quellabfragen nicht parallelisieren, den Admin-Abstand nicht
verringern und nach Rate Limits, CAPTCHA, Anmeldeauffälligkeiten oder
Schemaänderungen nicht fortfahren. Sensible Abfragen dürfen nur mit den
passenden expliziten Freigabeschaltern aktiviert werden. Der maschinenlesbare
Katalog ist über `myyolo catalog --format json` verfügbar. Die dokumentierten
Scopes stehen im [Lesekatalog](docs/read-catalog.md), die Metriken im
[Datenwörterbuch](docs/data-dictionary.md) und die Vertrauensgrenzen in der
[Architektur](docs/architecture.md).

## Ein oder mehrere Konten einrichten

Ein Profil enthält einen Satz Zugangsdaten sowie getrennte Sitzungen für
mySIGN und die Admin-Cookies. Bei der Passworteingabe erscheint kein Text im
Terminal. Passwörter werden nie als Kommandozeilenargument akzeptiert:

```bash
myyolo auth login \
  --profile studio-a \
  --source all \
  --partner YOUR_PARTNER_NUMBER \
  --username YOUR_USERNAME
```

Für einen zweiten autorisierten Zugang wird ein weiteres Profil verwendet.
`--source mysign` und `--source admin` prüfen nur das jeweilige System.
`--source all` ist die normale Einrichtung und gilt nur dann als erfolgreich,
wenn beide Anmeldungen funktionieren. Für Automationen ist
`--password-stdin` vorgesehen. `MYYOLO_PASSWORD` funktioniert als Rückfall,
kann aber für privilegierte lokale Prozesse sichtbar sein.

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

Logout entfernt nur das lokale Schlüsselbundprofil, nicht das Konto im
Quellsystem.

### Anmeldebefehle

#### `myyolo auth login`

Meldet sich an der ausgewählten Quelle oder an beiden Quellen an und speichert
danach Zugangsdaten und quellenspezifische Sitzungen im Schlüsselbund des
Betriebssystems.

| Schalter | Pflicht | Standard | Bedeutung |
|---|---:|---|---|
| `--profile NAME` | nein | `default` | Lokaler Name für einen getrennten Zugang |
| `--source SOURCE` | nein | `all` | `mysign`, `admin` oder beide |
| `--partner NUMBER` | ja | keiner | myYOLO-Partnernummer |
| `--username USER` | ja | keiner | myYOLO-Benutzername |
| `--password-stdin` | nein | `false` | Passwort über die Standardeingabe lesen |

Profilnamen dürfen Buchstaben, Zahlen, `.`, `_` und `-` enthalten, höchstens
64 Zeichen lang sein und nicht mit einem Sonderzeichen beginnen. Die CLI
besitzt bewusst keinen `--password`-Schalter, weil Prozessargumente für andere
Software sichtbar sein können.

#### `myyolo auth check`

Führt für die ausgewählte Quelle die kleinstmögliche Serverabfrage aus. Eine
gültige gespeicherte Sitzung wird wiederverwendet. Bei einer abgelaufenen
Sitzung folgen genau ein Login und ein abschließender Leseversuch. Danach wird
bei einem Fehler abgebrochen. Eine reparierte Sitzung wird für den nächsten
Befehl gespeichert.

```bash
myyolo auth check --profile studio-a --source all
myyolo auth check --profile studio-a --source admin --force-relogin
```

`--force-relogin` ignoriert für diese Prüfung die gespeicherte Sitzung, meldet
sich einmal mit den bereits im Schlüsselbund liegenden Zugangsdaten an, führt
einen abschließenden Leseversuch aus und ersetzt den Sitzungscache. Der Befehl
fragt nicht nach dem Passwort und dient als Nachweis, dass die unbeaufsichtigte
Sitzungswiederherstellung funktioniert.

#### `myyolo auth status`

Prüft ausschließlich den lokalen Schlüsselbund. Das Quellkonto wird nicht
getestet und es findet keine Netzwerkanfrage statt.

```bash
myyolo auth status --profile studio-a
```

Die JSON-Antwort zeigt, ob Zugangsdaten und beide quellenspezifischen Sitzungen
vorhanden sind. Benutzernamen, Passwörter, Cookies und Tokens werden nicht
ausgegeben.

#### `myyolo auth logout`

Löscht die Zugangsdaten dieses Profils und beide zwischengespeicherten
Sitzungen aus dem lokalen Schlüsselbund. Es wird keine entfernte
Abmelderoute aufgerufen und die SQLite-Datenbank bleibt unverändert.

## Synchronisation und Reports

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

Reports unterstützen Tabellen, JSON und CSV. Die Standarddatenbank liegt unter
`~/.local/share/myyolo-cli/myyolo.sqlite`. Mit `--db` oder `MYYOLO_DB_PATH`
kann ein anderer Pfad gewählt werden. Verwende für jedes Profil eine eigene
Datenbank, wenn die Quellkonten getrennt bleiben müssen.

### mySIGN-Synchronisation

```bash
myyolo sync mysign [--profile NAME] [--db PATH]
# Weiterhin unterstützte Kurzform:
myyolo sync [--profile NAME] [--db PATH]
```

Der Befehl lädt das ausgewählte Profil, prüft die gespeicherte Sitzung, ruft
einen Snapshot ab, validiert dessen Graphen und importiert ihn in einer
SQLite-Transaktion.

Abfolge der Abfragen:

1. Gültige gespeicherte Sitzung: eine Snapshot-Abfrage.
2. Keine gespeicherte Sitzung: Login und danach eine Snapshot-Abfrage.
3. Abgelaufene gespeicherte Sitzung: fehlgeschlagener Leseversuch, genau ein
   Login und danach genau ein abschließender Leseversuch.

Es gibt keine vierte Abfrage, keine allgemeine Wiederholungsschleife und keinen
teilweisen Datenbankimport. Fehler bei Anmeldung oder HTTP, zu große Antworten,
unbekannte Schemas und fehlerhafte Querverweise führen zu einem Exit-Code
ungleich null.

### Admin-Synchronisation und Erkennung

```bash
myyolo sync admin \
  [--profile NAME] [--db PATH] \
  [--delay 2s] [--request-budget 5]

myyolo discover admin \
  [--profile NAME] [--db PATH] \
  [--delay 2s] [--request-budget 10]
```

`sync admin` liest nur die Startseite der Anwesenheit. `discover admin` liest
die sechs dokumentierten Aggregat- und Listenseiten aus der
[Funktionsübersicht](docs/capabilities.md). Beide Befehle arbeiten seriell.
Die CLI lehnt Abstände unter zwei Sekunden und Budgets oberhalb der festen
Grenzen ab. Mit einer gültigen Sitzung benötigt die Erkennung einschließlich
Sitzungsprüfung sieben Abfragen. Bei einer abgelaufenen Sitzung sind es genau
zehn.

HTTP 429, CAPTCHA, eine unerwartete Weiterleitung, ein fehlgeschlagener Login,
ein ausgeschöpftes Abfragebudget oder eine Schemaänderung beenden den Befehl
sofort. Die Erkennung importiert erst dann Daten, wenn alle sechs Serverabfragen
erfolgreich waren.

### Typisierter Lesekatalog und Datensammlung

`catalog` arbeitet ausschließlich lokal und zeigt alle 99 klassifizierten
Lesefunktionen mit Gruppe, Sensibilität, HTTP-Methode, statischem Pfad und
zulässigen Filtern:

```bash
myyolo catalog [--group GROUP] [--format table|json|csv]
myyolo collect CAPABILITY [filters] \
  [--profile NAME] [--db PATH] \
  [--delay 2s] [--request-budget 5] \
  [--dry-run] [--format table|json|csv]
```

Die sechs Gruppen heißen `analytics`, `courses`, `members`, `prescriptions`,
`compliance` und `financial`. Nutze `catalog`, statt Pfade oder Formularfelder
zu erraten. `collect` erlaubt genau eine Funktion. Einen Massen- oder
Platzhaltermodus gibt es nicht. Folgende typisierte Filter sind zulässig:

| Schalter | Prüfung |
|---|---|
| `--from`, `--to`, `--date` | `YYYY-MM-DD`, für die Quelle normalisiert; Bereiche höchstens 366 Tage |
| `--year` | 2000 bis 2100 |
| `--week-from`, `--week-to` | 1 bis 53 |
| `--threshold` | 0 bis 100000 |
| `--member-id`, `--course-id`, `--referrer-id` | Positive numerische Kennung |
| `--planner`, `--population` | Funktionsspezifischer Auswahlwert |
| `--search` | 1 bis 128 Zeichen, keine Steuerzeichen |
| `--ik` | Genau neun Ziffern |

Nicht benötigte Filter werden abgelehnt. `--dry-run` validiert den vollständigen
Befehl, ohne Zugangsdaten zu laden, SQLite zu öffnen oder HTTP-Abfragen
auszuführen. Übergebene Werte werden in der Ausgabe bewusst weggelassen. Jede
echte Sammlung:

1. prüft Funktion, Parameter, Datenumfang, Abstand und Budget lokal;
2. prüft eine gespeicherte Sitzung oder führt genau einen begrenzten Login aus;
3. ruft genau eine Funktion ab;
4. speichert die strukturierte Beobachtung unter `capability:NAME`;
5. gibt nur Importzahlen und einen Schema-Fingerabdruck aus, niemals
   Datensatzwerte.

Beispiele:

```bash
# Aggregierte historische Reports
myyolo collect attendance-monthly --profile point
myyolo collect studio-hourly-load \
  --from 2026-07-01 --to 2026-07-31 --profile point

# Personenbezogene und Reha-Abfragen brauchen explizite lokale Freigaben
myyolo collect member-checkins --member-id 123 \
  --include-personal-data --profile point
myyolo collect member-reha-history --member-id 123 \
  --include-personal-data --include-health-data --profile point

# Finanzabfragen bleiben von normalen Sammlungen getrennt
myyolo collect digital-billing-complete-archive --ik 123456789 \
  --include-personal-data --include-financial-data --profile point
```

Die oben verwendeten Mitglieds-IDs und Filterwerte sind Platzhalter. Echte
Zugangsdaten, Gesundheits- oder Mitgliederdaten gehören weder in die
Shell-Historie noch in Issues, Logs oder öffentliche Repositories. Der
vollständige Katalog steht unter
[`docs/read-catalog.md`](docs/read-catalog.md).

### Datenbankbefehle

```bash
myyolo db init [--db PATH]
myyolo db status [--format table|json|csv] [--db PATH]
myyolo doctor [--profile NAME] [--db PATH]
myyolo import mysign --file PATH [--db PATH]
```

`db init` erstellt oder migriert eine leere lokale Datenbank. `import mysign`
liest eine lokale JSON-Antwort von `GetListData` und nutzt dieselbe Validierung
und denselben transaktionalen Importweg wie der Live-Sync. Der Offline-Import
ist für Entwicklung und Wiederherstellung gedacht. Rohe Live-Antworten sollten
normalerweise nicht aufbewahrt werden.

`db status` führt den SQLite-`quick_check` aus und gibt Schemaversion,
Quellzahlen und Zeitpunkte der letzten erfolgreichen Synchronisation zurück.
`doctor` ergänzt eine rein lokale Prüfung des Schlüsselbunds und testet keine
Sitzung im Quellsystem.

### Befehlsreferenz für Reports

Alle Reports lesen ausschließlich aus SQLite:

```bash
myyolo report REPORT [--db PATH] [--format table|json|csv]
```

| Reportbefehl | Gruppierung | Felder |
|---|---|---|
| `summary` | Gesamte Datenbank | Mitglieder, Termine, Anwesenheiten, Teilnahmen, Unterschriften, Stornos, fehlende Unterschriften, No-Shows, eindeutige Teilnehmende und letzter Sync |
| `courses` | Kursbezeichnung | Termine, Buchungen und alle Anwesenheitskennzahlen |
| `days` | Kalendertag | Termine, Buchungen und alle Anwesenheitskennzahlen |
| `hours` | Startstunde | Termine, Buchungen und alle Anwesenheitskennzahlen |
| `sessions` | Kurstermin | Datum, Zeit, Kurs, Raum und alle Anwesenheitskennzahlen |
| `members` | Mitglied | Quell-ID, Mitgliedsnummer, Name und alle Anwesenheitskennzahlen |
| `admin-capabilities` | Admin-Route und Tabelle | Route, Titel, Überschriften, Schema-Fingerabdruck, Zeilenzahl und Beobachtungszeit |
| `admin-reha-hours` | Reha-Zeitfenster | Teilnehmende, Dauer und Teilnehmerminuten |
| `admin-course-months` | Kalendermonat | Teilnehmende und Kurse mit numerischen Werten |
| `admin-missing-signatures` | Aktuelles Aggregat | Mitgliederzeilen und angezeigte Anzahl fehlender Unterschriften |
| `admin-reha-attendance` | Mitgliederzeile | Aktuelle Details zur Reha-Anwesenheit |
| `admin-missing-signature-members` | Mitgliederzeile | Aktuelle Details zu fehlenden Unterschriften |
| `admin-records` | Route und Tabellenzeile | Allgemeine strukturierte Werte aktueller oder historischer Beobachtungen |
| `capability CAPABILITY` | Neueste Beobachtung einer typisierten Funktion | Allgemeine strukturierte Zeilenwerte, durch Sensibilitätsfreigaben geschützt |

Mitglieder- und Admin-Detailreports können personenbezogene Daten enthalten
und benötigen deshalb die explizite Freigabe:

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

Ein ungefilterter `admin-records`-Report kann alle Sensibilitätsklassen mischen
und benötigt deshalb alle drei Datenfreigaben. Eine als gesundheitlich oder
finanziell klassifizierte Route braucht den jeweils passenden zusätzlichen
Schalter.

mySIGN-Reports akzeptieren `--as-of RFC3339`. Damit werden die Grenzen zwischen
offen und No-Show reproduzierbar. Admin-Reports verwenden immer die neueste
vollständige Beobachtung ihrer Route.

Ausgabeformate:

- `table` ist das menschenlesbare Standardformat.
- `json` eignet sich für Skripte und Weiterverarbeitung.
- `csv` eignet sich für lokale Tabellenanalysen.

Ausgaben können normal umgeleitet werden. Dateien auf Mitgliedsebene enthalten
jedoch personenbezogene Daten:

```bash
myyolo report courses --format csv > course-report.csv
myyolo report summary --format json > summary.json
```

## Definitionen der Kennzahlen

Die Reports verwenden die gespeicherten mySIGN-Schalter. Aus einem
Kurszeitfenster wird keine körperliche Anwesenheit im Studio abgeleitet.

| Kennzahl | Definition |
|---|---|
| `bookings` | Gespeicherte Beziehungen zwischen Anwesenheit, Kurs und Mitglied |
| `attended` | `Teilgenommen = true` |
| `signed` | `HatUnterschrift = true` |
| `cancelled` | `Storniert = true` |
| `missing_signatures` | Teilgenommen, nicht unterschrieben und nicht storniert |
| `no_shows` | Nicht teilgenommen, nicht storniert und Kursende liegt vor `as-of` |
| `pending` | Nicht teilgenommen, nicht storniert und Kursende ist aktuell, zukünftig oder unbekannt |
| `distinct_participants` | Eindeutige myYOLO-Mitglieds-IDs in den Anwesenheitszeilen |

Das sind technische Definitionen. Prüfe sie gegen die operative Bedeutung in
deiner Organisation, bevor du sie als Abrechnungs-, Compliance- oder
Management-Kennzahlen verwendest.

## Lokale Historie und Idempotenz

mySIGN liefert einen rollierenden Snapshot statt eines vollständigen
historischen Exports. Jeder Sync aktualisiert Datensätze anhand ihrer stabilen
Quell-IDs:

- bestehende Datensätze werden aktualisiert;
- wiederholte Importe erzeugen keine Duplikate;
- ältere Zeilen bleiben in SQLite, wenn sie aus einem späteren Snapshot fallen;
- jeder Importversuch erhält einen Prüfdatensatz unter `sync_runs`;
- fehlgeschlagene Importe werden zurückgerollt und als fehlgeschlagen markiert.

Damit wird die lokale Historie mit der Zeit aussagekräftiger, während
Reportbefehle ohne Serverabfragen auskommen. Zeiträume, die nie erfasst wurden,
lassen sich nicht nachträglich rekonstruieren.

Admin-Erkennung und typisierte Sammlung speichern Schema-, Herkunfts- und
unterschiedliche strukturierte Zeilenversionen. Funktionsreports wählen nur
Zeilen der neuesten vollständigen Beobachtung. Wiederholtes Sammeln ist damit
idempotent und alte Versionen verfälschen die aktuelle Ansicht nicht.
Historische Versionen bleiben für lokale Längsschnittanalysen in SQLite.

## Mehrere Zugänge und Datenbanken

Zugangsprofile bleiben im Schlüsselbund des Betriebssystems getrennt. Die
Datenbankauswahl ist davon unabhängig. Wähle deshalb pro Konto eine eigene
Datenbank, wenn Datensätze nicht gemischt werden dürfen:

```bash
myyolo sync --profile studio-a --db ~/.local/share/myyolo-cli/studio-a.sqlite
myyolo sync --profile studio-b --db ~/.local/share/myyolo-cli/studio-b.sqlite

myyolo report summary --db ~/.local/share/myyolo-cli/studio-a.sqlite
```

Verwende für unabhängige Profile nicht dieselbe Datenbank, außer die
Zusammenführung ist ausdrücklich beabsichtigt und autorisiert.

## Datenmodell

| Tabelle | Inhalt |
|---|---|
| `members` | Quellnamespace, Mitglieds-ID, Mitgliedsnummer und Name |
| `course_sessions` | Quellnamespace, Datum, Kurs, Raum, Zeitfenster, ausgewertetes UTC-Ende und Quellzahlen |
| `attendance` | Quellnamespace, Verknüpfung von Mitglied, Termin und Verordnung sowie Anwesenheitsschalter |
| `prescriptions` | Quellnamespace, Behandlung sowie Zähler für Wochenbehandlungen und Besuche |
| `admin_capabilities` | Exakte Route, Seiten- und Tabellenmetadaten, Schema-Fingerabdruck und letzte Beobachtung |
| `admin_records` | Exakte Route und Tabelle, deterministischer Zeilenhash, strukturierte Werte und Beobachtungshistorie |
| `sync_runs` | Quelle, normalisierter Schema-Fingerabdruck, Zeitpunkte, Status und Zeilenzahlen |

Rohe HTTP-Antworten, Passwörter, Cookies und rotierende Abfragetokens werden
nicht in SQLite gespeichert.

## Vollständiger Befehlsindex

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

## Verhalten nach Betriebssystem

- macOS nutzt die Schlüsselbundverwaltung.
- Windows nutzt die Anmeldeinformationsverwaltung.
- Linux nutzt Secret Service über den Desktop-Schlüsselbund und die
  D-Bus-Sitzung.
- SQLite-Dateien erhalten auf POSIX-Systemen den Modus `0600`. Unter Windows
  sollten Benutzerprofil und Datenbankverzeichnis durch passende
  Kontoberechtigungen geschützt werden.
- Die CLI besitzt keine Telemetrie, keinen Cloud-Speicher, keinen
  Hintergrunddienst und keine Update-Signale.

## Fehlerbehebung

### `profile "NAME" is not configured`

Führe zuerst `myyolo auth login --profile NAME ...` aus und prüfe das Profil
mit `auth status`.

### Schlüsselbundfehler unter Linux

Stelle sicher, dass ein Secret-Service-Anbieter wie GNOME Keyring oder KWallet
und eine D-Bus-Benutzersitzung verfügbar sind. Auf Servern ohne grafische
Oberfläche fehlt das häufig standardmäßig.

### Anmeldung fehlgeschlagen

Prüfe Partnernummer, Benutzername und Passwort. Die CLI speichert abgelehnte
Zugangsdaten nicht und wiederholt ungültige Logins nicht in einer Schleife.

### Sitzung abgelaufen

Eine normale Prüfung, Synchronisation oder Erkennung behandelt das einmal
automatisch. Meldet der abschließende Leseversuch weiterhin eine abgelaufene
Sitzung, stoppt der Befehl. Führe `auth login` bewusst erneut aus, statt den
Befehl in einer Schleife zu wiederholen.

### Rate Limit oder CAPTCHA

Die CLI stoppt ohne Wiederholung. Verringere den Abstand nicht, wechsle keine
Identitäten und automatisiere nicht um die Sperre herum. Warte auf das vom
Betreiber oder Anbieter freigegebene Zeitfenster und starte dann bewusst neu.

### Antwortschema geändert

Der Parser bricht sicher ab, damit geänderte oder unvollständige
Anwesenheitsfelder Reports nicht unbemerkt verfälschen. Eröffne ein Issue mit
einem vollständig synthetischen Beispiel. Hänge niemals die Live-Antwort an.

### Datenbank ist gesperrt

Führe keine überlappenden Synchronisationen gegen dieselbe Datenbank aus. Warte
auf das Ende des anderen Prozesses oder verwende getrennte Datenbankpfade.

### Wo liegen meine Daten?

Der Standardpfad lautet `~/.local/share/myyolo-cli/myyolo.sqlite`. Verwende
einen expliziten `--db`-Pfad, wenn Portabilität wichtig ist.

## Offline-Entwicklung

```bash
myyolo import mysign \
  --file ./internal/mysign/testdata/get_list_data.synthetic.json \
  --db ./data/synthetic.sqlite
myyolo report summary --db ./data/synthetic.sqlite
```

Es werden ausschließlich synthetische Testdaten eingecheckt. Hänge niemals
Zugangsdaten, Tokens, Screenshots, HAR-Dateien, Datenbanken oder
Mitgliederexporte an ein Issue.

## Entwicklungs- und Release-Prüfungen

```bash
make format  # Go-Quellcode formatieren
make test    # Testsuite mit Race-Erkennung
make check   # Format-, Modul-, Vet-, Race- und Build-Prüfungen
make build   # lokale Binärdatei unter bin/myyolo
```

Die Tests decken striktes JSON- und HTML-Parsing, Migration und Idempotenz,
Auswahl des neuesten Snapshots, typisierte Funktionsfilter, Redaktionen im
Trockenlauf, Freigaben für personenbezogene, gesundheitliche und finanzielle Daten,
reproduzierbare No-Show-Grenzen, Dateirechte, Ausgabeformate, getrennte
Schlüsselbundsitzungen, exakte Routen-Allowlisten, Abstände und Budgets,
Abbrüche bei Rate Limits und CAPTCHA, Größen- und Inhaltstypgrenzen,
Schemaänderungen, gespeicherte Sitzungen und den genau einmaligen erneuten Login
ab.

Die CI führt dieselben Qualitätsprüfungen plus `govulncheck` aus. Getaggte
`v*`-Pushes bauen mit GoReleaser Archive samt Prüfsummen für macOS, Linux und
Windows auf AMD64 und ARM64.

Der eingecheckte Printing-Press-Vertrag kann ohne Zugangsdaten und ohne
Live-Traffic geprüft werden:

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

Der generierte Client dient nur als Referenz. Die produktive Laufzeit bleibt
handgeschrieben, weil ihre exakte Allowlist sowie die Regeln für Schlüsselbund,
Abfragebudget und erneuten Login strenger sind als der generierte Transport.

## Abgrenzung

Diese Version enthält keine Magicline-Integration, keinen Scheduler, Server,
Cloud-Upload, gehostetes Dashboard, MCP und keine schreibenden myYOLO-Befehle.
Ein lokales Dashboard ist ein eigener Schritt und sollte ausschließlich SQLite
lesen. Gleiche Namen sind kein sicherer systemübergreifender Schlüssel, weil
sie sich ändern oder mehrfach vorkommen können. Eine spätere
Magicline-Integration sollte eine stabile Mitgliedsnummer oder Quellkennung
verwenden und braucht eine eigene Datenschutzprüfung.

Weitere Details stehen in der [Architektur](docs/architecture.md), der
[Sicherheitsrichtlinie](SECURITY.md) und im
[Printing-Press-Vertrag](printing-press/contract.md).
