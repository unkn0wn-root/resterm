package cli

import (
	"errors"
	"io"
	"slices"
	"strings"
	"testing"

	"github.com/mattn/go-runewidth"
)

func TestFlagSetParsesInterspersedArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantRequest string
		wantTags    []string
		wantArgs    []string
		wantAll     bool
	}{
		{
			name:        "flags before positional",
			args:        []string{"-request", "createPost", "requests.http"},
			wantRequest: "createPost",
			wantArgs:    []string{"requests.http"},
		},
		{
			name:        "flags after positional",
			args:        []string{"requests.http", "-r", "createPost"},
			wantRequest: "createPost",
			wantArgs:    []string{"requests.http"},
		},
		{
			name:        "inline flag after positional",
			args:        []string{"requests.http", "--request=createPost"},
			wantRequest: "createPost",
			wantArgs:    []string{"requests.http"},
		},
		{
			name:        "stdin before flag",
			args:        []string{"-", "--request", "health"},
			wantRequest: "health",
			wantArgs:    []string{"-"},
		},
		{
			name:        "flag value starts with dash",
			args:        []string{"requests.http", "--request", "-internal"},
			wantRequest: "-internal",
			wantArgs:    []string{"requests.http"},
		},
		{
			name:     "repeated and boolean flags preserve order",
			args:     []string{"first", "--tag", "one", "middle", "-a", "--tag=two", "last"},
			wantTags: []string{"one", "two"},
			wantArgs: []string{"first", "middle", "last"},
			wantAll:  true,
		},
		{
			name:     "terminator keeps remaining args positional",
			args:     []string{"requests.http", "-a", "--", "--request", "ignored"},
			wantArgs: []string{"requests.http", "--request", "ignored"},
			wantAll:  true,
		},
		{
			name:        "terminator is consumed as a flag value",
			args:        []string{"requests.http", "--request", "--", "-a"},
			wantRequest: "--",
			wantArgs:    []string{"requests.http"},
			wantAll:     true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewFlagSet("test")
			var request string
			var tags []string
			var all bool
			fs.StringVarAliases(&request, "", "request", "request", "r")
			fs.StringListVarAliases(&tags, "tag", "tag", "g")
			fs.BoolVarAliases(&all, false, "all", "all", "a")

			if err := fs.Parse(test.args); err != nil {
				t.Fatalf("Parse(%q): %v", test.args, err)
			}
			if request != test.wantRequest {
				t.Errorf("request = %q, want %q", request, test.wantRequest)
			}
			if !slices.Equal(tags, test.wantTags) {
				t.Errorf("tags = %q, want %q", tags, test.wantTags)
			}
			if all != test.wantAll {
				t.Errorf("all = %t, want %t", all, test.wantAll)
			}
			if got := fs.Args(); !slices.Equal(got, test.wantArgs) {
				t.Errorf("Args() = %q, want %q", got, test.wantArgs)
			}
			if got := fs.NArg(); got != len(test.wantArgs) {
				t.Errorf("NArg() = %d, want %d", got, len(test.wantArgs))
			}
			for i, want := range test.wantArgs {
				if got := fs.Arg(i); got != want {
					t.Errorf("Arg(%d) = %q, want %q", i, got, want)
				}
			}
			if got := fs.Arg(len(test.wantArgs)); got != "" {
				t.Errorf("Arg(%d) = %q, want empty string", len(test.wantArgs), got)
			}
		})
	}
}

func TestFlagSetReportsErrorsAfterPositionalArgs(t *testing.T) {
	t.Run("help", func(t *testing.T) {
		fs := NewFlagSet("test")
		if err := fs.Parse([]string{"requests.http", "--help"}); !errors.Is(err, ErrHelp) {
			t.Fatalf("Parse() error = %v, want help error", err)
		}
	})

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "unknown flag",
			args: []string{"requests.http", "--unknown"},
			want: "flag provided but not defined",
		},
		{
			name: "missing value",
			args: []string{"requests.http", "--request"},
			want: "needs an argument",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewFlagSet("test")
			var request string
			fs.StringVar(&request, "request", "", "request")

			err := fs.Parse(test.args)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("Parse(%q) error = %v, want %q", test.args, err, test.want)
			}
		})
	}
}

func TestUnexpectedArgs(t *testing.T) {
	fs := NewSubcommandFlagSet("resterm", "history export", io.Discard)
	var out string
	fs.StringVar(&out, "out", "", "output")

	if err := fs.Parse([]string{"--out", "dump.json"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if err := fs.UnexpectedArgs(); err != nil {
		t.Fatalf("UnexpectedArgs() = %v, want nil", err)
	}

	if err := fs.Parse([]string{"--out", "dump.json", "extra", "args"}); err != nil {
		t.Fatalf("Parse: %v", err)
	}
	err := fs.UnexpectedArgs()
	if err == nil || err.Error() != "history export: unexpected args: extra args" {
		t.Fatalf("UnexpectedArgs() = %v, want the command name and both args", err)
	}
}

func TestWasSet(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "absent", args: []string{"requests.http"}, want: false},
		{name: "long name", args: []string{"--request", "health"}, want: true},
		{name: "short alias", args: []string{"-r", "health"}, want: true},
		{name: "inline value", args: []string{"--request=health"}, want: true},
		{name: "after a positional", args: []string{"requests.http", "-r", "health"}, want: true},
		{name: "value equal to the default", args: []string{"--request", "auto"}, want: true},
		{name: "another flag only", args: []string{"-a"}, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fs := NewFlagSet("test")
			var request string
			var all bool
			fs.StringVarAliases(&request, "auto", "request", "request", "r")
			fs.BoolVarAliases(&all, false, "all", "all", "a")

			if err := fs.Parse(test.args); err != nil {
				t.Fatalf("Parse(%q): %v", test.args, err)
			}
			if got := fs.WasSet("request"); got != test.want {
				t.Errorf("WasSet(\"request\") = %t, want %t", got, test.want)
			}
		})
	}
}

func TestStringVarTrimsDefaultAndParsedValue(t *testing.T) {
	fs := NewFlagSet("trim")
	var got string
	fs.StringVar(&got, "name", "  dev  ", "name")
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
	fs.StringVarAliases(&got, "", "request", "request", "r")

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
	fs.StringVarAliases(&env, "", "Environment name to use", "env", "e")
	fs.BoolVarAliases(&recursive, false, "Recursively scan workspace", "recursive", "R")

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
	fs.StringVar(&code, "code", "0", "Status code")
	fs.StringVar(&disabled, "disabled", "false", "Disabled marker")
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
	fs.StringVarAliases(
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
	fs.StringVarAliases(
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
	for line := range strings.SplitSeq(s, "\n") {
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
