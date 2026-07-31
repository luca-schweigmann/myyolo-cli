# Capability-Map

Diese Map trennt, was die CLI verifiziert hat, von dem, was lediglich als Link
oder mögliche künftige Integration erscheint. Sie enthält keine Kontowerte,
Mitgliederdatensätze, erfassten Antworten, Cookies oder Live-Zähler.

Der aktuelle typisierte Katalog enthält 99 schreibgeschützte Capabilities über
Analytics, Kurse, Mitglieder, Rezepte, Compliance und isolierte Finanz-Reads.
Siehe [den vollständigen Read-Katalog](read-catalog.md) oder führe
`myyolo catalog --format json` aus.

## Status-Vokabular

- **Supported**: implementiert, durch synthetische Tests abgedeckt und gegen die
  autorisierte schreibgeschützte Oberfläche verifiziert.
- **Structured**: in private SQLite mit stabilem Schema-Fingerprint erfasst,
  aber noch nicht in jeden möglichen domänenspezifischen Report übernommen.
- **Classified**: im typisierten Allowlist mit synthetischer Validierung
  implementiert; die Quelle kann auf einer konkreten Installation weiterhin
  keine Zeilen, ein geändertes Schema oder einen nicht-HTML-Export liefern; in
  dem Fall scheitert die Collection fail-closed.
- **Linked candidate**: von einer autorisierten Navigationsseite sichtbar, aber
  von der CLI weder auf der Allowlist noch abgerufen.
- **Out of scope**: in der schreibgeschützten CLI bewusst nicht verfügbar.

## mySIGN

| Capability | Status | Lokale Daten / Befehl |
|---|---|---|
| Partnerbezogenes Login | Supported | `auth login`, getrennte mySIGN-Keyring-Session |
| Wiederherstellung abgelaufener Session | Supported | ein Login und ein finaler Read, maximal drei Anfragen |
| Kursvorkommen | Supported | `course_sessions`; `report sessions`, `courses`, `days`, `hours` |
| Mitgliederreferenzen | Supported | `members`; PII-geschütztes `report members` |
| Anwesenheitsflags | Supported | attended, signed, manually signed, cancelled |
| Rezepte | Supported | treatment, weekly-treatment, visit, remaining-unit fields |
| Fehlende Unterschriften | Supported | attended, unsigned, not cancelled |
| No-Shows | Supported | nur nach Kursende; zukünftige/laufende/unbekannt-endende Zeilen sind pending |
| Physische Aufenthaltsdauer in der Einrichtung | Not exposed | Kursfenster dürfen nicht als physische Check-in-Dauer dargestellt werden |

mySIGN liefert einen rollierenden Snapshot. Die CLI baut aus wiederholten
Erfassungen lokale Historie auf, kann aber Zeiträume, die nie erfasst wurden,
nicht rekonstruieren.

## Klassische myYOLO-Administration

Der ursprüngliche Sechs-Seiten-Befehl `discover admin` bleibt ein begrenzter
Kompatibilitäts- und Schema-Discovery-Pfad. Neue operative Reads nutzen
`myyolo collect CAPABILITY`, der eine typisierte Capability und ihre Parameter
validiert, höchstens fünf Anfragen inklusive Session-Wiederherstellung ausführt
und das Ergebnis unter einem parameterfreien Capability-Key speichert.

Alle Routen unten sind exakte Host-, Methoden- und Pfad-GET-Reads. Login
nutzt den beobachteten Form-POST plus die zwei beobachteten Redirect-Hops.
Discovery übermittelt niemals Mitglieder-IDs, Signaturen, Edits,
Abrechnungsaktionen oder Datums-Suchformulare.

