package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const defaultMockURL = "http://" + mock.DefaultAddr

type mockControlConfig struct {
	url      string
	timeout  time.Duration
	insecure bool
}

func (c *mockControlConfig) bind(fs *flag.FlagSet) {
	cli.StringVarAliases(fs, &c.url, defaultMockURL, "Mock server URL", "url", "u")
	cli.DurationVarAliases(fs, &c.timeout, 5*time.Second, "Control request timeout", "timeout", "t")
	cli.BoolVarAliases(fs, &c.insecure, false, "Skip HTTPS certificate verification", "insecure", "k")
}

func (c mockControlConfig) client() (*mock.Client, error) {
	if c.timeout <= 0 {
		return nil, errors.New("timeout must be positive")
	}
	return mock.NewClient(c.url, mock.ClientOptions{
		Timeout:            c.timeout,
		InsecureSkipVerify: c.insecure,
	})
}

// controlSetup parses the shared control flags and builds the client. done is
// true when help was printed and the command should exit successfully.
func controlSetup(
	cmd string,
	args []string,
	errOut io.Writer,
	extra func(*flag.FlagSet),
) (client *mock.Client, pos []string, done bool, err error) {
	fs := cli.NewSubcommandFlagSet("resterm", cmd, errOut)
	var cfg mockControlConfig
	cfg.bind(fs)
	if extra != nil {
		extra(fs)
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, nil, true, nil
		}
		return nil, nil, false, mockUsageError(err)
	}
	client, err = cfg.client()
	if err != nil {
		return nil, nil, false, mockUsageError(fmt.Errorf("%s: %w", cmd, err))
	}
	return client, fs.Args(), false, nil
}

func controlContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func runMockReset(args []string, out, errOut io.Writer) error {
	client, pos, done, err := controlSetup("mock reset", args, errOut, nil)
	if done || err != nil {
		return err
	}
	if len(pos) > 1 {
		return mockUsageError(errors.New("mock reset accepts at most one sequence name"))
	}
	name := ""
	if len(pos) == 1 {
		name = strings.TrimSpace(pos[0])
		if name == "" || !restfile.ValidMockName(name) {
			return mockUsageError(fmt.Errorf("invalid mock sequence name %q", pos[0]))
		}
	}
	ctx, stop := controlContext()
	defer stop()
	reset, err := client.ResetSequences(ctx, name)
	if err != nil {
		return fmt.Errorf("mock reset: %w", err)
	}
	if name != "" && reset == 0 {
		return fmt.Errorf("mock reset: sequence %q was not found", name)
	}
	_, _ = fmt.Fprintf(out, "Reset %d sequence cursor(s)\n", reset)
	return nil
}

func runMockClear(args []string, out, errOut io.Writer) error {
	client, pos, done, err := controlSetup("mock clear", args, errOut, nil)
	if done || err != nil {
		return err
	}
	if len(pos) != 0 {
		return mockUsageError(errors.New("mock clear does not accept positional arguments"))
	}
	ctx, stop := controlContext()
	defer stop()
	if err := client.Clear(ctx); err != nil {
		return fmt.Errorf("mock clear: %w", err)
	}
	_, _ = fmt.Fprintln(out, "Cleared mock request journal and logs")
	return nil
}

func runMockVerify(args []string, out, errOut io.Writer) error {
	var recursive bool
	client, pos, done, err := controlSetup("mock verify", args, errOut, func(fs *flag.FlagSet) {
		cli.BoolVarAliases(fs, &recursive, false, "Scan workspace recursively", "recursive", "r")
	})
	if done || err != nil {
		return err
	}
	if len(pos) > 1 {
		return mockUsageError(errors.New("mock verify accepts at most one source"))
	}
	path := "."
	if len(pos) == 1 {
		path = pos[0]
	}
	handler, err := mock.Load(path, recursive, nil)
	if err != nil {
		return mockUsageError(fmt.Errorf("mock verify: %w", err))
	}
	expectations := handler.Expectations()
	if len(expectations) == 0 {
		err := fmt.Errorf("mock verify: no # @expect declarations found in %s", path)
		return mockUsageError(err)
	}
	ctx, stop := controlContext()
	defer stop()
	passed := true
	for _, result := range mock.Verify(ctx, client, expectations) {
		status := "PASS"
		if result.Err != nil || !result.Passed {
			status = "FAIL"
			passed = false
		}
		_, _ = fmt.Fprintf(out, "%s %s: %s\n", status, result.Expectation.Label(), result.Detail())
	}
	if !passed {
		return cli.ExitErr{Code: 1}
	}
	return nil
}
