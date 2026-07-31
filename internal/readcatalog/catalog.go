package readcatalog

import (
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
)

type Sensitivity string

const (
	Aggregate Sensitivity = "aggregate"
	Personal  Sensitivity = "personal"
	Health    Sensitivity = "health"
	Financial Sensitivity = "financial"
)

type Location string

const (
	Query Location = "query"
	Form  Location = "form"
)

type Kind string

const (
	Date           Kind = "date"
	PositiveID     Kind = "positive_id"
	NonNegativeInt Kind = "non_negative_int"
	Year           Kind = "year"
	Week           Kind = "week"
	Enum           Kind = "enum"
	SearchText     Kind = "search_text"
	IKNumber       Kind = "ik_number"
)

type Parameter struct {
	Name     string
	Remote   string
	Location Location
	Kind     Kind
	Required bool
	Fixed    string
	Allowed  []string
}

type Capability struct {
	Name        string      `json:"name"`
	Group       string      `json:"group"`
	Description string      `json:"description"`
	Method      string      `json:"method"`
	Path        string      `json:"path"`
	Sensitivity Sensitivity `json:"sensitivity"`
	Parameters  []Parameter `json:"-"`
}

type Request struct {
	Capability Capability
	Path       string
	Body       string
}

const maxDateRangeDays = 366

var (
	positiveIDPattern = regexp.MustCompile(`^[1-9][0-9]{0,11}$`)
	ikPattern         = regexp.MustCompile(`^[0-9]{9}$`)
)

func query(name, remote string, kind Kind, required bool) Parameter {
	return Parameter{Name: name, Remote: remote, Location: Query, Kind: kind, Required: required}
}

func form(name, remote string, kind Kind, required bool) Parameter {
	return Parameter{Name: name, Remote: remote, Location: Form, Kind: kind, Required: required}
}

func fixedQuery(remote, value string) Parameter {
	return Parameter{Remote: remote, Location: Query, Fixed: value, Required: true}
}

func enumQuery(name, remote string, required bool, allowed ...string) Parameter {
	return Parameter{
		Name: name, Remote: remote, Location: Query, Kind: Enum,
		Required: required, Allowed: allowed,
	}
}

func enumForm(name, remote string, required bool, allowed ...string) Parameter {
	return Parameter{
		Name: name, Remote: remote, Location: Form, Kind: Enum,
		Required: required, Allowed: allowed,
	}
}

func cap(
	name, group, description, method, path string,
	sensitivity Sensitivity,
	parameters ...Parameter,
) Capability {
	return Capability{
		Name:        name,
		Group:       group,
		Description: description,
		Method:      method,
		Path:        path,
		Sensitivity: sensitivity,
		Parameters:  parameters,
	}
}

