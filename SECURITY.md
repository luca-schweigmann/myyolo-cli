# Sicherheitsrichtlinie

## Unterstützte Versionen

Sicherheitskorrekturen gibt es für die neueste getaggte Version.

## Schwachstelle melden

Öffne kein öffentliches Issue mit Schwachstelle, Zugangsdaten, Token, Cookie,
Datenbank, HAR-Datei, Mitgliederidentität oder Anwesenheitsdatensatz. Nutze
den privaten Security-Advisory-Ablauf von GitHub für dieses Repository.

Füge einen minimalen synthetischen Reproduktionsschritt, die betroffene
Version und die erwartete Auswirkung bei. Ersetze alle Konten- und
Personendaten durch erfundene Werte.

## Betreiberpflichten

- Nutze nur ein Konto und Daten, auf die du zugreifen darfst.
- Führe die CLI auf einem vertrauenswürdigen, gepatchten Host mit
  Festplattenverschlüsselung aus.
- Behandle die lokale Datenbank als gesundheitsnahe personenbezogene Daten,
  auch wenn normalerweise nur aggregierte Berichte angezeigt werden.
- Sichere und verschlüssele Backups der lokalen SQLite-Datenbank.
- Halte exportierte CSV-/JSON-Berichte aus gemeinsamen Ordnern und der
  Versionskontrolle heraus.
- Schwäche die zweisekündige Admin-Verzögerung nicht und erhöhe die
  kompilierten Anfrage-Obergrenzen nicht. Stoppe bei Rate-Limits, CAPTCHA,
  Auth-Anomalien oder Schemaabweichungen.
- Starte keine überlappenden Sammelprozesse. Bevorzuge genau eine ausgewählte
  `collect`-Operation und danach beliebig viele lokale Berichte.
- Aktiviere personenbezogene, gesundheitliche oder finanzielle
  Sammel-/Berichtsflags nur für den freigegebenen Zweck und halte die
  finanzielle Sammlung von Routinejobs getrennt.
- Entferne ein lokales Profil mit `myyolo auth logout --profile NAME`, wenn der
  Zugang endet. Das entfernt Zugangsdaten sowie die getrennten
  mySIGN-/Admin-Sitzungen aus dem Schlüsselbund, nicht das entfernte Konto.

## Aufbewahrung und Löschung lokaler Daten

Die Standard-Datenbank ist
`~/.local/share/myyolo-cli/myyolo.sqlite`. `MYYOLO_DB_PATH` oder `--db` können
einen anderen Pfad wählen. Berichte können zusätzliche Dateien anlegen, wohin
die Shell-Ausgabe umgeleitet wird.

Stoppe vor dem Löschen einer Datenbank alle CLI-Prozesse, die sie nutzen.
Lösche die exakte Datenbankdatei und, falls daneben vorhanden, die Begleiter
`-wal` und `-shm`. Lösche exportierte CSV-/JSON-Dateien und geschützte Backups
getrennt. Dieser Vorgang ist lokal und unumkehrbar; er löscht keinen Datensatz
im Quellsystem.

Wähle eine dokumentierte Aufbewahrungsfrist, die zur Freigabe und Rechtsgrundlage
der Organisation passt. Dieses Projekt stellt keinen Hintergrundscheduler und
keinen automatischen Aufbewahrungsjob bereit.

Die CLI hat bewusst keine Telemetrie, keinen Update-Beacon, keinen
Cloud-Speicher, keinen Hintergrunddienst, kein Remote-Logout und keine
Remote-Schreibroute.
