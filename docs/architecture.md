# Architektur

Die CLI wandelt begrenzte, autorisierte Reads aus zwei verwandten Systemen in
dauerhafte lokale Historie um und führt alle Auswertungen gegen diese Historie
aus.

```text
ein Operator-Credential-Profil im OS-Schlüsselbund
                    |
          +---------+---------+
          |                   |
          v                   v
  mySIGN-JSON-Client     klassischer ASP-Admin-Client
  rotierendes Token      isolierter Cookie-Jar
  <=3 Anfragen           <=5 Sync / <=10 Discovery
  >=1s seriell           >=2s seriell
          |                   |
  strikter JSON-Graph    typisierter 99-Routen-Read-Katalog
                         begrenzter Leaf-Table-Parser
          |                   |
          +---------+---------+
                    v
          private SQLite-Datenbank
                    |
          nur lokale Reports
          Sensitivitätsdetails hinter expliziten Flags
```

## Vertrauensgrenzen

1. `internal/transport` ist das finale Gate für Host/Methode/Pfad/Query. Ein POST
   wird niemals allein anhand des Verbs als Write oder Read angenommen; jede
   exakte Route ist klassifiziert.
2. `internal/readcatalog` ist der typisierte Quellenvertrag für Collections
   mit genau einer Capability. Er validiert Daten, IDs, Wochen, Enums,
   Suchlänge, IK-Nummern, feste Query-Werte und Sensitivität vor dem
   Netzwerkzugriff.
3. `internal/secrets` verwaltet einen Credential-Envelope sowie getrennte
   `mysign`- und `admin`-Session-Envelopes im Plattform-Schlüsselbund.
   Geheimwerte gelangen niemals in Logs, Konfigurationsdateien,
   Kommandozeilenargumente oder SQLite.
4. `internal/mysign` parst einen strikten Graphen. Fehlende
   Anwesenheitsmetriken, doppelte Identifier und unbekannte Referenzen scheitern
   fail-closed.
5. `internal/admin` deaktiviert automatische Redirects, zählt jeden ASP-Auth-Hop,
   besitzt einen isolierten Cookie-Jar, dekodiert den deklarierten
   Antwort-Charset, lehnt CAPTCHA-/Rate-Limit-/Schema-Anomalien und nicht-HTML-
   Binärexporte ab und parst begrenzte Leaf-Tabellen.
6. `internal/store` importiert mySIGN-Graphen und strukturierte
   Admin-Observations. Aktuelle Admin-Reports verknüpfen nur die neueste
   Observation; ältere Zeilenversionen bleiben als lokale Historie verfügbar.
7. `internal/output` rendert lokale Tabelle, JSON oder CSV. Personen-,
   Gesundheits- und Finanzzeilen erfordern jeweils eigene explizite
   Data-Scope-Flags.

## mySIGN-Session-Zustandsmaschine

```text
gecachte Session?
  +-- ja  -> Read
  |          +-- Erfolg -> Token rotieren, persistieren, stop (1 Anfrage)
  |          +-- abgelaufen -> Login -> einmal lesen, stop (max. 3)
  |          +-- anderer Fehler/Schema-Drift -> stop
  +-- nein -> Login -> einmal lesen, stop (2 Anfragen)
```

Es gibt keine generische Retry-Schleife. Ein zweiter fehlgeschlagener Read löst
niemals ein weiteres Login aus. Gleichzeitige Aufrufer in einem Prozess werden
serialisiert. Die CLI plant sich nicht selbst; Nutzer oder ein externer
Scheduler steuern die Häufigkeit.

## Admin-Session-Zustandsmaschine

```text
gecachte Admin-Cookies?
  +-- ja  -> exakter Session-Probe
  |          +-- Erfolg -> angeforderte Seiten lesen
  |          +-- Redirect zur beobachteten unauthentifizierten Einstiegsseite
  |                        -> einmal Login -> einmal lesen
  |          +-- jedes andere Ergebnis -> stop
  +-- nein -> einmal Login -> einmal lesen
```

Der Login-Flow wird explizit gezählt:

```text
POST /Anmelden.asp?vw=
  -> GET /LoginHandler.asp
  -> GET /Start.asp (oder beobachteter Kleinbuchstaben-Alias)
```

`http.Client` folgt Redirects niemals automatisch. Jeder Hop durchläuft dieselbe
Allowlist, dieselbe Verzögerung und dasselbe gemeinsame Budget. Ein normaler
Admin-Sync nutzt höchstens fünf Anfragen; vollständige Discovery höchstens zehn,
einschließlich eines Probes bei abgelaufener Session. Eine typisierte Collection
liest genau eine Capability mit maximal fünf Anfragen. Alle Aufrufe innerhalb
einer Operation sind seriell und mindestens zwei Sekunden voneinander entfernt.

## Persistenz

- `members`: Quell-Namespace, Quell-ID, Mitgliedsnummer, lokale Anzeigeidentität.
- `course_sessions`: datierte Kursvorkommen, Raum und Zeit.
- `prescriptions`: lokale Rezept-Zähler.
- `attendance`: Mitglied/Session/Rezept-Verknüpfungen und Anwesenheitsflags.
- `admin_capabilities`: Route-/Capability-Key, bereinigte Schema-Metadaten und neueste Observation.
- `admin_records`: Route-/Tabellen-/Zeilen-Hash, strukturierte Werte und Observation-Range.
- `sync_runs`: Quell-Fingerprint, Status, Zeitstempel und aggregierte Zeilenzahlen.

mySIGN liefert einen rollierenden Snapshot. Zeilen bleiben lokal erhalten, wenn
sie später aus diesem Fenster verschwinden. Admin-Zeilen-Hashes bewahren
geänderte/entfernte Versionen, während aktuelle Reports auf die neueste
Capability-Observation filtern. SQLite- und WAL-Dateien haben Mode `0600`;
Betreiber sollten zusätzlich Vollplattenverschlüsselung, Host-Zugriffskontrollen
und verschlüsselte Backups nutzen.

## Printing Press

Zwei bereinigte Printing-Press-Specs dokumentieren den beobachteten mySIGN- und
einen repräsentativen Admin-Vertrag. Der vollständige ausführbare Admin-Vertrag
liegt im typisierten Go-Katalog und ist mit `myyolo catalog --format json`
exportierbar. Generierte Clients sind nicht die Runtime, weil der Generator
rotierende Tokens, Cookie-Isolation, manuelle Redirect-Budgets, semantische
Read-POSTs, Sensitivitäts-Gates oder fail-closed Schema-Grenzen nicht als einen
generierten Vertrag abbilden kann.

## Nicht-Ziele für Version 1

- keine Magicline-Integration und kein namensbasiertes Cross-System-Matching;
- kein Polling, Daemon, Cloud-Sync, gehostetes Dashboard oder MCP-Server;
- keine myYOLO-Writes, Signaturen, Kursänderungen, Dokumente oder
  Abrechnungsoperationen;
- kein CAPTCHA-Bypass, Browser-Fingerprinting, Proxy-Rotation oder
  Stealth-Verhalten;
- kein Crawling von Mitgliederdetails und keine Fan-out-Collection;
- keine unklassifizierte Query- oder Formular-Submission;
- keine Behauptung, dass Kurs-/Reha-Fenster der physischen Aufenthaltsdauer in
  der Einrichtung entsprechen.
