package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/update"
)

const (
	updateRepo         = "unkn0wn-root/resterm"
	updateCheckTimeout = 20 * time.Second
	updateApplyTimeout = 10 * time.Minute
)

var errUpdateDisabled = errors.New("update disabled for dev build")

const changelogDividerErr = "print changelog divider failed: %v"

type cliUpdater struct {
	cl  update.Client
	ver string
	out io.Writer
	err io.Writer
}

func newCLIUpdater(cl update.Client, ver string) cliUpdater {
	return cliUpdater{
		cl:  cl,
		ver: strings.TrimSpace(ver),
		out: os.Stdout,
		err: os.Stderr,
	}
}

func (u cliUpdater) check(ctx context.Context) (update.Result, bool, error) {
	if u.ver == "" || u.ver == "dev" {
		return update.Result{}, false, errUpdateDisabled
	}
	ctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()

	plat, err := update.Detect()
	if err != nil {
		return update.Result{}, false, err
	}

	res, err := u.cl.Check(ctx, u.ver, plat)
	if err != nil {
		if errors.Is(err, update.ErrNoUpdate) {
			return update.Result{}, false, nil
		}
		return update.Result{}, false, err
	}
	return res, true, nil
}

func (u cliUpdater) apply(ctx context.Context, res update.Result) (update.SwapStatus, error) {
	ctx, cancel := context.WithTimeout(ctx, updateApplyTimeout)
	defer cancel()

	exe, err := os.Executable()
	if err != nil {
		return update.SwapStatus{}, fmt.Errorf("locate executable: %w", err)
	}
	exe = resolveExecPath(exe)
	st, err := update.Apply(ctx, u.cl, res, exe)
	if errors.Is(err, update.ErrPendingSwap) {
		return st, err
	}
	if err != nil {
		return st, err
	}
	if _, werr := fmt.Fprintf(u.out, "resterm updated to %s\n", res.Info.Version); werr != nil {
		log.Printf("print update notice failed: %v", werr)
	}
	return st, nil
}

func resolveExecPath(path string) string {
	clean := strings.TrimSpace(path)
	if clean == "" {
		return path
	}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		return resolved
	}
	return clean
}

func (u cliUpdater) printNoUpdate() {
	if _, err := fmt.Fprintln(u.out, "resterm is up to date."); err != nil {
		log.Printf("print no-update failed: %v", err)
	}
}

func (u cliUpdater) printAvailable(res update.Result) {
	if _, err := fmt.Fprintf(u.out, "New version available: %s\n", res.Info.Version); err != nil {
		log.Printf("print available failed: %v", err)
	}
}

func (u cliUpdater) printStaged(st update.SwapStatus) {
	if st.NewPath != "" {
		if _, err := fmt.Fprintf(u.out, "Update staged at %s. Restart resterm to complete.\n", st.NewPath); err != nil {
			log.Printf("print staged path failed: %v", err)
		}
	} else {
		if _, err := fmt.Fprintln(u.out, "Update staged. Restart resterm to complete."); err != nil {
			log.Printf("print staged notice failed: %v", err)
		}
	}
}

func (u cliUpdater) printChangelog(res update.Result) {
	notes := strings.TrimSpace(res.Info.Notes)
	divider := strings.Repeat("-", 64)
	if _, err := fmt.Fprintln(u.out, divider); err != nil {
		log.Printf(changelogDividerErr, err)
	}
	if notes == "" {
		if _, err := fmt.Fprintln(u.out, "Changelog: not provided"); err != nil {
			log.Printf("print changelog missing failed: %v", err)
		}
		if _, err := fmt.Fprintln(u.out, divider); err != nil {
			log.Printf(changelogDividerErr, err)
		}
		return
	}
	if _, err := fmt.Fprintln(u.out, "Changelog:"); err != nil {
		log.Printf("print changelog header failed: %v", err)
		return
	}
	for _, line := range formatChangelog(notes) {
		if _, err := fmt.Fprintln(u.out, line); err != nil {
			log.Printf("print changelog body failed: %v", err)
			return
		}
	}
	if _, err := fmt.Fprintln(u.out, divider); err != nil {
		log.Printf(changelogDividerErr, err)
	}
}

func formatChangelog(raw string) []string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmedRight := strings.TrimRight(line, " \t")
		if strings.TrimSpace(trimmedRight) == "" {
			out = append(out, "")
			continue
		}

		leading := countLeadingSpaces(line)
		token := strings.TrimSpace(trimmedRight)
		switch {
		case strings.HasPrefix(token, "- ") || strings.HasPrefix(token, "* "):
			item := strings.TrimSpace(token[2:])
			out = append(out, strings.Repeat(" ", leading)+"• "+item)
		default:
			out = append(out, trimmedRight)
		}
	}
	return out
}

func countLeadingSpaces(s string) int {
	count := 0
	for _, r := range s {
		if r == ' ' || r == '\t' {
			if r == '\t' {
				count += 4
			} else {
				count++
			}
			continue
		}
		break
	}
	return count
}
