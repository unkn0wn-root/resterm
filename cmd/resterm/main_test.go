package main

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunVersionFlag(t *testing.T) {
	out, errOut, err := captureRunIO(t, func() error {
		return run([]string{"--version"})
	})
	if err != nil {
		t.Fatalf("run --version: %v", err)
	}
	if errOut != "" {
		t.Fatalf("expected empty stderr, got %q", errOut)
	}
	if !strings.Contains(out, "resterm ") {
		t.Fatalf("expected version header in stdout, got %q", out)
	}
}

func TestRunHelpFlag(t *testing.T) {
	out, errOut, err := captureRunIO(t, func() error {
		return run([]string{"--help"})
	})
	if err != nil {
		t.Fatalf("run --help: %v", err)
	}
	if strings.TrimSpace(out) != "" {
		t.Fatalf("expected empty stdout, got %q", out)
	}
	if !strings.Contains(errOut, "Usage: resterm [flags] [file]") {
		t.Fatalf("expected usage in stderr, got %q", errOut)
	}
	if !strings.Contains(errOut, "-file") {
		t.Fatalf("expected top-level flags in help output, got %q", errOut)
	}
}

func TestRunRejectsConflictingImportFlags(t *testing.T) {
	t.Setenv("RESTERM_CONFIG_DIR", t.TempDir())
	err := run([]string{
		"--from-curl", "curl https://example.com",
		"--from-openapi", "spec.yaml",
	})
	if err == nil {
		t.Fatalf("expected conflict error")
	}
	if !strings.Contains(err.Error(), "choose either --from-curl or --from-openapi") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRejectsInvalidCompareValue(t *testing.T) {
	t.Setenv("RESTERM_CONFIG_DIR", t.TempDir())
	err := run([]string{"--compare", "dev"})
	if err == nil {
		t.Fatalf("expected compare validation error")
	}
	if !strings.Contains(err.Error(), "invalid --compare value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRejectsReservedCompareBase(t *testing.T) {
	t.Setenv("RESTERM_CONFIG_DIR", t.TempDir())
	err := run([]string{"--compare", "dev,prod", "--compare-base", "$shared"})
	if err == nil {
		t.Fatalf("expected compare-base validation error")
	}
	if !strings.Contains(err.Error(), "invalid --compare-base value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRejectsMissingFile(t *testing.T) {
	t.Setenv("RESTERM_CONFIG_DIR", t.TempDir())
	fp := filepath.Join(t.TempDir(), "missing.http")
	err := run([]string{"--file", fp})
	if err == nil {
		t.Fatalf("expected read error")
	}
	if !strings.Contains(err.Error(), "read file") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunDispatchesHistorySubcommand(t *testing.T) {
	out, errOut, err := captureRunIO(t, func() error {
		return run([]string{"history", "-h"})
	})
	if err != nil {
		t.Fatalf("run history -h: %v", err)
	}
	if strings.TrimSpace(errOut) != "" {
		t.Fatalf("expected empty stderr, got %q", errOut)
	}
	if !strings.Contains(out, "Usage: resterm history <export|import|backup|stats|check|compact> [flags]") {
		t.Fatalf("expected history usage in stdout, got %q", out)
	}
}

func TestRunInvalidFlagReturnsCodeTwo(t *testing.T) {
	err := run([]string{"--definitely-not-a-flag"})
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if c := exitCode(err); c != 2 {
		t.Fatalf("expected exit code 2, got %d (err=%v)", c, err)
	}
	if !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("unexpected parse error: %v", err)
	}
}

func captureRunIO(t *testing.T, fn func() error) (string, string, error) {
	t.Helper()

	oldOut := os.Stdout
	oldErr := os.Stderr
	outR, outW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stdout: %v", err)
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe stderr: %v", err)
	}

	os.Stdout = outW
	os.Stderr = errW
	defer func() {
		os.Stdout = oldOut
		os.Stderr = oldErr
	}()

	runErr := fn()

	_ = outW.Close()
	_ = errW.Close()

	outData, outErr := io.ReadAll(outR)
	if outErr != nil {
		t.Fatalf("read stdout: %v", outErr)
	}
	errData, errErr := io.ReadAll(errR)
	if errErr != nil {
		t.Fatalf("read stderr: %v", errErr)
	}

	_ = outR.Close()
	_ = errR.Close()
	return string(outData), string(errData), runErr
}
