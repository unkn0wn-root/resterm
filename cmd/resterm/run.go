package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/runner"
	"github.com/unkn0wn-root/resterm/internal/runview"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

func handleRunSubcommand(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "run" {
		return false, nil
	}
	if len(args) == 1 && cli.HasFileConflict("run") {
		return true, cli.CommandFileConflict(
			"resterm",
			"run",
			"pass a request file like `resterm run ./requests.http`",
		)
	}
	return true, runRun(args[1:])
}

func runRun(args []string) error {
	if len(args) > 0 {
		op := strings.ToLower(args[0])
		switch op {
		case "-h", "--help", "help":
			cmd := newRunCmd()
			printRunUsage(os.Stdout, cmd.fs)
			return nil
		}
	}

	cmd := newRunCmd()
	if err := cmd.parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return runExit(err, 2)
	}
	return cmd.run()
}

type runExecFn func(context.Context, runner.Options) (*runner.Report, error)
type runClientFn func(string, cli.ExecFlags) (*httpclient.Client, func() error, error)

type runFormat string

const (
	runFmtAuto   runFormat = "auto"
	runFmtText   runFormat = "text"
	runFmtJSON   runFormat = "json"
	runFmtJUnit  runFormat = "junit"
	runFmtPretty runFormat = "pretty"
	runFmtRaw    runFormat = "raw"
)

type runUsageError struct {
	err error
}

func (e runUsageError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e runUsageError) Unwrap() error {
	return e.err
}

func isRunUsageError(err error) bool {
	var target runUsageError
	return errors.As(err, &target)
}

type runCmd struct {
	fs *flag.FlagSet

	exec cli.ExecFlags

	runFn     runExecFn
	newClient runClientFn

	in        io.Reader
	out       io.Writer
	stdinTTY  bool
	stdoutTTY bool
	lookupEnv termcolor.Lookup

	request        string
	workflow       string
	tag            string
	format         string
	color          string
	line           int
	artifactDir    string
	stateDir       string
	all            bool
	body           bool
	headers        bool
	profile        bool
	persistGlobals bool
	persistAuth    bool
	history        bool
}

func newRunCmd() *runCmd {
	cmd := &runCmd{
		exec:      cli.NewExecFlags(),
		runFn:     runner.RunContext,
		newClient: cli.NewExecClient,
		in:        os.Stdin,
		out:       os.Stdout,
		stdinTTY:  term.IsTerminal(int(os.Stdin.Fd())),
		stdoutTTY: term.IsTerminal(int(os.Stdout.Fd())),
		lookupEnv: os.LookupEnv,
		format:    "auto",
		color:     string(termcolor.ModeAuto),
	}
	fs := cli.NewFlagSet("run")
	cmd.fs = fs
	cmd.bind()
	fs.Usage = cmd.usage
	return cmd
}

func (c *runCmd) bind() {
	c.exec.Bind(c.fs)
	cli.StringVar(c.fs, &c.request, "request", "", "Run a named request")
	cli.StringVar(c.fs, &c.request, "r", "", "Run a named request")
	cli.StringVar(c.fs, &c.workflow, "workflow", "", "Run a named workflow")
	cli.StringVar(c.fs, &c.tag, "tag", "", "Run requests matching a tag")
	c.fs.BoolVar(&c.all, "all", false, "Run all requests in the file")
	c.fs.IntVar(&c.line, "line", 0, "Run the request or workflow at a specific line")
	cli.StringVar(
		c.fs,
		&c.format,
		"format",
		c.format,
		"Output format: auto, text, json, junit, pretty, raw",
	)
	cli.StringVar(c.fs, &c.color, "color", c.color, "Color for pretty output: auto, always, never")
	c.fs.BoolVar(&c.body, "body", false, "Print only the response body for a single request result")
	c.fs.BoolVar(&c.headers, "headers", false, "Include request and response headers in output")
	c.fs.BoolVar(&c.profile, "profile", false, "Force profile mode for the selected request")
	cli.StringVar(c.fs, &c.artifactDir, "artifact-dir", "", "Directory to write run artifacts")
	cli.StringVar(c.fs, &c.stateDir, "state-dir", "", "Directory for persisted run state")
	c.fs.BoolVar(
		&c.persistGlobals,
		"persist-globals",
		false,
		"Persist captured globals between invocations",
	)
	c.fs.BoolVar(
		&c.persistAuth,
		"persist-auth",
		false,
		"Persist cached auth state between invocations",
	)
	c.fs.BoolVar(&c.history, "history", false, "Persist run history to the state directory")
}

