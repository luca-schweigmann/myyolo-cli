package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
)

var version = "dev"

const usage = `myyolo - read-only myYOLO collector and reporting CLI

Usage:
  myyolo auth login [--profile NAME] [--source mysign|admin|all] --partner NUMBER --username USER [--password-stdin]
  myyolo auth check [--profile NAME] [--source mysign|admin|all]
  myyolo auth status [--profile NAME]
  myyolo auth logout [--profile NAME]
  myyolo sync [mysign] [--profile NAME] [--db PATH]
  myyolo sync admin [--profile NAME] [--db PATH] [--delay 2s] [--request-budget 1..5]
  myyolo discover admin [--profile NAME] [--db PATH] [--delay 2s] [--request-budget 1..10]
  myyolo db init [--db PATH]
  myyolo db status [--format table|json|csv] [--db PATH]
  myyolo doctor [--profile NAME] [--db PATH]
  myyolo import mysign --file PATH [--db PATH]
  myyolo report summary|courses|days|hours|sessions [--as-of RFC3339] [--format table|json|csv] [--db PATH]
  myyolo report admin-capabilities|admin-reha-hours|admin-course-months|admin-missing-signatures [--format table|json|csv] [--db PATH]
  myyolo report members --include-personal-data [--as-of RFC3339] [--format table|json|csv] [--db PATH]
  myyolo report admin-reha-attendance|admin-missing-signature-members|admin-records --include-personal-data [--route EXACT_PATH] [--format table|json|csv] [--db PATH]
  myyolo version

Credentials and cached sessions are stored in the operating-system keyring.
The CLI never calls a myYOLO write endpoint.`

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		_, err := fmt.Fprintln(stdout, usage)
		return err
	}

	switch args[0] {
	case "auth":
		if len(args) < 2 {
			break
		}
		switch args[1] {
		case "login":
			return authLogin(ctx, args[2:], stdin, stdout)
		case "check":
			return authCheck(ctx, args[2:], stdout)
		case "status":
			return authStatus(args[2:], stdout)
		case "logout":
			return authLogout(args[2:], stdout)
		}
	case "sync":
		if len(args) >= 2 && args[1] == "admin" {
			return syncAdmin(ctx, args[2:], stdout)
		}
		if len(args) >= 2 && args[1] == "mysign" {
			return syncMyYOLO(ctx, args[2:], stdout)
		}
		return syncMyYOLO(ctx, args[1:], stdout)
	case "discover":
		if len(args) >= 2 && args[1] == "admin" {
			return discoverAdmin(ctx, args[2:], stdout)
		}
	case "db":
		if len(args) >= 2 && args[1] == "init" {
			return initDB(ctx, args[2:], stdout)
		}
		if len(args) >= 2 && args[1] == "status" {
			return dbStatus(ctx, args[2:], stdout)
		}
	case "doctor":
		return doctor(ctx, args[1:], stdout)
	case "import":
		if len(args) >= 2 && args[1] == "mysign" {
			return importMySign(ctx, args[2:], stdout)
		}
	case "report":
		if len(args) >= 2 {
			return printReport(ctx, args[1], args[2:], stdout)
		}
	case "stats":
		if len(args) >= 2 && args[1] == "summary" {
			return printReport(ctx, "summary", args[2:], stdout)
		}
	case "version":
		_, err := fmt.Fprintln(stdout, buildVersion())
		return err
	}
	return fmt.Errorf("unknown command\n\n%s", usage)
}

func buildVersion() string {
	if version != "" && version != "dev" {
		return version
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "dev"
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" && setting.Value != "" {
			revision := setting.Value
			if len(revision) > 12 {
				revision = revision[:12]
			}
			return "dev+" + strings.ToLower(revision)
		}
	}
	return "dev"
}
