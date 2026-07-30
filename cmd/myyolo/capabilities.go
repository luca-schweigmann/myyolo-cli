package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"

	"github.com/luca-schweigmann/myyolo-cli/internal/admin"
	"github.com/luca-schweigmann/myyolo-cli/internal/output"
	"github.com/luca-schweigmann/myyolo-cli/internal/readcatalog"
	"github.com/luca-schweigmann/myyolo-cli/internal/secrets"
	"github.com/luca-schweigmann/myyolo-cli/internal/store"
)

type catalogRow struct {
	Name           string             `json:"name"`
	Group          string             `json:"group"`
	Sensitivity    string             `json:"sensitivity"`
	Method         string             `json:"method"`
	Path           string             `json:"path"`
	ParameterFlags string             `json:"parameter_flags"`
	Parameters     []catalogParameter `json:"parameters"`
	Description    string             `json:"description"`
}

type catalogParameter struct {
	Flag     string   `json:"flag"`
	Kind     string   `json:"kind"`
	Required bool     `json:"required"`
	Allowed  []string `json:"allowed,omitempty"`
}

type capabilityDryRun struct {
	Capability         string `json:"capability"`
	Group              string `json:"group"`
	Sensitivity        string `json:"sensitivity"`
	Method             string `json:"method"`
	Path               string `json:"path"`
	Parameters         string `json:"parameters"`
	RemoteRequests     int    `json:"remote_requests"`
	MinimumDelay       string `json:"minimum_delay"`
	LocalDatabaseWrite bool   `json:"local_database_write"`
}

type dataScopeFlags struct {
	personal  *bool
	health    *bool
	financial *bool
}

func addDataScopeFlags(flags *flag.FlagSet) dataScopeFlags {
	return dataScopeFlags{
		personal: flags.Bool(
			"include-personal-data",
			false,
			"allow member names and identifiers",
		),
		health: flags.Bool(
			"include-health-data",
			false,
			"allow Reha, prevention and prescription data",
		),
		financial: flags.Bool(
			"include-financial-data",
			false,
			"allow bank and billing data",
		),
	}
}

func (scope dataScopeFlags) authorize(sensitivity readcatalog.Sensitivity) error {
	switch sensitivity {
	case readcatalog.Aggregate:
		return nil
	case readcatalog.Personal:
		if !*scope.personal {
			return errors.New("this capability requires --include-personal-data")
		}
	case readcatalog.Health:
		if !*scope.personal || !*scope.health {
			return errors.New(
				"this capability requires --include-personal-data and --include-health-data",
			)
		}
	case readcatalog.Financial:
		if !*scope.personal || !*scope.financial {
			return errors.New(
				"this capability requires --include-personal-data and --include-financial-data",
			)
		}
	default:
		return fmt.Errorf("unsupported sensitivity %q", sensitivity)
	}
	return nil
}

func printCatalog(args []string, stdout io.Writer) error {
	flags := flag.NewFlagSet("catalog", flag.ContinueOnError)
	group := flags.String("group", "", "limit the local catalogue to one group")
	format := flags.String("format", "table", "table, json or csv")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("catalog does not accept positional arguments")
	}
	var rows []catalogRow
	for _, capability := range readcatalog.List() {
		if *group != "" && capability.Group != *group {
			continue
		}
		rows = append(rows, catalogRowFor(capability))
	}
	if len(rows) == 0 && *group != "" {
		return fmt.Errorf("unknown or empty capability group %q", *group)
	}
	return output.Write(stdout, rows, *format)
}

func catalogRowFor(capability readcatalog.Capability) catalogRow {
	var parameters []catalogParameter
	for _, parameter := range capability.Parameters {
		if parameter.Name == "" {
			continue
		}
		parameters = append(parameters, catalogParameter{
			Flag:     "--" + parameter.Name,
			Kind:     string(parameter.Kind),
			Required: parameter.Required,
			Allowed:  parameter.Allowed,
		})
	}
	return catalogRow{
		Name:           capability.Name,
		Group:          capability.Group,
		Sensitivity:    string(capability.Sensitivity),
		Method:         capability.Method,
		Path:           capability.Path,
		ParameterFlags: parameterSummary(capability),
		Parameters:     parameters,
		Description:    capability.Description,
	}
}

func parameterSummary(capability readcatalog.Capability) string {
	var parameters []string
	for _, parameter := range capability.Parameters {
		if parameter.Name == "" {
			continue
		}
		value := "--" + parameter.Name
		if !parameter.Required {
			value += " (optional)"
		}
		parameters = append(parameters, value)
	}
	sort.Strings(parameters)
	return strings.Join(parameters, ", ")
}

