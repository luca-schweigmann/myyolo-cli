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
	cap("attendance-current", "analytics", "Currently present people", http.MethodGet, "/Anwesenheit/Anwesend_Aktuell_Liste.asp", Personal),
	cap("attendance-today", "analytics", "People present today", http.MethodGet, "/Anwesenheit/Anwesend_Heute_Liste.asp", Personal),
	cap("attendance-date", "analytics", "Attendance page for the server-selected date", http.MethodGet, "/Anwesenheit/Anwesend_Datum_Liste.asp", Personal),
	cap("reha-attendance", "analytics", "Reha attendance and exposed time windows", http.MethodGet, "/Anwesenheit/Reha_Anwesend_Datum_Liste.asp", Health),
	cap("prevention-attendance", "analytics", "Prevention attendance in a date range", http.MethodPost, "/Anwesenheit/Praevention_Anwesend_Datum_Liste.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true)),
	cap("studio-hourly-load", "analytics", "Facility attendance grouped by hour for a date range", http.MethodPost, "/Statistiken/Auswertungen/auswertung_Auslastung_Studio.asp", Aggregate,
		form("from", "von", Date, true), form("to", "bis", Date, true)),
	cap("attendance-weekly", "analytics", "Historic facility attendance grouped by week", http.MethodGet, "/Statistiken/Auswertungen/auswertung_Anwesenheit_Wochen.asp", Aggregate),
	cap("attendance-monthly", "analytics", "Historic active-member attendance grouped by month", http.MethodGet, "/Kursplaner_Auswertungen/Statistik/Anwesenheit_Monat_Anzeigen.asp", Aggregate),
	cap("reha-attendance-monthly", "analytics", "Historic Reha attendance grouped by month", http.MethodGet, "/Kursplaner_Auswertungen/Statistik/Reha_Anwesenheit_Monat_Anzeigen.asp", Aggregate),
	cap("prevention-attendance-monthly", "analytics", "Historic prevention attendance grouped by month", http.MethodGet, "/Kursplaner_Auswertungen/Statistik/Praevention_Anwesenheit_Monat_Anzeigen.asp", Aggregate),
	cap("attendance-week-range", "analytics", "Active-member attendance for a bounded week range", http.MethodPost, "/Kursplaner_Auswertungen/Statistik/Anwesenheit_Wochen_Anzeigen.asp", Aggregate,
		fixedQuery("Anzeige", "1"), form("year", "Jahr", Year, true), form("week-from", "Wochevon", Week, true), form("week-to", "Wochebis", Week, true)),
	cap("reha-week-range", "analytics", "Reha attendance for a bounded week range", http.MethodPost, "/Kursplaner_Auswertungen/Statistik/Reha_Anwesenheit_Wochen_Anzeigen.asp", Aggregate,
		fixedQuery("Anzeige", "1"), form("year", "Jahr", Year, true), form("week-from", "Wochevon", Week, true), form("week-to", "Wochebis", Week, true)),
	cap("prevention-week-range", "analytics", "Prevention attendance for a bounded week range", http.MethodPost, "/Kursplaner_Auswertungen/Statistik/Praevention_Anwesenheit_Wochen_Anzeigen.asp", Aggregate,
		fixedQuery("Anzeige", "1"), form("year", "Jahr", Year, true), form("week-from", "Wochevon", Week, true), form("week-to", "Wochebis", Week, true)),
	cap("age-structure", "analytics", "Member age distribution", http.MethodGet, "/Statistiken/auswertung_Alter.asp", Aggregate),
	cap("first-contact-active", "analytics", "First-contact evaluation for active people", http.MethodGet, "/Statistiken/Erstkontakt_Besuch_Auswertung.asp", Personal,
		fixedQuery("MitgliedsformID", "1")),
	cap("first-contact-passive", "analytics", "First-contact evaluation for passive people", http.MethodGet, "/Statistiken/Erstkontakt_Besuch_Auswertung.asp", Personal,
		fixedQuery("MitgliedsformID", "2")),
	cap("absences", "analytics", "Recorded absence list", http.MethodGet, "/Statistiken/Fehlzeiten/Fehlzeiten.asp", Personal),

	// Course planning and course analytics.
	cap("course-monthly", "courses", "Monthly course attendance", http.MethodGet, "/Statistiken/Kursplaner/Kurs_Teilnehmer_anwesend_monatlich.asp", Aggregate),
	cap("course-history", "courses", "Attendance totals for one course by date", http.MethodGet, "/Statistiken/Kursplaner/Kurs_Teilnehmer_anwesend_Kurs.asp", Aggregate,
		query("course-id", "KursID", PositiveID, true)),
	cap("course-definitions", "courses", "Course master data, schedule, room and active status", http.MethodGet, "/Kursplaner/Kurse_Liste.asp", Aggregate),
	cap("course-planner-week", "courses", "Current weekly course planner", http.MethodGet, "/Kursplaner_WEB/Wochenplaner_Tabelle_liste.asp", Personal),
	cap("course-planner-day", "courses", "Current daily course planner", http.MethodGet, "/Kursplaner_WEB/Tagesplaner_Tabelle_liste.asp", Personal),
	cap("course-day-list", "courses", "Printable course list for one day", http.MethodPost, "/Kursplaner/Listen/Kursplaner_Liste_Tag_anzeigen.asp", Personal,
		form("date", "von", Date, true)),
	cap("course-session", "courses", "Participant list for one course occurrence", http.MethodGet, "/Kursplaner_WEB/Kursplaner_Teilnehmer_eingabe.asp", Health,
		query("course-id", "Kurs", PositiveID, true), query("date", "Datum", Date, true),
		enumQuery("planner", "defaultMode", true, "A", "B")),
	cap("members-with-appointment", "courses", "Active members with an appointment", http.MethodGet, "/Kursplaner_Auswertungen/Aktive_mit_Termin.asp", Personal),
	cap("members-with-appointment-7-days", "courses", "Active members with an appointment in seven days", http.MethodGet, "/Kursplaner_Auswertungen/Aktive_mit_Termin_7Tage.asp", Personal),
	cap("members-with-appointment-14-days", "courses", "Active members with an appointment in fourteen days", http.MethodGet, "/Kursplaner_Auswertungen/Aktive_mit_Termin_14Tage.asp", Personal),
	cap("members-without-appointment", "courses", "Active members without an appointment", http.MethodGet, "/Kursplaner_Auswertungen/Aktive_ohne_Termine.asp", Personal),
	cap("members-not-attending", "courses", "Active members currently classified as not attending", http.MethodGet, "/Kursplaner_Auswertungen/Aktive_nicht_anwesend.asp", Personal),
	cap("missed-appointment-contacts", "courses", "Contact list for missed appointments", http.MethodGet, "/Kursplaner_Auswertungen/Kurs_Teilnehmer_verpasste_Termine.asp", Personal),
	cap("repeated-course-no-shows", "courses", "Members with three missed course appointments", http.MethodGet, "/Kursplaner_Auswertungen/Kurs_Teilnehmer_3nicht_anwesend.asp", Personal),
	cap("course-attended", "courses", "Attended course participants in a date range", http.MethodPost, "/Kursplaner_Auswertungen/Kurs_Teilnehmer_anwesend.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true)),
	cap("course-not-attended", "courses", "Non-attending course participants in a date range", http.MethodPost, "/Kursplaner_Auswertungen/Kurs_Teilnehmer_nicht_anwesend.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true)),
	cap("course-cancelled", "courses", "Cancelled course participants in a date range", http.MethodPost, "/Kursplaner_Auswertungen/Kurs_Teilnehmer_storniert.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true)),
	cap("course-last-appointment-export", "courses", "Active people with their last planner appointment", http.MethodGet, "/Excel/Kursplaner/E_mitglieder_liste_Termin.asp", Personal),

	// Member and contract reads.
	cap("member-search", "members", "Search active or all people", http.MethodGet, "/Suchen/Suchen.asp", Personal,
		fixedQuery("Flag1", "1"), fixedQuery("Start", "1"),
		enumQuery("population", "Auswahl", true, "1", "2"),
		query("search", "Suchtext", SearchText, true)),
	cap("member-detail", "members", "One member detail page without submitting its edit form", http.MethodGet, "/Mitglieder/Mitglied_aendern.asp", Personal,
		query("member-id", "ID", PositiveID, true)),
	cap("member-checkins", "members", "Server-default individual check-in history", http.MethodGet, "/Mitglieder/Anwesenheit/Mitglied_Liste_anwesend.asp", Personal,
		query("member-id", "MitgliedID", PositiveID, true)),
	cap("member-reha-history", "members", "Server-default individual Reha attendance history", http.MethodGet, "/Mitglieder/Anwesenheit/Reha_Liste_anwesend.asp", Health,
		query("member-id", "MitgliedID", PositiveID, true)),
	cap("member-missing-signatures", "members", "Missing Reha signatures for one member", http.MethodGet, "/Mitglieder/Anwesenheit/Reha_Liste_fehlende_Unterschriften.asp", Health,
		query("member-id", "MitgliedID", PositiveID, true)),
	cap("member-expired-prescriptions", "members", "Expired Reha prescriptions for one member", http.MethodGet, "/Vertrag_Reha/Reha_Vertrag_Abgelaufen.asp", Health,
		query("member-id", "MitgliedID", PositiveID, true)),
	cap("member-reha-planner", "members", "Reha planner assignments for one member", http.MethodGet, "/Mitglieder/Anwesenheit/Mitglied_Liste_anwesend_Gesundheitsplaner.asp", Health,
		query("member-id", "MitgliedID", PositiveID, true)),
	cap("member-sport-planner", "members", "Sport planner assignments for one member", http.MethodGet, "/Mitglieder/Anwesenheit/Mitglied_Liste_anwesend_Gesundheitsplaner.asp", Personal,
		query("member-id", "MitgliedID", PositiveID, true), fixedQuery("mode", "sport")),
	cap("people", "members", "Current people list", http.MethodGet, "/Mitglieder/Personen_Liste.asp", Personal),
	cap("memberships", "members", "Membership list", http.MethodGet, "/Mitglieder/Vertrag_Liste.asp", Personal),
	cap("reha-members", "members", "Reha member list", http.MethodGet, "/Mitglieder/Mitglied_Reha_Liste.asp", Health),
	cap("prevention-members", "members", "Prevention member list", http.MethodGet, "/Mitglieder/Mitglied_Praevention_Liste.asp", Health),
	cap("card-contracts", "members", "Card-contract list", http.MethodGet, "/Mitglieder/Kartenvertrag_Liste.asp", Personal),
	cap("people-active-short", "members", "Short active-person export", http.MethodGet, "/Excel/Mitglied/E_mitglieder_liste_kurz.asp", Personal, fixedQuery("Status", "1")),
	cap("people-active-long", "members", "Long active-person export", http.MethodGet, "/Excel/Mitglied/E_mitglieder_liste_lang.asp", Personal, fixedQuery("Status", "1")),
	cap("people-former-short", "members", "Short former-person export", http.MethodGet, "/Excel/Mitglied/E_mitglieder_liste_kurz.asp", Personal, fixedQuery("Status", "2")),
	cap("people-former-long", "members", "Long former-person export", http.MethodGet, "/Excel/Mitglied/E_mitglieder_liste_lang.asp", Personal, fixedQuery("Status", "2")),
	cap("people-staff-short", "members", "Short staff export", http.MethodGet, "/Excel/Mitglied/E_mitglieder_liste_kurz.asp", Personal, fixedQuery("Status", "4")),
	cap("people-staff-long", "members", "Long staff export", http.MethodGet, "/Excel/Mitglied/E_mitglieder_liste_lang.asp", Personal, fixedQuery("Status", "4")),
	cap("people-without-reha-prevention", "members", "Active people without Reha or prevention", http.MethodGet, "/Excel/Mitglied/E_mitglieder_ohne_R_und_P.asp", Personal),
	cap("people-with-reha", "members", "Active people with Reha", http.MethodGet, "/Excel/Mitglied/E_mitglieder_Reha.asp", Health),
	cap("people-with-prevention", "members", "Active people with prevention", http.MethodGet, "/Excel/Mitglied/E_mitglieder_Praevention.asp", Health),
	cap("birthdays", "members", "Birthday list", http.MethodGet, "/Notizen/Geburtstag/Geburtstag.asp", Personal),
	cap("notes", "members", "Existing member notes", http.MethodGet, "/Notizen/Notizen/Notizen_liste.asp", Personal),
	cap("extensions", "members", "Extension/update list", http.MethodGet, "/Verwaltung_Start/Aktualisierung.asp", Personal),
	cap("contract-pauses", "members", "Current contract pauses", http.MethodGet, "/Vertrag/Auszeit/Auszeit_Liste_Vertraege.asp", Personal),
	cap("booked-contract-pauses", "members", "Booked/expired contract pauses", http.MethodGet, "/Vertrag/Auszeit/Auszeit_Liste_Vertraege_abgelaufen.asp", Personal),
	cap("prevention-pauses", "members", "Prevention pauses", http.MethodGet, "/Vertrag_Praevention/Auszeit/Auszeit_Liste_Praevention.asp", Health),
	cap("contracts-with-addons", "members", "Contracts with an add-on", http.MethodGet, "/Excel/Vertraege/E_Vertrag_Liste_Zusatz.asp", Personal),
	cap("contracts-complete-archive", "members", "Deleted and expired contracts", http.MethodGet, "/Excel/Vertraege/E_Vertrag_alle_geloescht.asp", Personal),
	cap("contracts-with-address", "members", "Contracts including address data", http.MethodGet, "/Excel/Sonstiges/E_Vertrag_Liste_mit_Anschrift.asp", Personal),

	// Prescription, Reha and prevention controlling.
	cap("prescription-remaining-units", "prescriptions", "People below a remaining-unit threshold", http.MethodPost, "/Statistiken/Verordnungen/Verordnung_Personen_Rest_Termine.asp", Health,
		form("threshold", "Menge", NonNegativeInt, true)),
	cap("prescription-expiry", "prescriptions", "Prescriptions ending in a date range", http.MethodPost, "/Statistiken/Verordnungen/Verordnung_Personen_Ende.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true)),
	cap("prescription-last-attendance", "prescriptions", "People without attendance since a date", http.MethodPost, "/Statistiken/Verordnungen_Anwesenheit/Verordnung_Personen_nicht_anwesend.asp", Health,
		form("from", "von", Date, true)),
	cap("reha-prescriptions-by-referrer", "prescriptions", "Reha prescriptions by person and referrer", http.MethodPost, "/Statistiken/Verordnungen/R_Auswertung_Verordnung_Personen_Liste.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true),
		form("referrer-id", "Arzt", PositiveID, false)),
	cap("reha-prescription-summary", "prescriptions", "Aggregate Reha prescriptions by referrer", http.MethodPost, "/Statistiken/Verordnungen/R_Auswertung_Verordnung_Summe_Liste.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true),
		form("referrer-id", "Arzt", PositiveID, false)),
	cap("prevention-prescriptions-by-referrer", "prescriptions", "Prevention prescriptions by person and referrer", http.MethodPost, "/Statistiken/Verordnungen/P_Auswertung_Verordnung_Personen_Liste.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true),
		form("referrer-id", "Arzt", PositiveID, false)),
	cap("prevention-prescription-summary", "prescriptions", "Aggregate prevention prescriptions by referrer", http.MethodPost, "/Statistiken/Verordnungen/P_Auswertung_Verordnung_Summe_Liste.asp", Health,
		form("from", "von", Date, true), form("to", "bis", Date, true),
		form("referrer-id", "Arzt", PositiveID, false)),
	cap("reha-prescriptions", "prescriptions", "Active Reha prescription export", http.MethodGet, "/Excel/Reha/Vertrag_Liste_Reha.asp", Health),
	cap("reha-prescriptions-archived", "prescriptions", "Expired and deleted Reha prescriptions", http.MethodGet, "/Excel/Reha/Vertrag_Liste_Reha_geloescht.asp", Health),
	cap("reha-multiple-prescriptions", "prescriptions", "People with multiple Reha prescriptions", http.MethodGet, "/Excel/Reha/Vertrag_Liste_Reha_Mehr_Vertraege.asp", Health),
	cap("reha-times-active", "prescriptions", "Active Reha prescription times", http.MethodGet, "/Excel/Reha/Vertrag_Liste_Reha_Zeiten.asp", Health, fixedQuery("Art", "1")),
	cap("reha-times-archived", "prescriptions", "Archived Reha prescription times", http.MethodGet, "/Excel/Reha/Vertrag_Liste_Reha_Zeiten.asp", Health, fixedQuery("Art", "2")),
	cap("reha-times-inconsistent", "prescriptions", "Inconsistent Reha prescription times", http.MethodGet, "/Excel/Reha/Vertrag_Liste_Reha_Unstimmigkeiten.asp", Health),
	cap("reha-plus", "prescriptions", "Reha-plus export", http.MethodGet, "/Excel/Reha/E_Reha_Plus_Liste.asp", Health),
	cap("reha-without-visit", "prescriptions", "Reha prescriptions without a visit for four weeks", http.MethodGet, "/Excel/Reha/Reha_ohne_Besuch_Liste.asp", Health),
	cap("reha-plus-without-visit", "prescriptions", "Reha-plus prescriptions without a visit for four weeks", http.MethodGet, "/Excel/Reha/Reha_plus_ohne_Besuch_Liste.asp", Health),
	cap("reha-without-prescription", "prescriptions", "Valid Reha contracts without a prescription", http.MethodGet, "/Excel/Sonstiges/E_Reha_ohne_Rezept.asp", Health),
	cap("reha-time-visit-counts", "prescriptions", "Reha time and visit counts", http.MethodGet, "/Excel/Sonstiges/E_Reha_Zeiten.asp", Health),
	cap("duplicate-reha-times", "prescriptions", "Duplicate Reha time entries", http.MethodGet, "/Excel/Reha_Zeiten/Reha_Zeit_doppelt_anzeigen.asp", Health),
	cap("trena-prescriptions", "prescriptions", "Active T-RENA prescriptions", http.MethodGet, "/Excel/TRena/Vertrag_Liste_TRena.asp", Health),
	cap("trena-prescriptions-archived", "prescriptions", "Expired and deleted T-RENA prescriptions", http.MethodGet, "/Excel/TRena/Vertrag_Liste_TRena_geloescht.asp", Health),

	// Read-only compliance checks. These pages must never be confused with billing actions.
	cap("missing-reha-signatures", "compliance", "People with missing Reha signatures", http.MethodGet, "/Vertrag_Reha/Unterschrift/Fehlende_Reha_Unterschriften_Liste.asp", Health),
	cap("missing-insurance-data", "compliance", "Prescriptions with missing health-insurance data", http.MethodGet, "/Krankenkasse/Krankenkasse_pruefen.asp", Health, fixedQuery("Art", "2")),
	cap("missing-attendee-signatures", "compliance", "Active prescriptions with missing attendee signatures", http.MethodGet, "/Abrechnung_Reha_Digital/Abrechnung_Reha_Unterschriften_Aktive_pruefen.asp", Health),
	cap("missing-instructor-signatures", "compliance", "Course instructors with missing signatures", http.MethodGet, "/Abrechnung_Reha_Digital/Abrechnung_Reha_Unterschriften_Kursleiter_pruefen.asp", Health),
	cap("billing-number-mismatches", "compliance", "Visit and prescription billing-number mismatches", http.MethodGet, "/Abrechnung_Reha_Digital/Abrechnungsnummern_Digital_Zeit_VO_pruefen.asp", Health),
	cap("prescription-unit-overruns", "compliance", "Prescriptions exceeding ordered exercise units", http.MethodGet, "/Abrechnung_Reha_Digital/Abrechnung_Reha_Digital_Menge_Zeit_pruefen.asp", Health),
	cap("duplicate-daily-units", "compliance", "More than one exercise unit for a person on one day", http.MethodGet, "/Abrechnung_Reha_Digital/Abrechnung_Reha_Digital_Doppelte_Menge_Tag_pruefen.asp", Health),

	// Explicitly isolated financial reads.
	cap("member-bank-export", "financial", "Mandate ID, IBAN and BIC export", http.MethodGet, "/Excel/Sonstiges/E_Mitglieder_IBAN.asp", Financial),
	cap("private-billing-manual-archive", "financial", "Archive of manually billed private cases", http.MethodGet, "/Abrechnung_Reha_Privat/Manuell/Reha_Privat_Manuell_Abgerechnete_Liste.asp", Financial),
	cap("private-billing-digital-archive", "financial", "Archive of digitally billed private cases", http.MethodGet, "/Abrechnung_Reha_Privat/Digital/Reha_Privat_Digital_Abgerechnete_Liste.asp", Financial),
	cap("digital-billing-complete-archive", "financial", "Archive of completed digital full billings", http.MethodGet, "/Abrechnung_Reha_Digital/Aktive/Digital_Reha_Aktive_Abgerechnete_Liste.asp", Financial,
		query("ik", "IK", IKNumber, true)),
	cap("digital-billing-final-archive", "financial", "Archive of completed digital final billings", http.MethodGet, "/Abrechnung_Reha_Digital/Abgelaufene/Digital_Reha_Abgelaufene_Abgerechnete_Liste.asp", Financial,
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
	if err != nil || parsed.IsAbs() || !ValidateAdminURL(capability.Method, parsed) {
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
