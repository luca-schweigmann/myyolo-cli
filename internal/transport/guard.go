package transport

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const mySignHost = "sign.azh-myyolo.info"
const adminHost = "www.azh-myyolo.info"

type routePolicy struct {
	rawQueries map[string]struct{}
}

func queryPolicy(values ...string) routePolicy {
	queries := make(map[string]struct{}, len(values))
	for _, value := range values {
		queries[value] = struct{}{}
	}
	return routePolicy{rawQueries: queries}
}

var allowed = map[string]map[string]routePolicy{
	mySignHost: {
		http.MethodPost + " /Home/LoginUser":   queryPolicy(""),
		http.MethodPost + " /Home/GetListData": queryPolicy(""),
	},
	adminHost: {
		http.MethodPost + " /Anmelden.asp":                                                    queryPolicy("vw="),
		http.MethodGet + " /LoginHandler.asp":                                                 queryPolicy(""),
		http.MethodGet + " /Start.asp":                                                        queryPolicy(""),
		http.MethodGet + " /start.asp":                                                        queryPolicy(""),
		http.MethodGet + " /start_Anwesenheit.asp":                                            queryPolicy(""),
		http.MethodGet + " /Anwesenheit/Anwesend_Aktuell_Liste.asp":                           queryPolicy(""),
		http.MethodGet + " /Anwesenheit/Anwesend_Heute_Liste.asp":                             queryPolicy(""),
		http.MethodGet + " /Anwesenheit/Anwesend_Datum_Liste.asp":                             queryPolicy(""),
		http.MethodGet + " /Anwesenheit/Reha_Anwesend_Datum_Liste.asp":                        queryPolicy(""),
		http.MethodGet + " /Statistiken/Kursplaner/Kurs_Teilnehmer_anwesend_monatlich.asp":    queryPolicy(""),
		http.MethodGet + " /Vertrag_Reha/Unterschrift/Fehlende_Reha_Unterschriften_Liste.asp": queryPolicy(""),
	},
}

// ValidateReadRequest is the final network gate. Only explicitly classified
// reads are allowed. A POST is not assumed to be a write or read from its verb;
// the exact observed route must be listed.
func ValidateReadRequest(method, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse request URL: %w", err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("blocked non-HTTPS request")
	}
	if parsed.User != nil {
		return fmt.Errorf("blocked credentials embedded in URL")
	}
	if parsed.Fragment != "" {
		return fmt.Errorf("blocked URL fragment")
	}

	host := strings.ToLower(parsed.Hostname())
	routes, ok := allowed[host]
	if !ok {
		return fmt.Errorf("blocked unapproved host %q", host)
	}
	if port := parsed.Port(); port != "" && port != "443" {
		return fmt.Errorf("blocked unapproved port %q", port)
	}

	key := strings.ToUpper(method) + " " + parsed.EscapedPath()
	policy, ok := routes[key]
	if !ok {
		return fmt.Errorf("blocked unapproved route %s on %s", key, host)
	}
	if _, ok := policy.rawQueries[parsed.RawQuery]; !ok || parsed.ForceQuery {
		return fmt.Errorf("blocked unapproved query string on %s", key)
	}
	return nil
}