func collectCapability(
	ctx context.Context,
	name string,
	args []string,
	stdout io.Writer,
) error {
	flags := flag.NewFlagSet("collect "+name, flag.ContinueOnError)
	profile := flags.String("profile", "default", "credential profile")
	dbPath := flags.String("db", defaultDBPath(), "SQLite database path")
	delay := flags.Duration("delay", admin.DefaultDelay, "minimum delay between admin requests")
	requestBudget := flags.Int(
		"request-budget",
		admin.DefaultFetchBudget,
		"maximum requests including session probe, login and relogin",
	)
	format := flags.String("format", "json", "table, json or csv")
	dryRun := flags.Bool("dry-run", false, "validate locally without credentials or HTTP")
	scope := addDataScopeFlags(flags)
	values := addCapabilityValueFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("collect accepts exactly one capability and no extra positional arguments")
	}
	if *delay < admin.DefaultDelay {
		return fmt.Errorf("--delay must be at least %s", admin.DefaultDelay)
	}
	if *requestBudget < 1 || *requestBudget > admin.DefaultFetchBudget {
		return fmt.Errorf(
			"--request-budget must be between 1 and %d for collect",
			admin.DefaultFetchBudget,
		)
	}
	if err := secrets.ValidateProfile(*profile); err != nil {
		return err
	}
	request, err := readcatalog.Build(name, values())
	if err != nil {
		return err
	}
	if err := scope.authorize(request.Capability.Sensitivity); err != nil {
		return err
	}
	if *dryRun {
		return output.Write(stdout, capabilityDryRun{
			Capability:         request.Capability.Name,
			Group:              request.Capability.Group,
			Sensitivity:        string(request.Capability.Sensitivity),
			Method:             request.Capability.Method,
			Path:               request.Capability.Path,
			Parameters:         parameterSummary(request.Capability),
			RemoteRequests:     0,
			MinimumDelay:       delay.String(),
			LocalDatabaseWrite: false,
		}, *format)
	}

	secretStore := secrets.NewKeyringStore()
	credentials, err := secretStore.LoadCredentials(*profile)
	if err != nil {
		if secrets.IsNotFound(err) {
			return fmt.Errorf("profile %q is not configured; run myyolo auth login", *profile)
		}
		return err
	}
	session, err := secretStore.LoadAdminSession(*profile)
	if err != nil && !secrets.IsNotFound(err) {
		return err
	}
	client, err := admin.New(
		&http.Client{},
		admin.WithDelay(*delay),
		admin.WithRequestBudget(*requestBudget),
	)
	if err != nil {
		return err
	}
	page, nextSession, err := client.FetchCapability(ctx, credentials, session, request)
	if err != nil {
		return err
	}
	if err := secretStore.SaveAdminSession(*profile, nextSession); err != nil {
		return err
	}
	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	result, err := db.ImportAdminPage(ctx, page)
	if err != nil {
		return err
	}
	return output.Write(stdout, result, *format)
}

func addCapabilityValueFlags(flags *flag.FlagSet) func() map[string]string {
	values := map[string]*string{}
	for _, item := range []struct {
		name        string
		description string
	}{
		{"from", "range start (YYYY-MM-DD)"},
		{"to", "range end (YYYY-MM-DD)"},
		{"date", "single date (YYYY-MM-DD)"},
		{"year", "calendar year"},
		{"week-from", "first ISO week"},
		{"week-to", "last ISO week"},
		{"threshold", "non-negative threshold"},
		{"member-id", "positive member identifier"},
		{"course-id", "positive course identifier"},
		{"planner", "planner mode"},
		{"population", "member-search population"},
		{"search", "member-search text"},
		{"referrer-id", "optional referrer identifier"},
		{"ik", "nine-digit institution code"},
	} {
		values[item.name] = flags.String(item.name, "", item.description)
	}
	return func() map[string]string {
		result := make(map[string]string)
		for name, value := range values {
			if strings.TrimSpace(*value) != "" {
				result[name] = strings.TrimSpace(*value)
			}
		}
		return result
	}
}

func printCapabilityReport(
	ctx context.Context,
	name string,
	args []string,
	stdout io.Writer,
) error {
	capability, ok := readcatalog.Lookup(name)
	if !ok {
		return fmt.Errorf("unknown capability %q", name)
	}
	flags := flag.NewFlagSet("report capability "+name, flag.ContinueOnError)
	dbPath := flags.String("db", defaultDBPath(), "SQLite database path")
	format := flags.String("format", "table", "table, json or csv")
	scope := addDataScopeFlags(flags)
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return errors.New("report capability accepts one capability and no extra arguments")
	}
	if err := scope.authorize(capability.Sensitivity); err != nil {
		return err
	}
	db, err := store.Open(ctx, *dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	report, err := db.CurrentAdminRecords(ctx, readcatalog.RouteKey(capability.Name))
	if err != nil {
		return err
	}
	return output.Write(stdout, report, *format)
}
