# Printing-Press-Vertrag

Die Specs dokumentieren die beiden beobachteten Quellverträge:

- `myyolo-pp-spec.yaml`: mySIGN-JSON-Login und rotierender Snapshot.
- `myyolo-admin-pp-spec.yaml`: klassisches ASP-Login und repräsentative
  HTML-Lesezugriffe aus allen sechs Lese-Kataloggruppen.

Sie enthalten keine Zugangsdaten, Cookies, Tokens, Mitgliederdatensätze,
erfassten Antworten oder HAR-Dateien.

Printing Press dient hier als reproduzierbarer Vertrags- und Generator-Check,
nicht als Laufzeit-Sicherheitsgrenze. Version 4.22.1 stuft die beiden `POST`-
Operationen syntaktisch als create-ähnliche Befehle ein und kann nicht alle
dieser Runtime-Invarianten im generierten Client ausdrücken:

- `POST /Home/GetListData` ist semantisch schreibgeschützt;
- nur die beiden klassifizierten mySIGN-Kombinationen aus Host, Methode und
  Pfad dürfen den mySIGN-Transport verlassen;
- Passwörter gehören in den Schlüsselbund des Betriebssystems;
- ein mySIGN-Sync darf höchstens drei Anfragen stellen;
- eine abgelaufene Sitzung löst genau einen erneuten Login aus;
- eine Schemaänderung muss ohne erneute Authentifizierung stoppen;
- der Admin-Client muss Weiterleitungen manuell zählen, mindestens zwei
  Sekunden warten, ein Budget von maximal fünf Anfragen pro Fähigkeit teilen
  und niemals eine `collect all`-Operation bereitstellen;
- personenbezogene, gesundheitliche und finanzielle Fähigkeiten brauchen
  getrennte explizite Freigaben;
- mySIGN- und Admin-Sitzungen dürfen niemals denselben Schlüsselbund-Account
  oder Cookie-Jar teilen;
- alle anderen myYOLO-Routen sind gesperrt.

Die handgeschriebenen Pakete unter `internal/transport` und `internal/secrets`
bleiben deshalb maßgeblich. Ein generierter Baum darf zum Vergleich dienen,
wird aber bewusst weder versioniert noch gegen ein Live-Konto ausgeführt.

## Reproduzieren

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

Beide Dry-Runs müssen ohne Zugangsdaten und ohne Live-Datenverkehr parsen.

Die Specs sind bereinigte Interoperabilitäts-Fixtures. Die maßgebliche
vollständige Liste der 99 klassifizierten Runtime-Fähigkeiten wird lokal
exportiert mit:

```bash
myyolo catalog --format json
```
