package transport

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const mySignHost = "sign.azh-myyolo.info"

var allowed = map[string]map[string]struct{}{
	mySignHost: {
		http.MethodPost + " /Home/LoginUser":   {},
		http.MethodPost + " /Home/GetListData": {},
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
	if parsed.RawQuery != "" || parsed.ForceQuery {
		return fmt.Errorf("blocked unapproved query string")
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
	if _, ok := routes[key]; !ok {
		return fmt.Errorf("blocked unapproved route %s on %s", key, host)
	}
	return nil
}
