package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/history"
	histdb "github.com/unkn0wn-root/resterm/internal/history/sqlite"
)

func TestHandleHistorySubcommandNotMatched(t *testing.T) {
	handled, err := handleHistorySubcommand([]string{"init"})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if handled {
		t.Fatalf("expected not handled")
	}
}

func TestRunHistoryRequiresSubcommand(t *testing.T) {
	err := runHistory(nil)
	if err == nil {
		t.Fatalf("expected error for missing subcommand")
	}
}

func TestRunHistoryUnknownSubcommand(t *testing.T) {
	err := runHistory([]string{"unknown"})
	if err == nil {
		t.Fatalf("expected error for unknown subcommand")
	}
}

func TestRunHistoryHelpFlagShowsUsage(t *testing.T) {
	stdout, stderr, err := captureHistoryIO(t, func() error {
		return runHistory([]string{"-h"})
	})
	if err != nil {
		t.Fatalf("help flag: %v", err)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("expected empty stderr on help flag, got %q", stderr)
	}
	if !strings.Contains(stdout, "Usage: resterm history <export|import|backup|stats|check|compact> [flags]") {
		t.Fatalf("expected history usage in stdout, got %q", stdout)
	}
}

func TestRunHistoryMaintenanceCommands(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RESTERM_CONFIG_DIR", dir)

	if err := runHistory([]string{"stats"}); err != nil {
		t.Fatalf("stats: %v", err)
	}
	if err := runHistory([]string{"check"}); err != nil {
		t.Fatalf("check: %v", err)
	}
	if err := runHistory([]string{"check", "--full"}); err != nil {
		t.Fatalf("check full: %v", err)
	}
	if err := runHistory([]string{"compact"}); err != nil {
		t.Fatalf("compact: %v", err)
	}
}

func TestRunHistoryE2E(t *testing.T) {
	srcDir := t.TempDir()
	t.Setenv("RESTERM_CONFIG_DIR", srcDir)

	src := histdb.New(filepath.Join(srcDir, "history.db"))
	if err := src.Load(); err != nil {
		t.Fatalf("load src: %v", err)
	}
	t1 := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	t2 := t1.Add(1 * time.Minute)
	if err := src.Append(
		history.Entry{ID: "1", ExecutedAt: t1, Method: "GET", URL: "https://one.test"},
	); err != nil {
		t.Fatalf("append src 1: %v", err)
	}
	if err := src.Append(
		history.Entry{ID: "2", ExecutedAt: t2, Method: "POST", URL: "https://two.test"},
	); err != nil {
		t.Fatalf("append src 2: %v", err)
	}
	if err := src.Close(); err != nil {
		t.Fatalf("close src: %v", err)
	}

	jsonPath := filepath.Join(t.TempDir(), "history.json")
	stdout, stderr, err := captureHistoryIO(t, func() error {
		return runHistory([]string{"export", "--out", jsonPath})
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on export, got %q", stderr)
	}
	if !strings.Contains(stdout, "Exported 2 history entries") {
		t.Fatalf("unexpected export output: %q", stdout)
	}

	var exported []history.Entry
	data, err := os.ReadFile(jsonPath)
	if err != nil {
		t.Fatalf("read export file: %v", err)
	}
	if err := json.Unmarshal(data, &exported); err != nil {
		t.Fatalf("decode export file: %v", err)
	}
	if len(exported) != 2 {
		t.Fatalf("expected 2 exported rows, got %d", len(exported))
	}

	backupPath := filepath.Join(t.TempDir(), "history.bak.db")
	stdout, stderr, err = captureHistoryIO(t, func() error {
		return runHistory([]string{"backup", "--out", backupPath})
	})
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on backup, got %q", stderr)
	}
	if !strings.Contains(stdout, "Created history backup") {
		t.Fatalf("unexpected backup output: %q", stdout)
	}

	bak := histdb.New(backupPath)
	if err := bak.Load(); err != nil {
		t.Fatalf("load backup db: %v", err)
	}
	if got := bak.Entries(); len(got) != 2 {
		t.Fatalf("expected 2 rows in backup db, got %d", len(got))
	}
	_ = bak.Close()

	dstDir := t.TempDir()
	t.Setenv("RESTERM_CONFIG_DIR", dstDir)

	stdout, stderr, err = captureHistoryIO(t, func() error {
		return runHistory([]string{"import", "--in", jsonPath})
	})
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on import, got %q", stderr)
	}
	if !strings.Contains(stdout, "Imported 2 history entries") {
		t.Fatalf("unexpected import output: %q", stdout)
	}

	dst := histdb.New(filepath.Join(dstDir, "history.db"))
	if err := dst.Load(); err != nil {
		t.Fatalf("load dst: %v", err)
	}
	if got := dst.Entries(); len(got) != 2 {
		t.Fatalf("expected 2 rows in dst db, got %d", len(got))
	}
	_ = dst.Close()

	stdout, stderr, err = captureHistoryIO(t, func() error {
		return runHistory([]string{"stats"})
	})
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on stats, got %q", stderr)
	}
	if !strings.Contains(stdout, "Rows: 2") || !strings.Contains(stdout, "Schema:") {
		t.Fatalf("unexpected stats output: %q", stdout)
	}

	stdout, stderr, err = captureHistoryIO(t, func() error {
		return runHistory([]string{"check"})
	})
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on check, got %q", stderr)
	}
	if !strings.Contains(stdout, "History integrity check (quick): ok") {
		t.Fatalf("unexpected check output: %q", stdout)
	}

	stdout, stderr, err = captureHistoryIO(t, func() error {
		return runHistory([]string{"check", "--full"})
	})
	if err != nil {
		t.Fatalf("check full: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on full check, got %q", stderr)
	}
	if !strings.Contains(stdout, "History integrity check (full): ok") {
		t.Fatalf("unexpected full check output: %q", stdout)
	}

	stdout, stderr, err = captureHistoryIO(t, func() error {
		return runHistory([]string{"compact"})
	})
	if err != nil {
		t.Fatalf("compact: %v", err)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr on compact, got %q", stderr)
	}
	if !strings.Contains(stdout, "Compacted history db") {
		t.Fatalf("unexpected compact output: %q", stdout)
	}
}

