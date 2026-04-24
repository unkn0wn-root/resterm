package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

func TestHandleRunSubcommandNotMatched(t *testing.T) {
	handled, err := handleRunSubcommand([]string{"history"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if handled {
		t.Fatalf("expected not handled")
	}
}

func TestHandleRunSubcommandAmbiguousFile(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(cwd)
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "run"), []byte("data"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	handled, err := handleRunSubcommand([]string{"run"})
	if !handled {
		t.Fatalf("expected run to be handled")
	}
	if err == nil {
		t.Fatalf("expected ambiguity error")
	}
}

func TestRunRunRequiresFile(t *testing.T) {
	err := runRun(nil)
	if err == nil {
		t.Fatalf("expected missing file error")
	}
	if !strings.Contains(err.Error(), "request file path is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunRunHelpFlagShowsUsage(t *testing.T) {
	stdout, stderr, err := captureRunIO(t, func() error {
		return runRun([]string{"-h"})
	})
	if err != nil {
		t.Fatalf("help flag: %v", err)
	}
	if str.Trim(stderr) != "" {
		t.Fatalf("expected empty stderr on help flag, got %q", stderr)
	}
	if !strings.Contains(stdout, "Usage: resterm run [flags] <file|->") {
		t.Fatalf("expected run usage in stdout, got %q", stdout)
	}
	if !strings.Contains(stdout, "-request") {
		t.Fatalf("expected request flag in stdout, got %q", stdout)
	}
}

func TestRunRunFlagErrorsHaveCommandPrefix(t *testing.T) {
	err := runRun([]string{"--bad"})
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if code := cli.ExitCode(err); code != 2 {
		t.Fatalf("expected exit code 2, got %d (err=%v)", code, err)
	}
	if !strings.Contains(err.Error(), "run:") {
		t.Fatalf("expected run prefix, got %q", err.Error())
	}
}

func TestRunDispatchesRunSubcommand(t *testing.T) {
	stdout, stderr, err := captureRunIO(t, func() error {
		return run([]string{"run", "-h"})
	})
	if err != nil {
		t.Fatalf("run dispatch: %v", err)
	}
	if str.Trim(stderr) != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
	if !strings.Contains(stdout, "Usage: resterm run [flags] <file|->") {
		t.Fatalf("expected run usage in stdout, got %q", stdout)
	}
}

func TestRunCmdLoadSourceFromStdinUsesSyntheticPath(t *testing.T) {
	cmd := newRunCmd()
	cmd.exec.Workspace = t.TempDir()
	cmd.in = strings.NewReader("GET https://example.com\n")

	src, err := cmd.loadSource("-")
	if err != nil {
		t.Fatalf("loadSource(-): %v", err)
	}
	if !src.Stdin {
		t.Fatalf("expected stdin source")
	}
	want := cli.StdinRunPath(cmd.exec.Workspace)
	if src.Path != want {
		t.Fatalf("unexpected stdin path: got %q want %q", src.Path, want)
	}
}

func TestParseRunDocReturnsParseError(t *testing.T) {
	_, err := cli.ParseRunDoc(cli.RunSource{
		Path: "broken.http",
		Data: []byte("# @k8s namespace=default\nGET http://example.com\n"),
	})
	if err == nil {
		t.Fatalf("expected parse error")
	}
	if !strings.Contains(err.Error(), "parse error") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmdSingleRequestSetsDefaultLine(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cmd := newRunCmd()
	cmd.stdinTTY = false
	cmd.stdoutTTY = false
	cmd.newClient = stubRunClient
	var got runner.Options
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		got = opts
		return stubRunReport(true), nil
	}

	if err := cmd.parse([]string{file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if cmd.line != 1 {
		t.Fatalf("expected selected line 1, got %d", cmd.line)
	}
	if got.Select.Line != 1 {
		t.Fatalf("expected runner select line 1, got %+v", got.Select)
	}
}

func TestRunCmdNonInteractiveListsRequestsAndReturnsUsageExit(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "many.http")
	src := strings.Join([]string{
		"### One",
		"# @name one",
		"GET https://example.com/one",
		"",
		"### Two",
		"# @name two",
		"GET https://example.com/two",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.stdinTTY = false
	cmd.stdoutTTY = false

	if err := cmd.parse([]string{file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected usage exit")
	}
	if code := cli.ExitCode(err); code != 2 {
		t.Fatalf("expected exit code 2, got %d (err=%v)", code, err)
	}
	if !strings.Contains(out.String(), "Multiple requests found in") {
		t.Fatalf("expected list output, got %q", out.String())
	}
	if !strings.Contains(out.String(), "GET one") || !strings.Contains(out.String(), "GET two") {
		t.Fatalf("expected request list, got %q", out.String())
	}
}

func TestRunCmdInteractiveSelectsRequestLineBeforeExecution(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "many.http")
	src := strings.Join([]string{
		"### One",
		"# @name one",
		"GET https://example.com/one",
		"",
		"### Two",
		"# @name two",
		"GET https://example.com/two",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.in = strings.NewReader("2\n")
	cmd.out = &out
	cmd.stdinTTY = true
	cmd.stdoutTTY = true
	cmd.newClient = stubRunClient
	var got runner.Options
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		got = opts
		return stubRunReport(true), nil
	}

	if err := cmd.parse([]string{file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if cmd.line != 6 {
		t.Fatalf("expected selected line 6, got %d", cmd.line)
	}
	if got.Select.Line != 6 {
		t.Fatalf("expected runner select line 6, got %+v", got.Select)
	}
	gotOut := ansi.Strip(out.String())
	if !strings.Contains(gotOut, "Select request [1-2]:") {
		t.Fatalf("expected text prompt fallback, got %q", gotOut)
	}
}

func TestRunCmdMapsRunnerUsageErrorsToExitCodeTwo(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "many.http")
	src := strings.Join([]string{
		"### One",
		"# @name one",
		"GET https://example.com/one",
		"",
		"### Two",
		"# @name two",
		"GET https://example.com/two",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cmd := newRunCmd()
	cmd.newClient = stubRunClient
	if err := cmd.parse([]string{"--request", "one", "--tag", "two", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected usage error")
	}
	if code := cli.ExitCode(err); code != 2 {
		t.Fatalf("expected exit code 2, got %d (err=%v)", code, err)
	}
	if !strings.Contains(err.Error(), "cannot be combined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmdMapsFailedReportToExitCodeOne(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cmd := newRunCmd()
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return stubRunReport(false), nil
	}
	if err := cmd.parse([]string{file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected failure exit")
	}
	if code := cli.ExitCode(err); code != 1 {
		t.Fatalf("expected exit code 1, got %d (err=%v)", code, err)
	}
	if msg := str.Trim(err.Error()); msg != "" {
		t.Fatalf("expected silent failure message, got %q", msg)
	}
}

func TestRunCmdUsesDetailedFailureExitCodeByDefault(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cmd := newRunCmd()
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return stubRunErrorReport(context.DeadlineExceeded), nil
	}
	if err := cmd.parse([]string{file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected timeout exit")
	}
	if code := cli.ExitCode(err); code != 20 {
		t.Fatalf("expected exit code 20, got %d (err=%v)", code, err)
	}
}

func TestRunCmdSummaryExitCodeModeKeepsLegacyFailureCode(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cmd := newRunCmd()
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return stubRunErrorReport(context.DeadlineExceeded), nil
	}
	if err := cmd.parse([]string{"--exit-code-mode", "summary", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected failure exit")
	}
	if code := cli.ExitCode(err); code != 1 {
		t.Fatalf("expected exit code 1, got %d (err=%v)", code, err)
	}
}

func TestRunCmdSummaryExitCodeModeAppliesToRuntimeErrors(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cmd := newRunCmd()
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return nil, context.DeadlineExceeded
	}
	if err := cmd.parse([]string{"--exit-code-mode", "summary", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected runtime error exit")
	}
	if code := cli.ExitCode(err); code != 1 {
		t.Fatalf("expected exit code 1, got %d (err=%v)", code, err)
	}
}

func TestRunCmdRejectsUnsupportedExitCodeMode(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cmd := newRunCmd()
	if err := cmd.parse([]string{"--exit-code-mode", "legacy", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected exit-code-mode error")
	}
	if code := cli.ExitCode(err); code != 2 {
		t.Fatalf("expected exit code 2, got %d (err=%v)", code, err)
	}
	if !strings.Contains(err.Error(), "unsupported --exit-code-mode") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmdFailedOutputEndsWithNewline(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return stubRunReport(false), nil
	}
	if err := cmd.parse([]string{file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected failure exit")
	}
	if !strings.HasSuffix(out.String(), "\n") {
		t.Fatalf("expected output to end with newline, got %q", out.String())
	}
}

func TestRunCmdRejectsUnsupportedFormat(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cmd := newRunCmd()
	if err := cmd.parse([]string{"--format", "yaml", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected format error")
	}
	if code := cli.ExitCode(err); code != 2 {
		t.Fatalf("expected exit code 2, got %d (err=%v)", code, err)
	}
	if !strings.Contains(err.Error(), "unsupported --format") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmdRejectsUnsupportedColor(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cmd := newRunCmd()
	if err := cmd.parse([]string{"--color", "blue", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected color error")
	}
	if code := cli.ExitCode(err); code != 2 {
		t.Fatalf("expected exit code 2, got %d (err=%v)", code, err)
	}
	if !strings.Contains(err.Error(), "unsupported --color") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmdAutoUsesHumanViewForSingleRequest(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return stubRunReport(true), nil
	}
	if err := cmd.parse([]string{file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := cmd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Request: GET https://example.com/one") {
		t.Fatalf("expected human request summary, got %q", got)
	}
	if !strings.Contains(got, "Body:\n{\n  message: \"ok\"\n}") {
		t.Fatalf("expected pretty body view, got %q", got)
	}
	if strings.Contains(got, "Summary: total=") {
		t.Fatalf("expected auto human view, got report text %q", got)
	}
}

func TestRunCmdAutoColorsPrettyOutputForTTY(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.stdoutTTY = true
	cmd.lookupEnv = envLookup(map[string]string{"TERM": "xterm-256color"})
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return stubRunReport(true), nil
	}
	if err := cmd.parse([]string{file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := cmd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected colored pretty output, got %q", got)
	}
	if plain := ansi.Strip(got); !strings.Contains(plain, "Request: GET https://example.com/one") {
		t.Fatalf("expected stripped output to preserve text, got %q", plain)
	}
}

func TestRunCmdAutoColorsWorkflowOutputForTTY(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "workflow.http")
	src := strings.Join([]string{
		"# @workflow sample-order",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.stdoutTTY = true
	cmd.lookupEnv = envLookup(map[string]string{"TERM": "xterm-256color"})
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return &runner.Report{
			Results: []runner.Result{{
				Kind:     runner.ResultKindWorkflow,
				Name:     "sample-order",
				Method:   "WORKFLOW",
				Passed:   true,
				Duration: time.Second,
				Steps: []runner.StepResult{{
					Name:     "Login",
					Passed:   true,
					Duration: 250 * time.Millisecond,
					Response: &httpclient.Response{
						Status:     "200 OK",
						StatusCode: 200,
					},
				}},
			}},
			Total:  1,
			Passed: 1,
		}, nil
	}
	if err := cmd.parse([]string{"--workflow", "sample-order", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := cmd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "\x1b[") {
		t.Fatalf("expected colored workflow output, got %q", got)
	}
	plain := ansi.Strip(got)
	if !strings.Contains(plain, "PASS WORKFLOW sample-order") {
		t.Fatalf("expected stripped workflow output to preserve text, got %q", plain)
	}
	if !strings.Contains(plain, "1. PASS Login") {
		t.Fatalf("expected stripped workflow step output to preserve text, got %q", plain)
	}
}

func TestRunCmdAutoKeepsPipedPrettyPlain(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.stdoutTTY = false
	cmd.lookupEnv = envLookup(map[string]string{"TERM": "xterm-256color"})
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return stubRunReport(true), nil
	}
	if err := cmd.parse([]string{file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := cmd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("expected plain output when stdout is not a tty, got %q", out.String())
	}
}

func TestRunCmdColorNeverDisablesPrettyColor(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.stdoutTTY = true
	cmd.lookupEnv = envLookup(map[string]string{"TERM": "xterm-256color"})
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return stubRunReport(true), nil
	}
	if err := cmd.parse([]string{"--color", "never", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := cmd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	if strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("expected --color never to disable ansi, got %q", out.String())
	}
}

func TestRunCmdAutoFallsBackToTextForMultiResult(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "many.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return &runner.Report{
			Results: []runner.Result{
				stubRunReport(true).Results[0],
				stubRunReport(true).Results[0],
			},
			Total:  2,
			Passed: 2,
		}, nil
	}
	if err := cmd.parse([]string{"--all", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := cmd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	got := out.String()
	if !strings.Contains(got, "Running 2 request(s)") {
		t.Fatalf("expected text report fallback, got %q", got)
	}
}

func TestRunCmdPrettyRequiresSingleRequestResult(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "wf.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cmd := newRunCmd()
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return &runner.Report{
			Results: []runner.Result{{
				Kind:     runner.ResultKindWorkflow,
				Name:     "wf",
				Method:   "WORKFLOW",
				Passed:   true,
				Duration: time.Second,
			}},
			Total:  1,
			Passed: 1,
		}, nil
	}
	if err := cmd.parse([]string{"--format", "pretty", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected format shape error")
	}
	if code := cli.ExitCode(err); code != 2 {
		t.Fatalf("expected exit code 2, got %d (err=%v)", code, err)
	}
	if !strings.Contains(err.Error(), "requires exactly one request result") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmdRawUsesHumanRawView(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return stubRunReport(true), nil
	}
	if err := cmd.parse([]string{"--format", "raw", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := cmd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "Raw Body:\n{\n  \"message\": \"ok\"\n}") {
		t.Fatalf("expected raw body output, got %q", out.String())
	}
}

func TestRunCmdBodyDefaultsToRawBodyOnly(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return stubRunReport(true), nil
	}
	if err := cmd.parse([]string{"--body", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := cmd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	got := out.String()
	if strings.Contains(got, "Name:") || strings.Contains(got, "Body:") {
		t.Fatalf("expected body-only output, got %q", got)
	}
	if got != "{\n  \"message\": \"ok\"\n}\n" {
		t.Fatalf("unexpected body-only output %q", got)
	}
}

func TestRunCmdBodyKeepsExitCodeWithoutFailureBanner(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return stubRunReport(false), nil
	}
	if err := cmd.parse([]string{"--body", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected failure exit")
	}
	if code := cli.ExitCode(err); code != 1 {
		t.Fatalf("expected exit code 1, got %d (err=%v)", code, err)
	}
	if msg := str.Trim(err.Error()); msg != "" {
		t.Fatalf("expected silent failure message, got %q", msg)
	}
	if out.String() != "{\n  \"message\": \"ok\"\n}\n" {
		t.Fatalf("unexpected body output %q", out.String())
	}
}

func TestRunCmdBodyPrettyUsesColorFlag(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.stdoutTTY = false
	cmd.lookupEnv = envLookup(map[string]string{"TERM": "xterm-256color"})
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		return stubRunReport(true), nil
	}
	if err := cmd.parse(
		[]string{"--body", "--format", "pretty", "--color", "always", file},
	); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := cmd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}

	if !strings.Contains(out.String(), "\x1b[") {
		t.Fatalf("expected colored pretty body output, got %q", out.String())
	}
	if got := ansi.Strip(out.String()); got != "{\n  message: \"ok\"\n}\n" {
		t.Fatalf("unexpected stripped body output %q", got)
	}
}

func TestRunCmdBodyRejectsTextFormat(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	cmd := newRunCmd()
	if err := cmd.parse([]string{"--body", "--format", "text", file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	err := cmd.run()
	if err == nil {
		t.Fatalf("expected format/body error")
	}
	if code := cli.ExitCode(err); code != 2 {
		t.Fatalf("expected exit code 2, got %d (err=%v)", code, err)
	}
	if !strings.Contains(err.Error(), "--body can only be combined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunCmdShowsUnresolvedTemplateWarning(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "one.http")
	src := strings.Join([]string{
		"# @name one",
		"GET https://example.com/one",
		"",
	}, "\n")
	if err := os.WriteFile(file, []byte(src), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	var out bytes.Buffer
	cmd := newRunCmd()
	cmd.out = &out
	cmd.newClient = stubRunClient
	cmd.runFn = func(_ context.Context, opts runner.Options) (*runner.Report, error) {
		rep := stubRunReport(true)
		rep.Results[0].SetUnresolvedTemplateVars([]string{"reporting.token", "trace.id"})
		return rep, nil
	}
	if err := cmd.parse([]string{file}); err != nil {
		t.Fatalf("parse: %v", err)
	}
	if err := cmd.run(); err != nil {
		t.Fatalf("run: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Warnings:") {
		t.Fatalf("expected warnings section, got %q", got)
	}
	if !strings.Contains(got, "reporting.token, trace.id") {
		t.Fatalf("expected unresolved template warning, got %q", got)
	}
}

func TestPromptRunRequestChoiceRetriesInvalidInput(t *testing.T) {
	choices := []cli.RunRequestChoice{
		{Line: 3, Label: "GET one"},
		{Line: 7, Label: "GET two"},
	}
	var out bytes.Buffer
	got, err := cli.PromptRunRequestChoice(
		strings.NewReader("0\nnope\n2\n"),
		&out,
		"many.http",
		choices,
		cli.RunRequestPromptOptions{},
	)
	if err != nil {
		t.Fatalf("promptRunRequestChoice: %v", err)
	}
	if got.Line != 7 {
		t.Fatalf("expected second choice, got %+v", got)
	}
	if n := strings.Count(out.String(), "Enter a number between 1 and 2."); n != 2 {
		t.Fatalf("expected two retry notices, got %d in %q", n, out.String())
	}
}

func TestPromptRunRequestChoiceEOF(t *testing.T) {
	_, err := cli.PromptRunRequestChoice(
		strings.NewReader(""),
		&bytes.Buffer{},
		"many.http",
		[]cli.RunRequestChoice{
			{Line: 3, Label: "GET one"},
			{Line: 7, Label: "GET two"},
		},
		cli.RunRequestPromptOptions{},
	)
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF, got %v", err)
	}
}

func stubRunClient(string, cli.ExecFlags) (*httpclient.Client, func() error, error) {
	return httpclient.NewClient(nil), nil, nil
}

func envLookup(vals map[string]string) termcolor.Lookup {
	return func(key string) (string, bool) {
		v, ok := vals[key]
		return v, ok
	}
}

func stubRunReport(pass bool) *runner.Report {
	item := runner.Result{
		Kind:   runner.ResultKindRequest,
		Name:   "one",
		Method: "GET",
		Target: "https://example.com/one",
		Response: &httpclient.Response{
			Status:       "200 OK",
			StatusCode:   200,
			Headers:      http.Header{"Content-Type": {"application/json"}},
			Body:         []byte(`{"message":"ok"}`),
			Duration:     10 * time.Millisecond,
			EffectiveURL: "https://example.com/one",
			ReqMethod:    "GET",
		},
		Passed: pass,
	}
	rep := &runner.Report{
		Results: []runner.Result{item},
		Total:   1,
	}
	if pass {
		rep.Passed = 1
		return rep
	}
	rep.Failed = 1
	return rep
}

func stubRunErrorReport(err error) *runner.Report {
	item := runner.Result{
		Kind:   runner.ResultKindRequest,
		Name:   "one",
		Method: "GET",
		Target: "https://example.com/one",
		Err:    err,
		Passed: false,
	}
	return &runner.Report{
		Results: []runner.Result{item},
		Total:   1,
		Failed:  1,
	}
}
