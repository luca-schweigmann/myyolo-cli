package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

var cliBinary string

func TestMain(m *testing.M) {
	directory, err := os.MkdirTemp("", "myyolo-e2e-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	cliBinary = filepath.Join(directory, "myyolo")
	build := exec.Command("go", "build", "-o", cliBinary, ".")
	build.Stdout = os.Stdout
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		os.RemoveAll(directory)
		os.Exit(1)
	}
	code := m.Run()
	os.RemoveAll(directory)
	os.Exit(code)
}

func runCLI(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	command := exec.CommandContext(ctx, cliBinary, args...)
	command.Stdin = stdin
	command.Stdout = stdout
	var stderr bytes.Buffer
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("%w: %s", err, stderr.String())
	}
	return nil
}
