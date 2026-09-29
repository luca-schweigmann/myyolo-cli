package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/output"
	"github.com/luca-schweigmann/myyolo-cli/internal/transport"
)

var errProfileMissing = errors.New("Zugangsprofil fehlt")

// errorCode is one stable, machine-readable failure class. Callers such as the
// point-analytics runtime may rely on Code and Exit; ActionDE is team-facing.
type errorCode struct {
	Code     string `json:"code"`
	Exit     int    `json:"exit_code"`
	Severity string `json:"severity"`
	ActionDE string `json:"action_de"`
}

// errorCodes is the published contract. Codes and exit codes never change
// meaning; new classes get new numbers. Unclassified errors exit with 1.
var errorCodes = []errorCode{
	{"NETWORK_TLS_CERTIFICATE_INVALID", 10, "failed", "Das Zertifikat des Anbieters ist ungültig oder abgelaufen. Das liegt beim Anbieter; später erneut versuchen und bei Dauer den Anbieter informieren."},
	{"NETWORK_TIMEOUT", 11, "failed", "Der Anbieter hat nicht rechtzeitig geantwortet. Später erneut versuchen."},
	{"NETWORK_UNREACHABLE", 12, "failed", "Der Anbieter ist nicht erreichbar. Internetverbindung prüfen und später erneut versuchen."},
	{"AUTH_PROFILE_MISSING", 20, "failed", "Für dieses Profil sind keine Zugangsdaten gespeichert. myyolo auth login ausführen."},
	{"AUTH_REJECTED", 21, "failed", "Der Anbieter hat die Anmeldung abgelehnt. Zugangsdaten prüfen und myyolo auth login erneut ausführen."},
	{"AUTH_SESSION_EXPIRED", 22, "failed", "Die Sitzung beim Anbieter ist abgelaufen und ließ sich nicht erneuern. myyolo auth check --force-relogin ausführen."},
	{"PROVIDER_RATE_LIMITED", 30, "failed", "Der Anbieter bremst Anfragen. Nicht sofort wiederholen; der nächste geplante Lauf versucht es erneut."},
	{"PROVIDER_CAPTCHA", 31, "failed", "Der Anbieter verlangt ein CAPTCHA. Einmal im Browser anmelden, danach erneut versuchen."},
	{"PROVIDER_HTTP_ERROR", 32, "failed", "Der Anbieter hat mit einem Fehler geantwortet. Später erneut versuchen."},
	{"PROVIDER_SCHEMA_DRIFT", 40, "failed", "Der Anbieter hat sein Datenformat geändert. Die CLI muss angepasst werden; nicht wiederholen."},
	{"REQUEST_BUDGET_EXHAUSTED", 41, "failed", "Das Anfragebudget dieses Laufs ist aufgebraucht. Nicht erhöhen; den nächsten Lauf abwarten."},
}

func lookupErrorCode(code string) errorCode {
	for _, item := range errorCodes {
		if item.Code == code {
			return item
		}
	}
	panic("unknown error code " + code)
}

// classifyError maps a command error onto the published contract. It returns
// false for errors that already carry their own code or are not classified.
func classifyError(err error) (errorCode, bool) {
	var certificate *tls.CertificateVerificationError
	var invalid x509.CertificateInvalidError
	var authority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var netErr net.Error
	var opErr *net.OpError
	var dnsErr *net.DNSError
	code := ""
	switch {
	case errors.As(err, &certificate), errors.As(err, &invalid), errors.As(err, &authority), errors.As(err, &hostname):
		code = "NETWORK_TLS_CERTIFICATE_INVALID"
	case errors.Is(err, context.DeadlineExceeded), errors.As(err, &netErr) && netErr.Timeout():
		code = "NETWORK_TIMEOUT"
	case errors.As(err, &dnsErr), errors.As(err, &opErr):
		code = "NETWORK_UNREACHABLE"
	case errors.Is(err, errProfileMissing):
		code = "AUTH_PROFILE_MISSING"
	case errors.Is(err, transport.ErrAuthentication), errors.Is(err, admin.ErrAuthentication):
		code = "AUTH_REJECTED"
	case errors.Is(err, transport.ErrSessionExpired), errors.Is(err, admin.ErrSessionExpired):
		code = "AUTH_SESSION_EXPIRED"
	case errors.Is(err, transport.ErrRateLimited), errors.Is(err, admin.ErrRateLimited):
		code = "PROVIDER_RATE_LIMITED"
	case errors.Is(err, admin.ErrCAPTCHA):
		code = "PROVIDER_CAPTCHA"
	case errors.Is(err, transport.ErrHTTPStatus), errors.Is(err, admin.ErrHTTPStatus):
		code = "PROVIDER_HTTP_ERROR"
	case errors.Is(err, transport.ErrSchemaDrift), errors.Is(err, admin.ErrSchemaDrift):
		code = "PROVIDER_SCHEMA_DRIFT"
	case errors.Is(err, transport.ErrRequestBudget), errors.Is(err, admin.ErrRequestBudget):
		code = "REQUEST_BUDGET_EXHAUSTED"
	default:
		return errorCode{}, false
	}
	return lookupErrorCode(code), true
}

// reportError writes the stderr contract: "Fehler: CODE: detail" plus a
// "Zu tun:" line for classified errors, and returns the process exit code.
func reportError(stderr io.Writer, err error) int {
	code, ok := classifyError(err)
	if !ok {
		fmt.Fprintln(stderr, "Fehler:", err)
		return 1
	}
	fmt.Fprintf(stderr, "Fehler: %s: %v\nZu tun: %s\n", code.Code, err, code.ActionDE)
	return code.Exit
}

func printErrorCodes(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("errors", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	format := flags.String("format", "table", "table|json|csv")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		return errors.New("ungültige Optionen für errors")
	}
	return output.Write(stdout, errorCodes, *format)
}