| Capability | Route | Status | Lokaler Report |
|---|---|---|---|
| Anwesenheits-Navigation/Session-Probe | `/start_Anwesenheit.asp` | Supported | `sync admin`, `auth check --source admin` |
| Seite „aktuell anwesend“ | `/Anwesenheit/Anwesend_Aktuell_Liste.asp` | Structured | `report admin-capabilities`; PII-geschützte generische Datensätze |
| Seite „heute anwesend“ | `/Anwesenheit/Anwesend_Heute_Liste.asp` | Structured | `report admin-capabilities`; PII-geschützte generische Datensätze |
| Seite Anwesenheit nach Datum | `/Anwesenheit/Anwesend_Datum_Liste.asp` | Structured | nur GET-Snapshot; POST-Datumsfilter wird nicht übermittelt |
| Reha-Anwesenheit/Zeitfenster | `/Anwesenheit/Reha_Anwesend_Datum_Liste.asp` | Supported | `admin-reha-hours`; PII-geschütztes `admin-reha-attendance` |
| Monatliche Kursteilnahme | `/Statistiken/Kursplaner/Kurs_Teilnehmer_anwesend_monatlich.asp` | Supported | `admin-course-months` |
| Fehlende Reha-Unterschriften | `/Vertrag_Reha/Unterschrift/Fehlende_Reha_Unterschriften_Liste.asp` | Supported | aggregiert `admin-missing-signatures`; PII-geschütztes Mitgliederdetail |

Der Reha-Zeitfenster-Report liefert geplante oder erfasste Fenster, die diese
Seite exponiert. Er berechnet Minuten pro Fenster und Teilnehmer-Minuten. Er
behauptet nicht, dass diese Fenster Magicline-Ein-/Austrittsereignissen
entsprechen.

### Typisierte Capabilities

Der klassifizierte Katalog ergänzt 99 einzeln wählbare Reads. Er umfasst
historische aggregierte Anwesenheit, Kurs- und Teilnehmer-Controlling,
personenbezogene Check-in-/Reha-Historie, begrenzte Listen und Exporte,
Billing-Readiness-Checks und separat freigeschaltete Finanzarchive. Er crawlt
keine Navigation und fächert nicht automatisch aus Suchergebnissen auf. Jeder
Live-Lauf wählt genau eine benannte Capability.

Katalogmitgliedschaft bedeutet, dass Route und Parameterform klassifiziert sind;
sie bedeutet nicht, dass alle 99 Routen massenhaft gegen ein Live-Konto getestet
wurden. Das würde der Server-Last-Richtlinie widersprechen. Repräsentative
Live-Verifikation sollte klein und vom Betreiber gesteuert bleiben.

### Bewusst blockiert

- Logout-Routen, weil Remote-Logout eine Zustandsänderung ist und lokales
  Entfernen der Session ausreicht;
- Massenschleifen über Mitgliederdetails oder automatisches Traversieren aus
  einer Mitgliederliste;
- Login-Zeit-, Signatur-, Anwesenheits-, Vertrags-, Abrechnungs-, Notiz- oder
  Kurs-Edits;
- jeder Form-POST, der nicht explizit als semantischer Read mit typisierten
  Feldern klassifiziert ist;
- breites Crawling, geratene Pfade, automatische Formular-Submission, Retries
  nach Schema-Drift, CAPTCHA-Handling oder Rate-Limit-Bypass.

## Magicline-Nahtstelle

In dieser Version gibt es keinen Magicline-Transport. Identische Namen sind kein
sicherer Join-Key. Eine künftige Integration sollte stabile IDs oder
Mitgliedsnummern bevorzugen, Quell-Namespaces beibehalten, Match-Konfidenz und
Provenance erfassen und eine getrennte Datenschutz- und Abstimmungsprüfung
erfordern.

## Capability-Discovery-Budget

`myyolo discover admin` liest die sechs supported/structured Aggregatseiten
seriell. Mit gültiger gecachter Session nutzt es sieben Anfragen einschließlich
Session-Probe. Mit abgelaufener Session genau zehn: ein fehlgeschlagener Probe,
drei Login-/Redirect-Anfragen und sechs Reads. Kein partielles Remote-Ergebnis
wird importiert, wenn eine Seite fehlschlägt.

`myyolo collect` ist der sicherere operative Standard: ein Session-Probe plus ein
gewählter Read bei gültigem Cache, oder ein begrenztes Login plus ein gewählter
Read. Die gemeinsame harte Obergrenze liegt bei fünf Anfragen, die minimale
Verzögerung zwischen Anfragen bei zwei Sekunden. Es gibt kein `collect all`.