var capabilities = []Capability{
	// Attendance and aggregate analytics.
	cap("attendance-current", "analytics", "Aktuell anwesende Personen", http.MethodGet, "/Anwesenheit/Anwesend_Aktuell_Liste.asp", Personal),
	cap("attendance-today", "analytics", "Heute anwesende Personen", http.MethodGet, "/Anwesenheit/Anwesend_Heute_Liste.asp", Personal),
	cap("attendance-date", "analytics", "Anwesenheitsseite für das serverseitig gewählte Datum", http.MethodGet, "/Anwesenheit/Anwesend_Datum_Liste.asp", Personal),
	cap("reha-attendance", "analytics", "Reha-Anwesenheit und freigegebene Zeitfenster", http.MethodGet, "/Anwesenheit/Reha_Anwesend_Datum_Liste.asp", Health),
	cap("prevention-attendance", "analytics", "Präventions-Anwesenheit in einem Datumsbereich", http.MethodPost, "/Anwesenheit/Praevention_Anwesend_Datum_Liste.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true)),
	cap("studio-hourly-load", "analytics", "Studio-Anwesenheit nach Stunde für einen Datumsbereich", http.MethodPost, "/Statistiken/Auswertungen/auswertung_Auslastung_Studio.asp", Aggregate,
		form("from", "von", Date, true), form("to", "bis", Date, true)),
	cap("attendance-weekly", "analytics", "Historische Studio-Anwesenheit nach Woche", http.MethodGet, "/Statistiken/Auswertungen/auswertung_Anwesenheit_Wochen.asp", Aggregate),
	cap("attendance-monthly", "analytics", "Historische Anwesenheit aktiver Mitglieder nach Monat", http.MethodGet, "/Kursplaner_Auswertungen/Statistik/Anwesenheit_Monat_Anzeigen.asp", Aggregate),
	cap("reha-attendance-monthly", "analytics", "Historische Reha-Anwesenheit nach Monat", http.MethodGet, "/Kursplaner_Auswertungen/Statistik/Reha_Anwesenheit_Monat_Anzeigen.asp", Aggregate),
	cap("prevention-attendance-monthly", "analytics", "Historische Präventions-Anwesenheit nach Monat", http.MethodGet, "/Kursplaner_Auswertungen/Statistik/Praevention_Anwesenheit_Monat_Anzeigen.asp", Aggregate),
	cap("attendance-week-range", "analytics", "Anwesenheit aktiver Mitglieder für einen begrenzten Wochenbereich", http.MethodPost, "/Kursplaner_Auswertungen/Statistik/Anwesenheit_Wochen_Anzeigen.asp", Aggregate,
		fixedQuery("Anzeige", "1"), form("year", "Jahr", Year, true), form("week-from", "Wochevon", Week, true), form("week-to", "Wochebis", Week, true)),
	cap("reha-week-range", "analytics", "Reha-Anwesenheit für einen begrenzten Wochenbereich", http.MethodPost, "/Kursplaner_Auswertungen/Statistik/Reha_Anwesenheit_Wochen_Anzeigen.asp", Aggregate,
		fixedQuery("Anzeige", "1"), form("year", "Jahr", Year, true), form("week-from", "Wochevon", Week, true), form("week-to", "Wochebis", Week, true)),
	cap("prevention-week-range", "analytics", "Präventions-Anwesenheit für einen begrenzten Wochenbereich", http.MethodPost, "/Kursplaner_Auswertungen/Statistik/Praevention_Anwesenheit_Wochen_Anzeigen.asp", Aggregate,
		fixedQuery("Anzeige", "1"), form("year", "Jahr", Year, true), form("week-from", "Wochevon", Week, true), form("week-to", "Wochebis", Week, true)),
	cap("age-structure", "analytics", "Altersstruktur der Mitglieder", http.MethodGet, "/Statistiken/auswertung_Alter.asp", Aggregate),
	cap("first-contact-active", "analytics", "Erstkontakt-Auswertung für aktive Personen", http.MethodGet, "/Statistiken/Erstkontakt_Besuch_Auswertung.asp", Personal,
		fixedQuery("MitgliedsformID", "1")),
	cap("first-contact-passive", "analytics", "Erstkontakt-Auswertung für passive Personen", http.MethodGet, "/Statistiken/Erstkontakt_Besuch_Auswertung.asp", Personal,
		fixedQuery("MitgliedsformID", "2")),
	cap("absences", "analytics", "Liste erfasster Fehlzeiten", http.MethodGet, "/Statistiken/Fehlzeiten/Fehlzeiten.asp", Personal),

	// Course planning and course analytics.
	cap("course-monthly", "courses", "Monatliche Kurs-Anwesenheit", http.MethodGet, "/Statistiken/Kursplaner/Kurs_Teilnehmer_anwesend_monatlich.asp", Aggregate),
	cap("course-history", "courses", "Anwesenheitssummen für einen Kurs nach Datum", http.MethodGet, "/Statistiken/Kursplaner/Kurs_Teilnehmer_anwesend_Kurs.asp", Aggregate,
		query("course-id", "KursID", PositiveID, true)),
	cap("course-definitions", "courses", "Kursstammdaten, Zeitplan, Raum und Aktivstatus", http.MethodGet, "/Kursplaner/Kurse_Liste.asp", Aggregate),
	cap("course-planner-week", "courses", "Aktueller wöchentlicher Kursplaner", http.MethodGet, "/Kursplaner_WEB/Wochenplaner_Tabelle_liste.asp", Personal),
	cap("course-planner-day", "courses", "Aktueller täglicher Kursplaner", http.MethodGet, "/Kursplaner_WEB/Tagesplaner_Tabelle_liste.asp", Personal),
	cap("course-day-list", "courses", "Druckbare Kursliste für einen Tag", http.MethodPost, "/Kursplaner/Listen/Kursplaner_Liste_Tag_anzeigen.asp", Personal,
		form("date", "von", Date, true)),
	cap("course-session", "courses", "Teilnehmerliste für einen Kurstermin", http.MethodGet, "/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp", Health,
		query("course-id", "Kurs", PositiveID, true), query("date", "Datum", Date, true),
		enumQuery("planner", "defaultMode", true, "A", "B")),
	cap("members-with-appointment", "courses", "Aktive Mitglieder mit Termin", http.MethodGet, "/Kursplaner_Auswertungen/Aktive_mit_Termin.asp", Personal),
	cap("members-with-appointment-7-days", "courses", "Aktive Mitglieder mit Termin in sieben Tagen", http.MethodGet, "/Kursplaner_Auswertungen/Aktive_mit_Termin_7Tage.asp", Personal),
	cap("members-with-appointment-14-days", "courses", "Aktive Mitglieder mit Termin in vierzehn Tagen", http.MethodGet, "/Kursplaner_Auswertungen/Aktive_mit_Termin_14Tage.asp", Personal),
	cap("members-without-appointment", "courses", "Aktive Mitglieder ohne Termin", http.MethodGet, "/Kursplaner_Auswertungen/Aktive_ohne_Termine.asp", Personal),
	cap("members-not-attending", "courses", "Aktive Mitglieder, die derzeit als nicht anwesend eingestuft sind", http.MethodGet, "/Kursplaner_Auswertungen/Aktive_nicht_anwesend.asp", Personal),
	cap("missed-appointment-contacts", "courses", "Kontaktliste für verpasste Termine", http.MethodGet, "/Kursplaner_Auswertungen/Kurs_Teilnehmer_verpasste_Termine.asp", Personal),
	cap("repeated-course-no-shows", "courses", "Mitglieder mit drei verpassten Kursterminen", http.MethodGet, "/Kursplaner_Auswertungen/Kurs_Teilnehmer_3nicht_anwesend.asp", Personal),
	cap("course-attended", "courses", "Anwesende Kursteilnehmer in einem Datumsbereich", http.MethodPost, "/Kursplaner_Auswertungen/Kurs_Teilnehmer_anwesend.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true)),
	cap("course-not-attended", "courses", "Nicht anwesende Kursteilnehmer in einem Datumsbereich", http.MethodPost, "/Kursplaner_Auswertungen/Kurs_Teilnehmer_nicht_anwesend.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true)),
	cap("course-cancelled", "courses", "Stornierte Kursteilnehmer in einem Datumsbereich", http.MethodPost, "/Kursplaner_Auswertungen/Kurs_Teilnehmer_storniert.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true)),
	cap("course-last-appointment-export", "courses", "Aktive Personen mit ihrem letzten Planer-Termin", http.MethodGet, "/Excel/Kursplaner/E_mitglieder_liste_Termin.asp", Personal),

	// Member and contract reads.
	cap("member-search", "members", "Aktive oder alle Personen suchen", http.MethodGet, "/Suchen/Suchen.asp", Personal,
		fixedQuery("Flag1", "1"), fixedQuery("Start", "1"),
		enumQuery("population", "Auswahl", true, "1", "2"),
		query("search", "Suchtext", SearchText, true)),
	cap("member-detail", "members", "Mitglieder-Detailseite ohne Absenden des Bearbeitungsformulars", http.MethodGet, "/Mitglieder/Mitglied_aendern.asp", Personal,
		query("member-id", "ID", PositiveID, true)),
	cap("member-checkins", "members", "Individuelle Check-in-Historie (Server-Standard)", http.MethodGet, "/Mitglieder/Anwesenheit/Mitglied_Liste_anwesend.asp", Personal,
		query("member-id", "MitgliedID", PositiveID, true)),
	cap("member-reha-history", "members", "Individuelle Reha-Anwesenheitshistorie (Server-Standard)", http.MethodGet, "/Mitglieder/Anwesenheit/Reha_Liste_anwesend.asp", Health,
		query("member-id", "MitgliedID", PositiveID, true)),
	cap("member-missing-signatures", "members", "Fehlende Reha-Unterschriften für ein Mitglied", http.MethodGet, "/Mitglieder/Anwesenheit/Reha_Liste_fehlende_Unterschriften.asp", Health,
		query("member-id", "MitgliedID", PositiveID, true)),
	cap("member-expired-prescriptions", "members", "Abgelaufene Reha-Verordnungen für ein Mitglied", http.MethodGet, "/Vertrag_Reha/Reha_Vertrag_Abgelaufen.asp", Health,
		query("member-id", "MitgliedID", PositiveID, true)),
	cap("member-reha-planner", "members", "Reha-Planer-Zuweisungen für ein Mitglied", http.MethodGet, "/Mitglieder/Anwesenheit/Mitglied_Liste_anwesend_Gesundheitsplaner.asp", Health,
		query("member-id", "MitgliedID", PositiveID, true)),
	cap("member-sport-planner", "members", "Sport-Planer-Zuweisungen für ein Mitglied", http.MethodGet, "/Mitglieder/Anwesenheit/Mitglied_Liste_anwesend_Gesundheitsplaner.asp", Health,
		query("member-id", "MitgliedID", PositiveID, true), fixedQuery("mode", "sport")),
	cap("people", "members", "Aktuelle Personenliste", http.MethodGet, "/Mitglieder/Personen_Liste.asp", Personal),
	cap("memberships", "members", "Mitgliedschaftsliste", http.MethodGet, "/Mitglieder/Vertrag_Liste.asp", Personal),
	cap("reha-members", "members", "Reha-Mitgliederliste", http.MethodGet, "/Mitglieder/Mitglied_Reha_Liste.asp", Health),
	cap("prevention-members", "members", "Präventions-Mitgliederliste", http.MethodGet, "/Mitglieder/Mitglied_Praevention_Liste.asp", Health),
	cap("card-contracts", "members", "Kartenvertragsliste", http.MethodGet, "/Mitglieder/Kartenvertrag_Liste.asp", Personal),
	cap("people-active-short", "members", "Kurzer Export aktiver Personen", http.MethodGet, "/Excel/Mitglied/E_mitglieder_liste_kurz.asp", Personal, fixedQuery("Status", "1")),
	cap("people-active-long", "members", "Langer Export aktiver Personen", http.MethodGet, "/Excel/Mitglied/E_mitglieder_liste_lang.asp", Personal, fixedQuery("Status", "1")),
	cap("people-former-short", "members", "Kurzer Export ehemaliger Personen", http.MethodGet, "/Excel/Mitglied/E_mitglieder_liste_kurz.asp", Personal, fixedQuery("Status", "2")),
	cap("people-former-long", "members", "Langer Export ehemaliger Personen", http.MethodGet, "/Excel/Mitglied/E_mitglieder_liste_lang.asp", Personal, fixedQuery("Status", "2")),
	cap("people-staff-short", "members", "Kurzer Mitarbeiter-Export", http.MethodGet, "/Excel/Mitglied/E_mitglieder_liste_kurz.asp", Personal, fixedQuery("Status", "4")),
	cap("people-staff-long", "members", "Langer Mitarbeiter-Export", http.MethodGet, "/Excel/Mitglied/E_mitglieder_liste_lang.asp", Personal, fixedQuery("Status", "4")),
	cap("people-without-reha-prevention", "members", "Aktive Personen ohne Reha oder Prävention", http.MethodGet, "/Excel/Mitglied/E_mitglieder_ohne_R_und_P.asp", Personal),
	cap("people-with-reha", "members", "Aktive Personen mit Reha", http.MethodGet, "/Excel/Mitglied/E_mitglieder_Reha.asp", Health),
	cap("people-with-prevention", "members", "Aktive Personen mit Prävention", http.MethodGet, "/Excel/Mitglied/E_mitglieder_Praevention.asp", Health),
	cap("birthdays", "members", "Geburtstagsliste", http.MethodGet, "/Notizen/Geburtstag/Geburtstag.asp", Personal),
	cap("notes", "members", "Vorhandene Mitgliedernotizen", http.MethodGet, "/Notizen/Notizen/Notizen_liste.asp", Personal),
	cap("extensions", "members", "Liste der Verlängerungen/Aktualisierungen", http.MethodGet, "/Verwaltung_Start/Aktualisierung.asp", Personal),
	cap("contract-pauses", "members", "Aktuelle Vertragsauszeiten", http.MethodGet, "/Vertrag/Auszeit/Auszeit_Liste_Vertraege.asp", Personal),
	cap("booked-contract-pauses", "members", "Gebuchte/abgelaufene Vertragsauszeiten", http.MethodGet, "/Vertrag/Auszeit/Auszeit_Liste_Vertraege_abgelaufen.asp", Personal),
	cap("prevention-pauses", "members", "Präventions-Auszeiten", http.MethodGet, "/Vertrag_Praevention/Auszeit/Auszeit_Liste_Praevention.asp", Health),
	cap("contracts-with-addons", "members", "Verträge mit Zusatz", http.MethodGet, "/Excel/Vertraege/E_Vertrag_Liste_Zusatz.asp", Personal),
	cap("contracts-complete-archive", "members", "Gelöschte und abgelaufene Verträge", http.MethodGet, "/Excel/Vertraege/E_Vertrag_alle_geloescht.asp", Personal),
	cap("contracts-with-address", "members", "Verträge inklusive Adressdaten", http.MethodGet, "/Excel/Sonstiges/E_Vertrag_Liste_mit_Anschrift.asp", Personal),

	// Prescription, Reha and prevention controlling.
	cap("prescription-remaining-units", "prescriptions", "Personen unter einem Schwellenwert für Resttermine", http.MethodPost, "/Statistiken/Verordnungen/Verordnung_Personen_Rest_Termine.asp", Health,
		form("threshold", "Menge", NonNegativeInt, true)),
	cap("prescription-expiry", "prescriptions", "Verordnungen, die in einem Datumsbereich enden", http.MethodPost, "/Statistiken/Verordnungen/Verordnung_Personen_Ende.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true)),
	cap("prescription-last-attendance", "prescriptions", "Personen ohne Anwesenheit seit einem Datum", http.MethodPost, "/Statistiken/Verordnungen_Anwesenheit/Verordnung_Personen_nicht_anwesend.asp", Health,
		form("from", "von", Date, true)),
	cap("reha-prescriptions-by-referrer", "prescriptions", "Reha-Verordnungen nach Person und Überweiser", http.MethodPost, "/Statistiken/Verordnungen/R_Auswertung_Verordnung_Personen_Liste.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true),
		form("referrer-id", "Arzt", PositiveID, false)),
	cap("reha-prescription-summary", "prescriptions", "Aggregierte Reha-Verordnungen nach Überweiser", http.MethodPost, "/Statistiken/Verordnungen/R_Auswertung_Verordnung_Summe_Liste.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true),
		form("referrer-id", "Arzt", PositiveID, false)),
	cap("prevention-prescriptions-by-referrer", "prescriptions", "Präventions-Verordnungen nach Person und Überweiser", http.MethodPost, "/Statistiken/Verordnungen/P_Auswertung_Verordnung_Personen_Liste.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true),
		form("referrer-id", "Arzt", PositiveID, false)),
	cap("prevention-prescription-summary", "prescriptions", "Aggregierte Präventions-Verordnungen nach Überweiser", http.MethodPost, "/Statistiken/Verordnungen/P_Auswertung_Verordnung_Summe_Liste.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true),
		form("referrer-id", "Arzt", PositiveID, false)),
	cap("reha-prescriptions", "prescriptions", "Export aktiver Reha-Verordnungen", http.MethodGet, "/Excel/Reha/Vertrag_Liste_Reha.asp", Health),
	cap("reha-prescriptions-archived", "prescriptions", "Abgelaufene und gelöschte Reha-Verordnungen", http.MethodGet, "/Excel/Reha/Vertrag_Liste_Reha_geloescht.asp", Health),
	cap("reha-multiple-prescriptions", "prescriptions", "Personen mit mehreren Reha-Verordnungen", http.MethodGet, "/Excel/Reha/Vertrag_Liste_Reha_Mehr_Vertraege.asp", Health),
	cap("reha-times-active", "prescriptions", "Aktive Reha-Verordnungszeiten", http.MethodGet, "/Excel/Reha/Vertrag_Liste_Reha_Zeiten.asp", Health, fixedQuery("Art", "1")),
	cap("reha-times-archived", "prescriptions", "Archivierte Reha-Verordnungszeiten", http.MethodGet, "/Excel/Reha/Vertrag_Liste_Reha_Zeiten.asp", Health, fixedQuery("Art", "2")),
	cap("reha-times-inconsistent", "prescriptions", "Unstimmige Reha-Verordnungszeiten", http.MethodGet, "/Excel/Reha/Vertrag_Liste_Reha_Unstimmigkeiten.asp", Health),
	cap("reha-plus", "prescriptions", "Reha-Plus-Export", http.MethodGet, "/Excel/Reha/E_Reha_Plus_Liste.asp", Health),
	cap("reha-without-visit", "prescriptions", "Reha-Verordnungen ohne Besuch seit vier Wochen", http.MethodGet, "/Excel/Reha/Reha_ohne_Besuch_Liste.asp", Health),
	cap("reha-plus-without-visit", "prescriptions", "Reha-Plus-Verordnungen ohne Besuch seit vier Wochen", http.MethodGet, "/Excel/Reha/Reha_plus_ohne_Besuch_Liste.asp", Health),
	cap("reha-without-prescription", "prescriptions", "Gültige Reha-Verträge ohne Verordnung", http.MethodGet, "/Excel/Sonstiges/E_Reha_ohne_Rezept.asp", Health),
	cap("reha-time-visit-counts", "prescriptions", "Reha-Zeit- und Besuchszahlen", http.MethodGet, "/Excel/Sonstiges/E_Reha_Zeiten.asp", Health),
	cap("duplicate-reha-times", "prescriptions", "Doppelte Reha-Zeiteinträge", http.MethodGet, "/Excel/Reha_Zeiten/Reha_Zeit_doppelt_anzeigen.asp", Health),
	cap("trena-prescriptions", "prescriptions", "Aktive T-RENA-Verordnungen", http.MethodGet, "/Excel/TRena/Vertrag_Liste_TRena.asp", Health),
	cap("trena-prescriptions-archived", "prescriptions", "Abgelaufene und gelöschte T-RENA-Verordnungen", http.MethodGet, "/Excel/TRena/Vertrag_Liste_TRena_geloescht.asp", Health),

	// Read-only compliance checks. These pages must never be confused with billing actions.
	cap("missing-reha-signatures", "compliance", "Personen mit fehlenden Reha-Unterschriften", http.MethodGet, "/Vertrag_Reha/Unterschrift/Fehlende_Reha_Unterschriften_Liste.asp", Health),
	cap("missing-insurance-data", "compliance", "Verordnungen mit fehlenden Krankenkassendaten", http.MethodGet, "/Krankenkasse/Krankenkasse_pruefen.asp", Health, fixedQuery("Art", "2")),
	cap("missing-attendee-signatures", "compliance", "Aktive Verordnungen mit fehlenden Teilnehmer-Unterschriften", http.MethodGet, "/Abrechnung_Reha_Digital/Abrechnung_Reha_Unterschriften_Aktive_pruefen.asp", Health),
	cap("missing-instructor-signatures", "compliance", "Kursleiter mit fehlenden Unterschriften", http.MethodGet, "/Abrechnung_Reha_Digital/Abrechnung_Reha_Unterschriften_Kursleiter_pruefen.asp", Health),
	cap("billing-number-mismatches", "compliance", "Abweichungen der Abrechnungsnummern bei Besuch und Verordnung", http.MethodGet, "/Abrechnung_Reha_Digital/Abrechnungsnummern_Digital_Zeit_VO_pruefen.asp", Health),
	cap("prescription-unit-overruns", "compliance", "Verordnungen mit Überschreitung der verordneten Übungseinheiten", http.MethodGet, "/Abrechnung_Reha_Digital/Abrechnung_Reha_Digital_Menge_Zeit_pruefen.asp", Health),
	cap("duplicate-daily-units", "compliance", "Mehr als eine Übungseinheit pro Person an einem Tag", http.MethodGet, "/Abrechnung_Reha_Digital/Abrechnung_Reha_Digital_Doppelte_Menge_Tag_pruefen.asp", Health),

	// Explicitly isolated financial reads.
	cap("member-bank-export", "financial", "Export von Mandats-ID, IBAN und BIC", http.MethodGet, "/Excel/Sonstiges/E_Mitglieder_IBAN.asp", Financial),
	cap("private-billing-manual-archive", "financial", "Archiv manuell abgerechneter Privatfälle", http.MethodGet, "/Abrechnung_Reha_Privat/Manuell/Reha_Privat_Manuell_Abgerechnete_Liste.asp", Financial),
	cap("private-billing-digital-archive", "financial", "Archiv digital abgerechneter Privatfälle", http.MethodGet, "/Abrechnung_Reha_Privat/Digital/Reha_Privat_Digital_Abgerechnete_Liste.asp", Financial),
	cap("digital-billing-complete-archive", "financial", "Archiv abgeschlossener digitaler Vollabrechnungen", http.MethodGet, "/Abrechnung_Reha_Digital/Aktive/Digital_Reha_Aktive_Abgerechnete_Liste.asp", Financial,
		query("ik", "IK", IKNumber, true)),
	cap("digital-billing-final-archive", "financial", "Archiv abgeschlossener digitaler Schlussabrechnungen", http.MethodGet, "/Abrechnung_Reha_Digital/Abgelaufene/Digital_Reha_Abgelaufene_Abgerechnete_Liste.asp", Financial,
		query("ik", "IK", IKNumber, true)),
}

func init() {
	sort.Slice(capabilities, func(i, j int) bool {
		return capabilities[i].Name < capabilities[j].Name
	})
}

func List() []Capability {
	result := make([]Capability, len(capabilities))
	copy(result, capabilities)
	return result
}

func Lookup(name string) (Capability, bool) {
	name = strings.TrimSpace(strings.ToLower(name))
	index := sort.Search(len(capabilities), func(i int) bool {
		return capabilities[i].Name >= name
	})
	if index >= len(capabilities) || capabilities[index].Name != name {
		return Capability{}, false
	}
	return capabilities[index], true
}

func Build(name string, values map[string]string) (Request, error) {
	capability, ok := Lookup(name)
	if !ok {
		return Request{}, fmt.Errorf("unknown capability %q", name)
	}
	remaining := make(map[string]string, len(values))
	for key, value := range values {
		if strings.TrimSpace(value) != "" {
			remaining[key] = strings.TrimSpace(value)
		}
	}
	queryValues := url.Values{}
	formValues := url.Values{}
	normalizedValues := make(map[string]string)
	for _, parameter := range capability.Parameters {
		value := parameter.Fixed
		if parameter.Name != "" {
			value = remaining[parameter.Name]
			delete(remaining, parameter.Name)
		}
		if value == "" {
			if parameter.Required {
				return Request{}, fmt.Errorf(
					"capability %q requires --%s",
					capability.Name,
					parameter.Name,
				)
			}
			continue
		}
		normalized, err := normalizeValue(parameter, value)
		if err != nil {
			return Request{}, fmt.Errorf("--%s: %w", parameter.Name, err)
		}
		if parameter.Name != "" {
			normalizedValues[parameter.Name] = normalized
		}
		switch parameter.Location {
		case Query:
			queryValues.Set(parameter.Remote, normalized)
		case Form:
			formValues.Set(parameter.Remote, normalized)
		default:
			return Request{}, fmt.Errorf("capability %q has an invalid parameter location", capability.Name)
		}
	}
	if len(remaining) > 0 {
		keys := make([]string, 0, len(remaining))
		for key := range remaining {
			keys = append(keys, "--"+key)
		}
		sort.Strings(keys)
		return Request{}, fmt.Errorf(
			"capability %q does not accept %s",
			capability.Name,
			strings.Join(keys, ", "),
		)
	}
	if err := validateRange(normalizedValues); err != nil {
		return Request{}, fmt.Errorf("capability %q: %w", capability.Name, err)
	}
	requestPath := capability.Path
	if encoded := queryValues.Encode(); encoded != "" {
		requestPath += "?" + encoded
	}
	return Request{
		Capability: capability,
		Path:       requestPath,
		Body:       formValues.Encode(),
	}, nil
}

func validateRange(values map[string]string) error {
	fromValue, hasFrom := values["from"]
	toValue, hasTo := values["to"]
	if hasFrom && hasTo {
		from, fromErr := time.Parse("02.01.2006", fromValue)
		to, toErr := time.Parse("02.01.2006", toValue)
		if fromErr != nil || toErr != nil {
			return fmt.Errorf("date range is invalid")
		}
		if to.Before(from) {
			return fmt.Errorf("--to must not be before --from")
		}
		if to.Sub(from) > maxDateRangeDays*24*time.Hour {
			return fmt.Errorf("date range must not exceed %d days", maxDateRangeDays)
		}
	}
	weekFromValue, hasWeekFrom := values["week-from"]
	weekToValue, hasWeekTo := values["week-to"]
	if hasWeekFrom && hasWeekTo {
		weekFrom, _ := strconv.Atoi(weekFromValue)
		weekTo, _ := strconv.Atoi(weekToValue)
		if weekTo < weekFrom {
			return fmt.Errorf("--week-to must not be before --week-from")
		}
	}
	return nil
}

func ValidateAdminURL(method string, parsed *url.URL) bool {
	if parsed == nil {
		return false
	}
	method = strings.ToUpper(method)
	queryValues, err := url.ParseQuery(parsed.RawQuery)
	if err != nil {
		return false
	}
	for _, capability := range capabilities {
		if capability.Method != method || capability.Path != parsed.EscapedPath() {
			continue
		}
		if matchesQuery(capability, queryValues) {
			return true
		}
	}
	return false
}

func ValidateRequest(request Request) bool {
	capability, ok := Lookup(request.Capability.Name)
	if !ok ||
		request.Capability.Method != capability.Method ||
		request.Capability.Path != capability.Path ||
		request.Capability.Sensitivity != capability.Sensitivity {
		return false
	}
	parsed, err := url.Parse(request.Path)
	if err != nil ||
		parsed.IsAbs() ||
		parsed.EscapedPath() != capability.Path ||
		!matchesQuery(capability, parsed.Query()) {
		return false
	}
	if capability.Method == http.MethodGet {
		return request.Body == ""
	}
	if capability.Method != http.MethodPost {
		return false
	}
	formValues, err := url.ParseQuery(request.Body)
	if err != nil {
		return false
	}
	return matchesParameters(capability, Form, formValues)
}

func SensitivityForRouteKey(route string) (Sensitivity, bool) {
	const prefix = "capability:"
	if !strings.HasPrefix(route, prefix) {
		return "", false
	}
	capability, ok := Lookup(strings.TrimPrefix(route, prefix))
	if !ok {
		return "", false
	}
	return capability.Sensitivity, true
}

func SensitivityForStoredRoute(route string) (Sensitivity, bool) {
	if sensitivity, ok := SensitivityForRouteKey(route); ok {
		return sensitivity, true
	}
	if route == "/start_Anwesenheit.asp" {
		return Aggregate, true
	}
	var (
		selected Sensitivity
		found    bool
	)
	for _, capability := range capabilities {
		if capability.Path != route {
			continue
		}
		if !found || sensitivityRank(capability.Sensitivity) > sensitivityRank(selected) {
			selected = capability.Sensitivity
			found = true
		}
	}
	return selected, found
}

func RouteKey(name string) string {
	return "capability:" + strings.TrimSpace(strings.ToLower(name))
}

func sensitivityRank(sensitivity Sensitivity) int {
	switch sensitivity {
	case Aggregate:
		return 0
	case Personal:
		return 1
	case Health:
		return 2
	case Financial:
		return 3
	default:
		return 4
	}
}

func matchesQuery(capability Capability, values url.Values) bool {
	return matchesParameters(capability, Query, values)
}

func matchesParameters(
	capability Capability,
	location Location,
	values url.Values,
) bool {
	expected := make(map[string]Parameter)
	for _, parameter := range capability.Parameters {
		if parameter.Location == location {
			expected[parameter.Remote] = parameter
		}
	}
	for key, queryValues := range values {
		parameter, ok := expected[key]
		if !ok || len(queryValues) != 1 {
			return false
		}
		value := queryValues[0]
		if parameter.Fixed != "" {
			if value != parameter.Fixed {
				return false
			}
			continue
		}
		if _, err := validateRemoteValue(parameter, value); err != nil {
			return false
		}
	}
	for key, parameter := range expected {
		if parameter.Required && len(values[key]) != 1 {
			return false
		}
	}
	return true
}

func normalizeValue(parameter Parameter, value string) (string, error) {
	value = strings.TrimSpace(value)
	switch parameter.Kind {
	case Date:
		if parsed, err := time.Parse("2006-01-02", value); err == nil {
			return parsed.Format("02.01.2006"), nil
		}
		parsed, err := time.Parse("02.01.2006", value)
		if err != nil {
			return "", fmt.Errorf("must be YYYY-MM-DD")
		}
		return parsed.Format("02.01.2006"), nil
	default:
		return validateRemoteValue(parameter, value)
	}
}

func validateRemoteValue(parameter Parameter, value string) (string, error) {
	switch parameter.Kind {
	case "":
		if parameter.Fixed != "" && value == parameter.Fixed {
			return value, nil
		}
		return "", fmt.Errorf("invalid fixed value")
	case Date:
		if _, err := time.Parse("02.01.2006", value); err != nil {
			return "", fmt.Errorf("must be YYYY-MM-DD")
		}
	case PositiveID:
		if !positiveIDPattern.MatchString(value) {
			return "", fmt.Errorf("must be a positive numeric identifier")
		}
	case NonNegativeInt:
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 0 || parsed > 100_000 {
			return "", fmt.Errorf("must be between 0 and 100000")
		}
	case Year:
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 2000 || parsed > 2100 {
			return "", fmt.Errorf("must be between 2000 and 2100")
		}
	case Week:
		parsed, err := strconv.Atoi(value)
		if err != nil || parsed < 1 || parsed > 53 {
			return "", fmt.Errorf("must be between 1 and 53")
		}
	case Enum:
		for _, allowed := range parameter.Allowed {
			if value == allowed {
				return value, nil
			}
		}
		return "", fmt.Errorf("must be one of %s", strings.Join(parameter.Allowed, ", "))
	case SearchText:
		if len(value) > 128 || strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("must contain 1 to 128 characters")
		}
		for _, character := range value {
			if unicode.IsControl(character) {
				return "", fmt.Errorf("must not contain control characters")
			}
		}
	case IKNumber:
		if !ikPattern.MatchString(value) {
			return "", fmt.Errorf("must contain exactly nine digits")
		}
	default:
		return "", fmt.Errorf("unknown parameter kind %q", parameter.Kind)
	}
	return value, nil
}
