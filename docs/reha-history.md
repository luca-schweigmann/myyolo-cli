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

## Native Admin-Kursdetails

```sh
myyolo report admin-reha-sessions --db /private/admin.sqlite \
  --from 2026-07-01 --to 2026-09-16
```

Dieser ausschließlich lokale Report (`admin-reha-sessions.v1`) liest typisierte
Kursdetail-Beobachtungen aus `admin_observations`. Er gibt pseudonyme native
Kurs-/Termin-Schlüssel, Datum, optionalen originalen Planermodus, Beobachtungszeit,
`registered`, `attendance_marked`, `signed_attendance` und `participated` aus.
Die native Terminidentität stammt aus dem versteckten Buchungsschlüssel und
Datum der Detailseite; die Kursserienidentität aus dem archivierten
Kurs-ID/Datum/Planermodus-Request. Fehlt der Planermodus im Quelllink, bleibt er fehlen; er wird nie als A/B
hinzugedichtet. Der Report behauptet keinen Standort, den das Admin-Archiv nicht
selbst belegt. Die Integration muss den Account-/Standort-Scope separat erhalten.

`registered` zählt die validierten, lückenlos nummerierten Teilnehmerzeilen.
`attendance_marked` erfordert genau `anwesend_haken.png` in der AW-Spalte.
`participated` ist für den Management-Vertrag genau AW-Markierung UND
`unterschrift_gruen.png` in derselben Teilnehmerzeile. Die Quellenlegende benennt
das grüne Unterschriftensymbol als vorhandene, ansehbare Unterschrift; das rote
als fehlende Unterschrift. Andere grüne/rote Symbole werden nicht als Anwesenheit
interpretiert. `signed_attendance` ist derselbe fachliche Wert mit technischem
Namen. `cancelled` und `capacity` bleiben unbekannt; Stornierungen werden nicht
geraten oder von der belegten Teilnahme abgezogen.

Unbekannte Statusbilder, Steuerelemente, geänderte Tabellenüberschriften,
nicht lückenlose Zeilennummern, doppelte Teilnehmerreferenzen und mehrdeutige
Tabellen werden abgelehnt. Doppelte reine Links zählen nicht doppelt. Leere,
schematisch vollständige Tabellen sind beobachtete 0; fehlende typisierte
Altarchive sind ein sichtbarer Fehler. `past_local_day` wird ausschließlich für
Tage vor dem lokalen As-of-Tag gesetzt; heute und später bleiben `provisional`.

Der Report zählt nur tatsächlich archivierte native Termine und verbindet sie
nicht über Namen/Uhrzeiten mit mySIGN. Ein erwarteter Gesamtkalender und fehlende
Terminzahlen bleiben `null`. Die Detailseite kann mehr Daten darstellen als der
bisherige reine HTML-Textparser: Bildzustände sind jetzt explizit ausgewertet,
Personen- und Unterschriftsmaterial wird nicht exportiert.

## Historische Admin-Klassifikationen für Diagramme

```sh
myyolo report admin-reha-ranges --db /private/admin.sqlite \
  --from 2026-07-01 --to 2026-09-15
```

Der lokale Vertrag `admin-reha-ranges.v1` kombiniert drei vollständig validierte
Range-Beobachtungen je identischem Quellzeitraum: `course-attended`,
`course-not-attended`, `course-cancelled`. Parser prüfen die angezeigten
Formulardaten `von`/`bis`, exakte Tabellenüberschriften, lückenlose Zeilennummern,
Datumsgrenzen und native Buchungsreferenzen. Pagination-Hinweise, Drift und
fehlende Sammlungen werden abgelehnt. Eine ausdrücklich leere vollständige
Klassifikation ist 0; eine fehlende Sammlung ist kein 0-Wert.

Die Range-Referenz `KursID` bezeichnet **eine Buchung/einen Termin**, keine
wiederkehrende Kursserie. Der öffentliche Schlüssel ist ein namensraumgetrennter
Hash dieser nativen ID. Gleichnamige oder zeitgleiche Termine bleiben getrennt.
Eine Kursdetailseite kann über ihren versteckten nativen `Kurs`-Buchungsschlüssel
und `Datum` die exakte Beziehung zu ihrer angefragten Kursserien-ID belegen.
Nur diese ausdrückliche Beziehung erlaubt das optionale `stable_course_id` und
`signed_participated`; Label oder Uhrzeit erlauben keine solche Zuordnung.

