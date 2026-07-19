package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/mock"
	dvalue "github.com/unkn0wn-root/resterm/internal/parser/directive/value"
)

func handleMockSubcommand(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "mock" {
		return false, nil
	}
	if len(args) == 1 && cli.HasFileConflict("mock") {
		return true, cli.CommandFileConflict(
			"resterm",
			"mock",
			"pass a source like `resterm mock ./api.http` to run the mock server",
		)
	}
	return true, runMock(args[1:])
}

type mockConfig struct {
	path             string
	addr             string
	cors             string
	tlsCert          string
	tlsKey           string
	recursive        bool
	watch            bool
	quiet            bool
	sequenceKeyLimit int
	journalEntries   int
	journalBytes     string
	journalBodyLimit string
}

func runMock(args []string) error {
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "reset":
			return runMockReset(args[1:], os.Stdout, os.Stderr)
		case "clear":
			return runMockClear(args[1:], os.Stdout, os.Stderr)
		case "verify":
			return runMockVerify(args[1:], os.Stdout, os.Stderr)
		}
	}
	return runMockServe(args)
}

func defaultMockConfig() mockConfig {
	return mockConfig{
		addr:             mock.DefaultAddr,
		cors:             "auto",
		watch:            true,
		sequenceKeyLimit: mock.DefaultSequenceKeyLimit,
		journalEntries:   mock.DefaultJournalEntries,
		journalBytes:     "16MiB",
		journalBodyLimit: "64KiB",
	}
}