func (c *runCmd) parse(args []string) error {
	return c.fs.Parse(args)
}

func (c *runCmd) run() error {
	args := c.fs.Args()
	switch len(args) {
	case 0:
		return runExit(errors.New("request file path is required"), 2)
	case 1:
	default:
		return runExit(fmt.Errorf("unexpected args: %s", strings.Join(args[1:], " ")), 2)
	}

	src, err := c.loadSource(args[0])
	if err != nil {
		return runExit(err, 2)
	}
	doc, err := cli.ParseRunDoc(src)
	if err != nil {
		return runExit(err, 2)
	}
	if err := c.resolveDefaultRequest(doc, src); err != nil {
		return err
	}
	if err := c.validateFormat(); err != nil {
		return runExit(err, 2)
	}

	cfg, err := c.exec.Resolve(src.Path)
	if err != nil {
		return runExit(err, 2)
	}

	client, shutdown, err := c.client()
	if err != nil && c.exec.TelemetryConfig(version).Enabled() {
		log.Printf("telemetry init error: %v", err)
	}
	if shutdown != nil {
		defer func() {
			if shutErr := shutdown(); shutErr != nil {
				log.Printf("telemetry shutdown: %v", shutErr)
			}
		}()
	}

	rep, err := c.execRun(context.Background(), src, cfg, client)
	if err != nil {
		if runner.IsUsageError(err) {
			return runExit(err, 2)
		}
		return runExit(err, 1)
	}
	if err := c.writeReport(rep); err != nil {
		if isRunUsageError(err) {
			return runExit(err, 2)
		}
		return runExit(fmt.Errorf("write report: %w", err), 1)
	}
	if rep.Success() {
		return nil
	}
	return cli.ExitErr{Code: 1}
}

func (c *runCmd) hasRequestSelector() bool {
	if c == nil {
		return false
	}
	switch {
	case c.all:
		return true
	case c.line > 0:
		return true
	case c.request != "":
		return true
	case c.tag != "":
		return true
	default:
		return false
	}
}

func (c *runCmd) loadSource(arg string) (cli.RunSource, error) {
	path := str.Trim(arg)
	switch path {
	case "":
		return cli.RunSource{}, errors.New("run: request file path is required")
	case "-":
		cfg, err := c.exec.Resolve(cli.StdinRunPath(c.exec.Workspace))
		if err != nil {
			return cli.RunSource{}, err
		}
		data, err := io.ReadAll(c.stdin())
		if err != nil {
			return cli.RunSource{}, fmt.Errorf("run: read stdin: %w", err)
		}
		return cli.RunSource{Path: cfg.FilePath, Data: data, Stdin: true}, nil
	default:
		cfg, err := c.exec.Resolve(path)
		if err != nil {
			return cli.RunSource{}, err
		}
		data, err := os.ReadFile(cfg.FilePath)
		if err != nil {
			return cli.RunSource{}, fmt.Errorf("read file: %w", err)
		}
		return cli.RunSource{Path: cfg.FilePath, Data: data}, nil
	}
}

