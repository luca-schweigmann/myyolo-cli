package main

import (
	"context"
	"fmt"
	"io"
	"os"
)

var version = "dev"

const usage = `myyolo - read-only myYOLO collector and reporting CLI

Usage:
  myyolo auth login [--profile NAME] --partner NUMBER --username USER [--password-stdin]
  myyolo auth status [--profile NAME]
  myyolo auth logout [--profile NAME]
  myyolo sync [--profile NAME] [--db PATH]
  myyolo db init [--db PATH]
  myyolo import mysign --file PATH [--db PATH]
  myyolo report summary|courses|days|hours|sessions [--format table|json|csv] [--db PATH]
  myyolo report members --include-personal-data [--format table|json|csv] [--db PATH]
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
		case "status":
			return authStatus(args[2:], stdout)
		case "logout":
			return authLogout(args[2:], stdout)
		}
	case "sync":
		return syncMyYOLO(ctx, args[1:], stdout)
	case "db":
		if len(args) >= 2 && args[1] == "init" {
			return initDB(ctx, args[2:], stdout)
		}
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
		_, err := fmt.Fprintln(stdout, version)
		return err
	}
	return fmt.Errorf("unknown command\n\n%s", usage)
}
