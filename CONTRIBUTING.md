# Mitwirken

Verwende Go 1.26.5 oder neuer und führe `make check` aus.

Pro Nutzerflow gibt es einen E2E-Test gegen die gebaute CLI mit synthetischen
Daten. Neue Features erhalten ihren Flow-Test. Unit-, Komponenten-, Property-
und Contract-Tests werden nicht ergänzt.

Die Netzwerkgrenze wahrt die exakte Allowlist aus Methode, Host, Pfad und Query,
die quellenspezifische Sitzungstrennung, das Abfragebudget, den Abstand und den
einmaligen erneuten Login. Füge keine
schreibende Serverroute, nicht klassifizierte Formularübermittlung,
automatisierte Mitglieder-Detailabfragen, CAPTCHA-Behandlung oder Umgehung von
Rate Limits hinzu. Jede typisierte Sammlung bleibt auf eine Funktion pro Lauf
und höchstens fünf Abfragen begrenzt.

Verwende ausschließlich synthetische Testdaten. Zugangsdaten, Tokens, Cookies,
rohe Antworten, HAR-Dateien, SQLite-Datenbanken, Screenshots oder Exporte mit
echten Personen dürfen weder eingecheckt noch angehängt werden.

Bei Änderungen an Printing Press müssen beide Verträge weiterhin erfolgreich
geprüft werden:

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
