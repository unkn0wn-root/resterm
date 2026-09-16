package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/mock"
	"github.com/unkn0wn-root/resterm/internal/recorder"
)

func handleRecordSubcommand(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "record" {
		return false, nil
	}
	if len(args) == 1 && cli.HasFileConflict("record") {
		return true, cli.CommandFileConflict("resterm", "record", "pass --upstream and --out to record traffic")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return true, runRecord(ctx, args[1:], os.Stdout, os.Stderr)
}

type recordArgs struct {
	cfg  recorder.Config
	path string
	mode recorder.Mode
}

func runRecord(ctx context.Context, args []string, out, errOut io.Writer) error {
	a, err := parseRecordArgs(args, errOut)
	if err != nil || a == nil {
		return err
	}
	return serveRecording(ctx, *a, out, errOut)
}

// parseRecordArgs returns nil, nil after printing help.
func parseRecordArgs(args []string, errOut io.Writer) (*recordArgs, error) {
	fs := cli.NewFlagSet("record")
	flags := cli.NewRecordFlags()
	flags.Bind(fs)

	var path, mode string
	fs.StringVarAliases(&path, "", "New .http/.rest output file (required)", "out", "o")
	fs.StringVarAliases(&mode, string(recorder.Requests), "Output: requests, mocks, or both", "mode")
	fs.Usage = func() { cli.PrintFlagSetUsage(errOut, "resterm record", fs) }

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, cli.ErrHelp) {
			fs.Usage()
			return nil, nil
		}
		return nil, cli.ExitErr{Err: err, Code: 2}
	}

	cfg, err := flags.Resolve()
	if err != nil {
		return nil, cli.ExitErr{Err: err, Code: 2}
	}
	parsed, modeErr := recorder.ParseMode(mode)
	if path == "" || modeErr != nil || len(fs.Args()) != 0 {
		return nil, cli.ExitErr{
			Err: errors.New(
				"record requires --out, accepts --mode requests|mocks|both, and takes no positional arguments",
			),
			Code: 2,
		}
	}
	return &recordArgs{cfg: cfg, path: path, mode: parsed}, nil
}

func serveRecording(ctx context.Context, a recordArgs, out, errOut io.Writer) (result error) {
	output, err := recorder.CreateOutput(a.path)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, output.Close()) }()

	s, err := recorder.Start(ctx, a.cfg)
	if err != nil {
		return err
	}
	defer func() {
		closeCtx, cancel := context.WithTimeout(context.Background(), recorder.ShutdownTimeout)
		defer cancel()
		result = errors.Join(result, s.Close(closeCtx))
	}()

	if !mock.IsLoopbackAddr(s.Addr()) {
		_, _ = fmt.Fprintf(errOut, "Recorder is exposed on %s\n", s.Addr())
	}
	_, err = fmt.Fprintf(out, "Recorder listening on http://%s -> %s. Saving %s to %s\n",
		s.Addr(), s.Upstream(), a.mode, a.path)
	if err != nil {
		return err
	}

	sink := recordSink{session: s, output: output, mode: a.mode, path: a.path, errOut: errOut}
	for {
		select {
		case <-s.Updates():
		case <-s.Done():
			if err := sink.drain(); err != nil {
				return err
			}
			return sink.report(out)
		}
		if err := sink.drain(); err != nil {
			return err
		}
	}
}

type recordSink struct {
	session *recorder.Session
	output  *recorder.Output
	mode    recorder.Mode
	path    string
	errOut  io.Writer

	last     uint64
	excluded int
	limit    recorder.Limit
}

func (s *recordSink) drain() error {
	if err := s.flush(); err != nil {
		return err
	}
	return s.notice()
}

func (s *recordSink) notice() error {
	limit := s.session.Stats().Limit
	if limit == "" || limit == s.limit {
		return nil
	}
	s.limit = limit
	_, err := fmt.Fprintf(s.errOut, "Recorder %s. Forwarding continues without recording\n", limit)
	return err
}

func (s *recordSink) flush() error {
	entries := s.session.Snapshot(s.last)
	if len(entries) == 0 {
		return nil
	}

	p, err := recorder.Build(entries, recorder.ExportOptions{
		Mode:     s.mode,
		Path:     s.path,
		Upstream: s.session.Upstream(),
		Existing: s.output.Existing(),
	})
	if err != nil {
		return err
	}
	// Finish writing the block even if recording is canceled.
	if err := s.output.Append(context.Background(), p); err != nil {
		return err
	}

	for _, x := range p.Excluded {
		_, err := fmt.Fprintf(s.errOut, "Record %d: %s export excluded: %s\n", x.ID, x.Mode, x.Reason)
		if err != nil {
			return err
		}
	}
	for _, shadow := range p.Shadowed {
		_, err := fmt.Fprintf(s.errOut,
			"Record %d: mock matches an earlier scenario. Use X-Resterm-Mock to select this response\n",
			shadow.ID)
		if err != nil {
			return err
		}
	}
	s.excluded += len(p.Excluded)
	s.last = entries[len(entries)-1].ID
	return nil
}

func (s *recordSink) report(out io.Writer) error {
	stats := s.session.Stats()
	filtered := ""
	if stats.Filtered > 0 {
		filtered = fmt.Sprintf(" Filtered %d.", stats.Filtered)
	}
	_, writeErr := fmt.Fprintf(out,
		"Recorded %d exchanges.%s Skipped %d captures and %d exports. Output: %s\n",
		stats.Entries, filtered, stats.Excluded, s.excluded, s.path)
	if err := errors.Join(s.session.Err(), writeErr); err != nil {
		return err
	}
	if s.excluded > 0 || stats.Excluded > 0 {
		return errors.New("recording is incomplete. Check the recorder warnings and summary")
	}
	return nil
}
