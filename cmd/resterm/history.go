package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/config"
	histdb "github.com/unkn0wn-root/resterm/internal/history/sqlite"
)

func handleHistorySubcommand(args []string) (bool, error) {
	if len(args) == 0 || args[0] != "history" {
		return false, nil
	}
	if len(args) == 1 && historyTargetExists() {
		return true, fmt.Errorf(
			"history: found file named \"history\" in the current directory; use `resterm -- history` or `resterm ./history` to open it, or pass a subcommand like `resterm history export --out ./history.json`",
		)
	}
	return true, runHistory(args[1:])
}

func historyTargetExists() bool {
	info, err := os.Stat("history")
	if err != nil {
		return false
	}
	return !info.IsDir()
}

func runHistory(args []string) error {
	if len(args) == 0 {
		return errors.New(historyUsageText())
	}
	op := strings.TrimSpace(strings.ToLower(args[0]))
	switch op {
	case "-h", "--help", "help":
		if err := writeln(os.Stdout, historyUsageText()); err != nil {
			return fmt.Errorf("history: write output: %w", err)
		}
		return nil
	case "export":
		return runHistoryExport(args[1:])
	case "import":
		return runHistoryImport(args[1:])
	case "backup":
		return runHistoryBackup(args[1:])
	case "stats":
		return runHistoryStats(args[1:])
	case "compact", "vacuum":
		return runHistoryCompact(args[1:])
	case "check":
		return runHistoryCheck(args[1:])
	default:
		return fmt.Errorf("history: unknown subcommand %q\n\n%s", op, historyUsageText())
	}
}

func runHistoryExport(args []string) error {
	fs := newHistoryFlagSet("history export")
	var out string
	fs.StringVar(&out, "out", "", "Output JSON file path")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("history export: %w", err)
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("history export: unexpected args: %s", strings.Join(fs.Args(), " "))
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return errors.New("history export: --out is required")
	}

	s, err := openHistoryStore(true)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	n, err := s.ExportJSON(out)
	if err != nil {
		return err
	}
	if err := writef(os.Stdout, "Exported %d history entries to %s\n", n, out); err != nil {
		return fmt.Errorf("history export: write output: %w", err)
	}
	return nil
}

func runHistoryImport(args []string) error {
	fs := newHistoryFlagSet("history import")
	var in string
	fs.StringVar(&in, "in", "", "Input JSON file path")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("history import: %w", err)
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("history import: unexpected args: %s", strings.Join(fs.Args(), " "))
	}
	in = strings.TrimSpace(in)
	if in == "" {
		return errors.New("history import: --in is required")
	}

	s, err := openHistoryStore(false)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	n, err := s.ImportJSON(in)
	if err != nil {
		return err
	}
	if err := writef(os.Stdout, "Imported %d history entries from %s\n", n, in); err != nil {
		return fmt.Errorf("history import: write output: %w", err)
	}
	return nil
}

func runHistoryBackup(args []string) error {
	fs := newHistoryFlagSet("history backup")
	var out string
	fs.StringVar(&out, "out", "", "Output SQLite backup file path")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("history backup: %w", err)
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("history backup: unexpected args: %s", strings.Join(fs.Args(), " "))
	}
	out = strings.TrimSpace(out)
	if out == "" {
		return errors.New("history backup: --out is required")
	}

	s, err := openHistoryStore(true)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	if err := s.Backup(out); err != nil {
		return err
	}
	if err := writef(os.Stdout, "Created history backup at %s\n", out); err != nil {
		return fmt.Errorf("history backup: write output: %w", err)
	}
	return nil
}

func runHistoryStats(args []string) error {
	fs := newHistoryFlagSet("history stats")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("history stats: %w", err)
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("history stats: unexpected args: %s", strings.Join(fs.Args(), " "))
	}

	s, err := openHistoryStore(true)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	st, err := s.Stats()
	if err != nil {
		return err
	}
	if err := writef(os.Stdout, "History DB: %s\n", st.Path); err != nil {
		return fmt.Errorf("history stats: write output: %w", err)
	}
	if err := writef(os.Stdout, "Schema: %d\n", st.Schema); err != nil {
		return fmt.Errorf("history stats: write output: %w", err)
	}
	if err := writef(os.Stdout, "Rows: %d\n", st.Rows); err != nil {
		return fmt.Errorf("history stats: write output: %w", err)
	}
	if !st.Oldest.IsZero() {
		if err := writef(
			os.Stdout,
			"Oldest: %s\n",
			st.Oldest.UTC().Format(time.RFC3339),
		); err != nil {
			return fmt.Errorf("history stats: write output: %w", err)
		}
	}
	if !st.Newest.IsZero() {
		if err := writef(
			os.Stdout,
			"Newest: %s\n",
			st.Newest.UTC().Format(time.RFC3339),
		); err != nil {
			return fmt.Errorf("history stats: write output: %w", err)
		}
	}
	if err := writef(os.Stdout, "DB Size: %s\n", byteLabel(st.DBBytes)); err != nil {
		return fmt.Errorf("history stats: write output: %w", err)
	}
	if err := writef(os.Stdout, "WAL Size: %s\n", byteLabel(st.WALBytes)); err != nil {
		return fmt.Errorf("history stats: write output: %w", err)
	}
	if err := writef(os.Stdout, "SHM Size: %s\n", byteLabel(st.SHMBytes)); err != nil {
		return fmt.Errorf("history stats: write output: %w", err)
	}
	return nil
}