- `registered = attended + not_attended` aus beiden vollständigen Klassifikationen.
- `participated = attended`, Status `admin_attendance_classification`. Anzeige:
  **Teilgenommen**, Erklärung: „von myYOLO als anwesend geführt“.
- `cancelled` bleibt separat und wird nicht auf Eintragungen aufgeschlagen.
- `signed_participated` bleibt null, bis derselbe native Termin in einer
  Detailbeobachtung AW plus grüne Unterschrift derselben Zeile belegt.
- `capacity` bleibt null; es wird keine heutige Kapazität historisch zurückgerechnet.
- `registered_observed_at` enthält beide Quellzeitpunkte; Anwesenheit, Stornos
  und Signaturen erhalten eigene Beobachtungszeitpunkte.

Der Report enthält Label, Datum, Start-/Endzeit, alle oben genannten Werte und
Durchschnitte je beobachtetem abgeschlossenem Termin im gewählten Zeitraum.
Heutige und künftige lokale Tage bleiben `provisional` und gehen nicht in diese
Durchschnitte ein. Echte 0-Werte gehen ein; unbekannte Termine werden nicht
hinzugedichtet. Die Beobachtung ist der heute gelesene Stand historischer
Termine, keine Rekonstruktion früherer Voranmeldestände.

Alle angefragten Kalendertage müssen durch vollständige Range-Tripel abgedeckt
sein. Das beweist die Vollständigkeit dieser Quellklassifikationen, aber nicht
ein autoritatives Kalenderinventar: komplett leere/geplante Termine, die keine
Route aufführt, bleiben unbekannt. `expected_sessions` und `missing_sessions`
bleiben deshalb null. `source_windows` weist die konkreten Datenfenster aus.
Kein Mitgliedsname, keine Mitglieds-/Verordnungs-ID, kein privater Link oder
roher Buchungsschlüssel gelangt in diesen Chart-Vertrag.

### Vollständiges Wocheninventar und leere Termine

Die source-advertised Kalendernavigation erlaubt GET auf den Wochenplaner mit
`Datum=TT.MM.JJJJ` und leerem `Raum` (= alle Räume). Der typisierte Parser verlangt
sieben lückenlose Tagesabschnitte Montag–Sonntag, exakte Header und native
Kurs-/Datum-/Planer-Referenzen. `Plätze - belegt = frei` wird validiert;
negative freie Plätze bei Überbelegung bleiben sichtbar und werden nicht gekappt.

Ein explizit mit `belegt=0` geführter Planertermin wird nur dann zusätzlich als
0/0-Termin ausgegeben, wenn sein exakt verlinkter Kursdetail-Beleg eine leere
validierte Liste und die native Buchungs-ID liefert. Diese Termine gehen in
die Durchschnittsdenominatoren ein. Positive Termine stammen weiterhin aus
beiden vollständigen Range-Klassifikationen; eine dort fehlende exakte
Buchungs-ID ist **unbekannt/Fehler**, niemals stillschweigend 0.

`expected_sessions` kann bei vollständig archiviertem Wocheninventar dessen
Terminzahl angeben. `missing_sessions` bleibt ohne vollständigen Einzel-ID-
Crosswalk unbekannt. Der Report weist diesen Unterschied ausdrücklich aus.
Planerwerte `planner_registered`, `planner_cancelled`, `planner_linked`,
`planner_free` und historische `capacity` werden nur über die explizite
Kursdetail-Beziehung zum selben nativen Termin ergänzt und erhalten ihren
separaten `planner_observed_at`. Kein Join erfolgt über Label, Zeit oder gleiche
Zählwerte. Dadurch sind die Kapazitäten der geprüften Nulltermine belegt;
Kapazitäten nicht einzeln zugeordneter positiver Termine bleiben unbekannt.