func TestRunHistoryRecoveryWarning(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("RESTERM_CONFIG_DIR", dir)

	dbPath := filepath.Join(dir, "history.db")
	if err := os.WriteFile(dbPath, []byte("not-a-sqlite-db"), 0o644); err != nil {
		t.Fatalf("write corrupt db: %v", err)
	}

	stdout, stderr, err := captureHistoryIO(t, func() error {
		return runHistory([]string{"stats"})
	})
	if err != nil {
		t.Fatalf("stats: %v", err)
	}
	if !strings.Contains(stdout, "History DB:") {
		t.Fatalf("expected stats output, got %q", stdout)
	}
	if !strings.Contains(stderr, "history: warning: recovered corrupted db") {
		t.Fatalf("expected recovery warning, got %q", stderr)
	}
	if !strings.Contains(stderr, "resterm history import --in <path>") {
		t.Fatalf("expected recovery guidance in stderr, got %q", stderr)
	}

	matches, err := filepath.Glob(filepath.Join(dir, "history.db.corrupt-*"))
	if err != nil {
		t.Fatalf("glob corrupt backups: %v", err)
	}
	if len(matches) == 0 {
		t.Fatalf("expected quarantined corrupt db file")
	}
}

func TestRunHistoryFlagErrorsHaveCommandPrefix(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{args: []string{"export", "--bad"}, want: "history export:"},
		{args: []string{"import", "--bad"}, want: "history import:"},
		{args: []string{"backup", "--bad"}, want: "history backup:"},
		{args: []string{"stats", "--bad"}, want: "history stats:"},
		{args: []string{"check", "--bad"}, want: "history check:"},
		{args: []string{"compact", "--bad"}, want: "history compact:"},
	}
	for _, tc := range cases {
		err := runHistory(tc.args)
		if err == nil {
			t.Fatalf("expected error for %v", tc.args)
		}
		if !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("expected error %q to contain %q", err.Error(), tc.want)
		}
	}
}

func TestRunHistorySubcommandHelpShowsUsage(t *testing.T) {
	stdout, stderr, err := captureHistoryIO(t, func() error {
		return runHistory([]string{"export", "-h"})
	})
	if err != nil {
		t.Fatalf("export -h: %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Fatalf("expected empty stdout on help, got %q", stdout)
	}
	if !strings.Contains(stderr, "Usage: resterm history export [flags]") {
		t.Fatalf("expected usage in stderr, got %q", stderr)
	}
	if !strings.Contains(stderr, "-out") {
		t.Fatalf("expected --out flag in help output, got %q", stderr)
	}
}

func captureHistoryIO(t *testing.T, fn func() error) (string, string, error) {
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