func runHistoryCompact(args []string) error {
	fs := newHistoryFlagSet("history compact")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("history compact: %w", err)
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("history compact: unexpected args: %s", strings.Join(fs.Args(), " "))
	}

	s, err := openHistoryStore(true)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	b, err := s.Stats()
	if err != nil {
		return err
	}

	if err = s.Compact(); err != nil {
		return err
	}

	a, err := s.Stats()
	if err != nil {
		return err
	}

	if err := writef(
		os.Stdout,
		"Compacted history db (%s -> %s)\n",
		byteLabel(b.DBBytes+b.WALBytes+b.SHMBytes),
		byteLabel(a.DBBytes+a.WALBytes+a.SHMBytes),
	); err != nil {
		return fmt.Errorf("history compact: write output: %w", err)
	}
	return nil
}

func runHistoryCheck(args []string) error {
	fs := newHistoryFlagSet("history check")
	var full bool
	fs.BoolVar(&full, "full", false, "Use full integrity check")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return fmt.Errorf("history check: %w", err)
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("history check: unexpected args: %s", strings.Join(fs.Args(), " "))
	}

	s, err := openHistoryStore(true)
	if err != nil {
		return err
	}
	defer func() { _ = s.Close() }()

	if err := s.Check(full); err != nil {
		return err
	}
	mode := "quick"
	if full {
		mode = "full"
	}
	if err := writef(os.Stdout, "History integrity check (%s): ok\n", mode); err != nil {
		return fmt.Errorf("history check: write output: %w", err)
	}
	return nil
}

func openHistoryStore(migrate bool) (*histdb.Store, error) {
	// This centralizes all history startup behavior used by CLI maintenance commands.
	// It loads the database, prints recovery warnings, and optionally runs legacy import.
	// On migration failure the store is closed before returning.
	s := histdb.New(config.HistoryPath())
	if err := s.Load(); err != nil {
		return nil, fmt.Errorf("history: load store: %w", err)
	}
	// Recovery is non-fatal, but the warning still goes to stderr so
	// operators know the original file was quarantined.
	if rec := s.RecoveryInfo(); rec != nil {
		if err := printHistoryRecoveryWarning(os.Stderr, rec); err != nil {
			return nil, fmt.Errorf("history: write recovery warning: %w", err)
		}
	}
	// Legacy JSON import is optional because direct maintenance commands
	// may need to inspect or repair SQLite without backfilling first.
	if migrate {
		if _, err := s.MigrateJSON(config.LegacyHistoryPath()); err != nil {
			_ = s.Close()
			return nil, fmt.Errorf("history: migrate legacy json: %w", err)
		}
	}
	return s, nil
}

func newHistoryFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	// Errors are formatted manually so each subcommand can keep a clear
	// and consistent prefix in user-facing output.
	fs.SetOutput(io.Discard)
	// Help output still goes to stderr so `-h` behaves like a normal CLI.
	fs.Usage = func() {
		printHistoryFlagSetUsage(os.Stderr, fs)
	}
	return fs
}

func printHistoryFlagSetUsage(w io.Writer, fs *flag.FlagSet) {
	if _, err := fmt.Fprintf(w, "Usage: resterm %s [flags]\n", fs.Name()); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, ""); err != nil {
		return
	}
	if _, err := fmt.Fprintln(w, "Flags:"); err != nil {
		return
	}
	out := fs.Output()
	fs.SetOutput(w)
	fs.PrintDefaults()
	fs.SetOutput(out)
}

func historyUsageText() string {
	return strings.TrimSpace(`
Usage: resterm history <export|import|backup|stats|check|compact> [flags]

Subcommands:
  export --out <path>   Export history to JSON
  import --in <path>    Import history from JSON
  backup --out <path>   Create a SQLite-consistent backup
  stats                 Show history DB stats
  check [--full]        Run SQLite integrity check
  compact               Run VACUUM and checkpoint
`)
}

func printHistoryRecoveryWarning(w io.Writer, rec *histdb.RecoverInfo) error {
	if rec == nil {
		return nil
	}
	cause := strings.TrimSpace(rec.Cause)
	if cause == "" {
		cause = "unknown"
	}
	if err := writef(
		w,
		"history: warning: recovered corrupted db; moved %s to %s (cause: %s)\n",
		rec.Path,
		rec.Backup,
		cause,
	); err != nil {
		return err
	}
	if err := writeln(
		w,
		"history: warning: restore from an export with `resterm history import --in <path>` if needed",
	); err != nil {
		return err
	}
	return nil
}

func writef(w io.Writer, format string, args ...any) error {
	_, err := fmt.Fprintf(w, format, args...)
	return err
}

func writeln(w io.Writer, args ...any) error {
	_, err := fmt.Fprintln(w, args...)
	return err
}

func byteLabel(n int64) string {
	if n <= 0 {
		return "0 B"
	}
	const (
		kb = int64(1024)
		mb = kb * 1024
		gb = mb * 1024
	)
	switch {
	case n >= gb:
		return strconv.FormatFloat(float64(n)/float64(gb), 'f', 2, 64) + " GiB"
	case n >= mb:
		return strconv.FormatFloat(float64(n)/float64(mb), 'f', 2, 64) + " MiB"
	case n >= kb:
		return strconv.FormatFloat(float64(n)/float64(kb), 'f', 2, 64) + " KiB"
	default:
		return strconv.FormatInt(n, 10) + " B"
	}
}
