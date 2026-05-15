package cli

import (
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestStringVarTrimsDefaultAndParsedValue(t *testing.T) {
	fs := NewFlagSet("trim")
	var got string
	StringVar(fs, &got, "name", "  dev  ", "name")
	if got != "dev" {
		t.Fatalf("default value = %q, want %q", got, "dev")
	}

	if err := fs.Parse([]string{"-name", "  prod  "}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != "prod" {
		t.Fatalf("parsed value = %q, want %q", got, "prod")
	}
}

func TestStringVarSupportsAliasBinding(t *testing.T) {
	fs := NewFlagSet("trim")
	var got string
	StringVarAliases(fs, &got, "", "request", "request", "r")

	if err := fs.Parse([]string{"-r", "  sample  "}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != "sample" {
		t.Fatalf("alias value = %q, want %q", got, "sample")
	}
}

func TestPrintFlagDefaultsCombinesAliases(t *testing.T) {
	fs := NewFlagSet("help")
	var env string
	var recursive bool
	StringVarAliases(fs, &env, "", "Environment name to use", "env", "e")
	BoolVarAliases(fs, &recursive, false, "Recursively scan workspace", "recursive", "R")

	var out strings.Builder
	PrintFlagDefaults(&out, fs)
	got := out.String()

	for _, want := range []string{
		"-e, --env string",
		"Environment name to use",
		"-R, --recursive",
		"Recursively scan workspace",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected help to contain %q, got %q", want, got)
		}
	}
	if strings.Contains(got, "Alias for --") {
		t.Fatalf("expected aliases to be folded, got %q", got)
	}
}

func TestPrintFlagDefaultsShowsLiteralStringDefaults(t *testing.T) {
	fs := NewFlagSet("help")
	var code string
	var disabled string
	var retries int
	StringVar(fs, &code, "code", "0", "Status code")
	StringVar(fs, &disabled, "disabled", "false", "Disabled marker")
	fs.IntVar(&retries, "retries", 0, "Retry count")

	var out strings.Builder
	PrintFlagDefaults(&out, fs)
	got := out.String()

	for _, want := range []string{
		"--code string",
		"Status code (default 0)",
		"--disabled string",
		"Disabled marker (default false)",
		"--retries int",
		"Retry count",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("expected help to contain %q, got %q", want, got)
		}
	}
	if strings.Contains(got, "Retry count (default 0)") {
		t.Fatalf("expected numeric zero default to be hidden, got %q", got)
	}
}

func TestPrintFlagDefaultsUsesAvailableColumns(t *testing.T) {
	t.Setenv("COLUMNS", "110")

	fs := NewFlagSet("help")
	var endpoint string
	StringVarAliases(
		fs,
		&endpoint,
		"",
		"OTLP collector endpoint used when @trace is enabled",
		"trace-otel-endpoint",
		"toe",
	)

	var out strings.Builder
	PrintFlagDefaults(&out, fs)
	got := out.String()
	line := lineWith(got, "--trace-otel-endpoint")
	if !strings.Contains(line, "OTLP collector endpoint") {
		t.Fatalf("expected long flag and help on one line, got %q", got)
	}
	assertLinesFit(t, got, 110)
}

func TestPrintFlagDefaultsWrapsWithinAvailableColumns(t *testing.T) {
	t.Setenv("COLUMNS", "49")

	fs := NewFlagSet("help")
	var endpoint string
	StringVarAliases(
		fs,
		&endpoint,
		"",
		"OTLP collector endpoint used when @trace is enabled",
		"trace-otel-endpoint",
		"toe",
	)

	var out strings.Builder
	PrintFlagDefaults(&out, fs)
	got := out.String()
	assertLinesFit(t, got, 49)
}

func lineWith(s, sub string) string {
	for _, line := range strings.Split(s, "\n") {
		if strings.Contains(line, sub) {
			return line
		}
	}
	return ""
}

func assertLinesFit(t *testing.T, s string, width int) {
	t.Helper()

	for i, line := range strings.Split(strings.TrimRight(s, "\n"), "\n") {
		if got := runewidth.StringWidth(line); got > width {
			t.Fatalf("line %d exceeds width: got %d want <= %d\n%s", i+1, got, width, s)
		}
	}
}