func (c *runCmd) resolveDefaultRequest(doc *restfile.Document, src cli.RunSource) error {
	if c == nil || c.hasRequestSelector() || c.hasWorkflowSelector() {
		return nil
	}
	reqs := cli.BuildRunRequestChoices(doc)
	switch len(reqs) {
	case 0:
		return nil
	case 1:
		c.line = reqs[0].Line
		return nil
	}

	if src.Stdin || !c.stdinTTY || !c.stdoutTTY {
		if err := cli.WriteRunRequestChoices(c.stdout(), src.Path, reqs); err != nil {
			return fmt.Errorf("run: write request list: %w", err)
		}
		return cli.ExitErr{
			Err:  errors.New("run: multiple requests found; use --request, --tag, --all, or --line"),
			Code: 2,
		}
	}

	ch, err := cli.PromptRunRequestChoice(c.stdin(), c.stdout(), src.Path, reqs)
	if err != nil {
		return cli.ExitErr{Err: fmt.Errorf("run: %w", err), Code: 2}
	}
	c.line = ch.Line
	return nil
}

func (c *runCmd) client() (*httpclient.Client, func() error, error) {
	if c != nil && c.newClient != nil {
		return c.newClient(version, c.exec)
	}
	return cli.NewExecClient(version, c.exec)
}

func (c *runCmd) execRun(
	ctx context.Context,
	src cli.RunSource,
	cfg cli.ExecConfig,
	client *httpclient.Client,
) (*runner.Report, error) {
	runFn := runner.RunContext
	if c != nil && c.runFn != nil {
		runFn = c.runFn
	}
	return runFn(ctx, c.runOptions(src, cfg, client))
}

func (c *runCmd) runOptions(
	src cli.RunSource,
	cfg cli.ExecConfig,
	client *httpclient.Client,
) runner.Options {
	return runner.Options{
		Version:         version,
		FilePath:        src.Path,
		FileContent:     src.Data,
		WorkspaceRoot:   cfg.Workspace,
		Recursive:       cfg.Recursive,
		ArtifactDir:     c.artifactDir,
		StateDir:        c.stateDir,
		PersistGlobals:  c.persistGlobals,
		PersistAuth:     c.persistAuth,
		History:         c.history,
		EnvSet:          cfg.EnvSet,
		EnvName:         cfg.EnvName,
		EnvironmentFile: cfg.EnvFile,
		CompareTargets:  cfg.CompareTargets,
		CompareBase:     cfg.CompareBase,
		Profile:         c.profile,
		HTTPOptions:     cfg.HTTPOpts,
		GRPCOptions:     cfg.GRPCOpts,
		Client:          client,
		Select:          c.selector(),
	}
}

func (c *runCmd) selector() runner.Select {
	if c == nil {
		return runner.Select{}
	}
	return runner.Select{
		Request:  c.request,
		Workflow: c.workflow,
		Tag:      c.tag,
		All:      c.all,
		Line:     c.line,
	}
}

func (c *runCmd) writeReport(rep *runner.Report) error {
	if rep == nil {
		return errors.New("empty report")
	}
	if c.body {
		return c.writeBody(rep)
	}
	color := c.prettyColor()
	switch c.reportFormat() {
	case runFmtAuto:
		if runview.CanRenderRequest(rep) {
			return c.writeOutput(func(w io.Writer) error {
				return runview.Write(w, rep, runview.Options{
					Mode:    runview.ModePretty,
					Headers: c.headers,
					Color:   color,
				})
			})
		}
		return c.writeOutput(func(w io.Writer) error {
			if color.Enabled {
				return rep.WriteTextStyled(w, color)
			}
			return rep.WriteText(w)
		})
	case runFmtText:
		return c.writeOutput(func(w io.Writer) error {
			return rep.WriteText(w)
		})
	case runFmtJSON:
		return c.writeOutput(func(w io.Writer) error {
			return rep.WriteJSON(w)
		})
	case runFmtJUnit:
		return c.writeOutput(func(w io.Writer) error {
			return rep.WriteJUnit(w)
		})
	case runFmtPretty:
		if !runview.CanRenderRequest(rep) {
			return runUsageError{
				err: errors.New("--format pretty requires exactly one request result"),
			}
		}
		return c.writeOutput(func(w io.Writer) error {
			return runview.Write(w, rep, runview.Options{
				Mode:    runview.ModePretty,
				Headers: c.headers,
				Color:   color,
			})
		})
	case runFmtRaw:
		if !runview.CanRenderRequest(rep) {
			return runUsageError{
				err: errors.New("--format raw requires exactly one request result"),
			}
		}
		return c.writeOutput(func(w io.Writer) error {
			return runview.Write(w, rep, runview.Options{
				Mode:    runview.ModeRaw,
				Headers: c.headers,
			})
		})
	default:
		return runUsageError{err: fmt.Errorf("unsupported --format %q", c.format)}
	}
}