func runMockServe(args []string) error {
	fs := cli.NewFlagSet("mock")
	cfg := defaultMockConfig()
	cli.StringVarAliases(fs, &cfg.addr, cfg.addr, "Listen address", "addr", "a")
	cli.StringVarAliases(fs, &cfg.cors, cfg.cors, "CORS policy: auto, off, *, or comma-separated origins", "cors")
	cli.StringVarAliases(
		fs,
		&cfg.tlsCert,
		"",
		"Serve HTTPS using this PEM certificate (requires --tls-key)",
		"tls-cert",
	)
	cli.StringVarAliases(fs, &cfg.tlsKey, "", "PEM private key for --tls-cert", "tls-key")
	cli.BoolVarAliases(fs, &cfg.recursive, false, "Scan workspace recursively", "recursive", "r")
	cli.BoolVarAliases(fs, &cfg.watch, true, "Reload changed sources and fixtures", "watch", "w")
	cli.BoolVarAliases(fs, &cfg.quiet, false, "Suppress per-request access summaries", "quiet", "q")
	cli.IntVarAliases(
		fs,
		&cfg.sequenceKeyLimit,
		cfg.sequenceKeyLimit,
		"Maximum distinct keys retained by each sequence",
		"sequence-key-limit",
	)
	cli.IntVarAliases(
		fs,
		&cfg.journalEntries,
		cfg.journalEntries,
		"Maximum requests retained for verification",
		"journal-entries",
	)
	cli.StringVarAliases(
		fs,
		&cfg.journalBytes,
		cfg.journalBytes,
		"Maximum memory retained by the request journal",
		"journal-bytes",
	)
	cli.StringVarAliases(
		fs,
		&cfg.journalBodyLimit,
		cfg.journalBodyLimit,
		"Maximum body bytes retained per request",
		"journal-body-limit",
	)
	fs.Usage = func() { printMockUsage(os.Stderr, fs) }

	if len(args) == 1 {
		switch strings.ToLower(args[0]) {
		case "help", "-h", "--help":
			printMockUsage(os.Stdout, fs)
			return nil
		}
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return cli.ExitErr{Err: err, Code: 2}
	}
	switch len(fs.Args()) {
	case 0:
		cfg.path = "."
	case 1:
		cfg.path = fs.Arg(0)
	default:
		return cli.ExitErr{
			Err:  fmt.Errorf("mock: unexpected args: %s", strings.Join(fs.Args()[1:], " ")),
			Code: 2,
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return serveMocks(ctx, cfg, os.Stdout, os.Stderr)
}

func serveMocks(ctx context.Context, cfg mockConfig, out, errOut io.Writer) error {
	if (cfg.tlsCert == "") != (cfg.tlsKey == "") {
		return cli.ExitErr{Err: errors.New("mock: --tls-cert and --tls-key must be set together"), Code: 2}
	}
	cors, warning, err := mock.ResolveCORS(cfg.cors, cfg.addr)
	if err != nil {
		return cli.ExitErr{Err: fmt.Errorf("mock: %w", err), Code: 2}
	}
	if warning != "" {
		_, _ = fmt.Fprintln(errOut, "warning:", warning)
	}
	if !mock.IsLoopbackAddr(cfg.addr) {
		_, _ = fmt.Fprintf(errOut, "warning: mock server is exposed on %s\n", cfg.addr)
	}
	journalBytes, err := dvalue.ParseByteSize(cfg.journalBytes)
	if err != nil || journalBytes <= 0 {
		return cli.ExitErr{
			Err:  fmt.Errorf("mock: invalid --journal-bytes %q", cfg.journalBytes),
			Code: 2,
		}
	}
	journalBodyLimit, err := dvalue.ParseByteSize(cfg.journalBodyLimit)
	if err != nil || journalBodyLimit <= 0 {
		return cli.ExitErr{
			Err:  fmt.Errorf("mock: invalid --journal-body-limit %q", cfg.journalBodyLimit),
			Code: 2,
		}
	}
	if journalBodyLimit > journalBytes {
		return cli.ExitErr{
			Err:  errors.New("mock: --journal-body-limit must not exceed --journal-bytes"),
			Code: 2,
		}
	}
	if cfg.sequenceKeyLimit <= 0 || cfg.journalEntries <= 0 {
		return cli.ExitErr{
			Err:  errors.New("mock: sequence key and journal entry limits must be positive"),
			Code: 2,
		}
	}

	reloader := mock.NewReloader(cfg.path, cfg.recursive)
	handler, err := reloader.Reload("", nil)
	if err != nil {
		return fmt.Errorf("mock: %w", err)
	}
	if handler.Routes() == 0 {
		return fmt.Errorf("mock: no # @mock scenarios found in %s", cfg.path)
	}
	logger := log.New(errOut, "", 0)
	server, err := mock.Start(cfg.addr, handler, mock.Options{
		CORS:             cors,
		EnableControl:    true,
		TLSCert:          cfg.tlsCert,
		TLSKey:           cfg.tlsKey,
		SequenceKeyLimit: cfg.sequenceKeyLimit,
		JournalEntries:   cfg.journalEntries,
		JournalBytes:     journalBytes,
		JournalBodyLimit: journalBodyLimit,
		OnEvent: func(event mock.Event) {
			if !cfg.quiet && !event.Reload {
				printMockEvent(logger, event)
			}
		},
	})
	if err != nil {
		return fmt.Errorf("mock: %w", err)
	}
	scheme := "http"
	if cfg.tlsCert != "" {
		scheme = "https"
	}
	_, _ = fmt.Fprintf(
		out,
		"Mock server listening on %s://%s (%d routes, %d scenarios)\n",
		scheme,
		server.Addr(),
		handler.Routes(),
		handler.Scenarios(),
	)

	var ticks <-chan time.Time
	if cfg.watch {
		ticker := time.NewTicker(time.Second)
		ticks = ticker.C
		defer ticker.Stop()
	}
	watcher := &mockWatcher{reloader: reloader, server: server, out: out, errOut: errOut}

	for {
		select {
		case <-ctx.Done():
			return closeMockServer(server)
		case <-server.Done():
			if err := server.Err(); err != nil {
				return fmt.Errorf("mock server: %w", err)
			}
			return nil
		case <-ticks:
			watcher.tick()
		}
	}
}

type mockWatcher struct {
	reloader *mock.Reloader
	server   *mock.Server
	out      io.Writer
	errOut   io.Writer
	lastErr  string
}

func (w *mockWatcher) tick() {
	h, err := w.reloader.Reload("", nil)
	if err != nil {
		if msg := err.Error(); msg != w.lastErr {
			w.lastErr = msg
			w.server.RecordReload(err)
			_, _ = fmt.Fprintln(w.errOut, "mock reload failed; keeping last valid routes:", err)
		}
		return
	}
	w.lastErr = ""
	if h == nil {
		return
	}
	w.server.Reload(h)
	_, _ = fmt.Fprintf(w.out, "Reloaded %d routes (%d scenarios)\n", h.Routes(), h.Scenarios())
}

func printMockEvent(logger *log.Logger, event mock.Event) {
	scenario := ""
	if label := event.ScenarioLabel(); label != "" {
		scenario = " [" + label + "]"
	}
	logger.Printf(
		"%s %s -> %d%s (%s)",
		event.Method,
		event.Target,
		event.Status,
		scenario,
		event.Duration.Round(time.Microsecond),
	)
}

func closeMockServer(server *mock.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Close(ctx); err != nil {
		return fmt.Errorf("mock shutdown: %w", err)
	}
	return nil
}

func printMockUsage(w io.Writer, fs *flag.FlagSet) {
	_, _ = fmt.Fprintln(w, "Usage: resterm mock [flags] [file|dir]")
	_, _ = fmt.Fprintln(w, "       resterm mock reset [flags] [sequence]")
	_, _ = fmt.Fprintln(w, "       resterm mock clear [flags]")
	_, _ = fmt.Fprintln(w, "       resterm mock verify [flags] [file|dir]")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Serve # @mock response blocks from a request file or workspace.")
	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintln(w, "Flags:")
	cli.PrintFlagDefaults(w, fs)
}