func (c *runCmd) stdin() io.Reader {
	if c != nil && c.in != nil {
		return c.in
	}
	return os.Stdin
}

func (c *runCmd) stdout() io.Writer {
	if c != nil && c.out != nil {
		return c.out
	}
	return os.Stdout
}

func (c *runCmd) hasWorkflowSelector() bool {
	return c != nil && c.workflow != ""
}

func (c *runCmd) validateFormat() error {
	if _, err := termcolor.ParseMode(c.color); err != nil {
		return fmt.Errorf("unsupported --color %q", c.color)
	}
	format := c.reportFormat()
	if format == "" {
		return fmt.Errorf("unsupported --format %q", c.format)
	}
	if c.body {
		switch format {
		case runFmtAuto, runFmtPretty, runFmtRaw:
		default:
			return errors.New("--body can only be combined with --format auto, pretty, or raw")
		}
	}
	return nil
}

func (c *runCmd) reportFormat() runFormat {
	if c == nil {
		return ""
	}
	switch strings.ToLower(c.format) {
	case "", string(runFmtAuto):
		return runFmtAuto
	case string(runFmtText):
		return runFmtText
	case string(runFmtJSON):
		return runFmtJSON
	case string(runFmtJUnit):
		return runFmtJUnit
	case string(runFmtPretty):
		return runFmtPretty
	case string(runFmtRaw):
		return runFmtRaw
	default:
		return ""
	}
}

func (c *runCmd) writeBody(rep *runner.Report) error {
	if !runview.CanRenderRequest(rep) {
		return runUsageError{
			err: errors.New("--body requires exactly one request result"),
		}
	}
	mode := runview.ModeRaw
	if c.reportFormat() == runFmtPretty {
		mode = runview.ModePretty
	}
	color := termcolor.Config{}
	if mode == runview.ModePretty {
		color = c.prettyColor()
	}
	return c.writeOutput(func(w io.Writer) error {
		return runview.WriteBody(w, rep, runview.BodyOptions{Mode: mode, Color: color})
	})
}

func (c *runCmd) writeOutput(fn func(io.Writer) error) error {
	if fn == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := fn(&buf); err != nil {
		return err
	}
	out := buf.Bytes()
	if len(out) == 0 || out[len(out)-1] != '\n' {
		out = append(out, '\n')
	}
	_, err := c.stdout().Write(out)
	return err
}

func (c *runCmd) prettyColor() termcolor.Config {
	if c == nil {
		return termcolor.Config{}
	}
	mode, err := termcolor.ParseMode(c.color)
	if err != nil {
		return termcolor.Config{}
	}
	return termcolor.Resolve(termcolor.Input{
		Mode:   mode,
		TTY:    c.stdoutTTY,
		Lookup: c.lookupEnv,
	})
}

func (c *runCmd) usage() {
	printRunUsage(os.Stderr, c.fs)
}

func runExit(err error, code int) error {
	if err == nil {
		return nil
	}
	if strings.HasPrefix(err.Error(), "run: ") {
		return cli.ExitErr{Err: err, Code: code}
	}
	return cli.ExitErr{Err: fmt.Errorf("run: %w", err), Code: code}
}

func printRunUsage(w io.Writer, fs *flag.FlagSet) {
	if w == nil || fs == nil {
		return
	}
	if _, err := fmt.Fprintf(w, "Usage: resterm run [flags] <file|->\n\n"); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "Flags:"); err != nil {
		return
	}
	out := fs.Output()
	fs.SetOutput(w)
	defer fs.SetOutput(out)
	fs.PrintDefaults()
}
