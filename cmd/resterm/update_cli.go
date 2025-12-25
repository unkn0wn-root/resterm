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

type cliProgress struct {
	out        io.Writer
	label      string
	total      int64
	downloaded int64
	barWidth   int
	lastPct    int
	done       bool
}

func newCLIProgress(out io.Writer, label string) *cliProgress {
	return &cliProgress{
		out:      out,
		label:    label,
		barWidth: 28,
		lastPct:  -1,
	}
}

func (p *cliProgress) Start(total int64) {
	if p == nil || p.done {
		return
	}
	p.total = total
	p.render(true)
}

func (p *cliProgress) Advance(n int64) {
	if p == nil || p.done || n <= 0 {
		return
	}
	p.downloaded += n
	p.render(false)
}

func (p *cliProgress) Finish() {
	if p == nil || p.done {
		return
	}
	if p.total > 0 {
		p.downloaded = p.total
		p.render(true)
		if _, err := fmt.Fprintln(p.out); err != nil {
			log.Printf("progress finish write failed: %v", err)
		}
	} else {
		line := fmt.Sprintf("\r%s: %s", p.label, humanBytes(p.downloaded))
		if _, err := fmt.Fprintln(p.out, line); err != nil {
			log.Printf("progress finish write failed: %v", err)
		}
	}
	p.done = true
}

func (p *cliProgress) render(force bool) {
	if p == nil || p.done {
		return
	}
	var line string
	if p.total > 0 {
		percent := 0
		if p.total > 0 {
			percent = int((p.downloaded * 100) / p.total)
			if percent > 100 {
				percent = 100
			}
		}
		if !force && percent == p.lastPct {
			return
		}
		p.lastPct = percent
		filled := 0
		if p.total > 0 {
			filled = int((p.downloaded * int64(p.barWidth)) / p.total)
			if filled > p.barWidth {
				filled = p.barWidth
			}
		}
		if filled < 0 {
			filled = 0
		}
		bar := strings.Repeat("=", filled) + strings.Repeat(" ", p.barWidth-filled)
		line = fmt.Sprintf("\r%s: [%s] %3d%%", p.label, bar, percent)
	} else {
		if !force && p.downloaded == 0 {
			return
		}
		line = fmt.Sprintf("\r%s: %s", p.label, humanBytes(p.downloaded))
	}
	if _, err := fmt.Fprint(p.out, line); err != nil {
		log.Printf("progress write failed: %v", err)
	}
}

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
	current := strings.TrimSpace(u.ver)
	if current == "" {
		current = "unknown"
	}
	if _, werr := fmt.Fprintf(
		u.out,
		"Updating resterm %s → %s\n",
		current,
		res.Info.Version,
	); werr != nil {
		log.Printf("print update header failed: %v", werr)
	}
	if !res.HasSum {
		if _, werr := fmt.Fprintln(
			u.out,
			"Warning: checksum not published; proceeding without verification.",
		); werr != nil {
			log.Printf("print checksum warning failed: %v", werr)
		}
	}
	prog := newCLIProgress(u.out, "Downloading")
	st, err := update.ApplyWithProgress(ctx, u.cl, res, exe, prog)
	if err != nil && !errors.Is(err, update.ErrPendingSwap) {
		return st, err
	}
	if res.HasSum {
		if _, werr := fmt.Fprintln(u.out, "Checksum verified."); werr != nil {
			log.Printf("print checksum status failed: %v", werr)
		}
	} else {
		if _, werr := fmt.Fprintln(u.out, "Checksum verification skipped."); werr != nil {
			log.Printf("print checksum skip failed: %v", werr)
		}
	}
	if _, werr := fmt.Fprintln(u.out, "Binary verified."); werr != nil {
		log.Printf("print binary verification failed: %v", werr)
	}
	if st.Pending {
		if _, werr := fmt.Fprintf(
			u.out,
			"Update staged at %s. Restart resterm to complete.\n",
			st.NewPath,
		); werr != nil {
			log.Printf("print staged path failed: %v", werr)
		}
	}
	if _, werr := fmt.Fprintf(u.out, "resterm updated to %s\n", res.Info.Version); werr != nil {
		log.Printf("print update notice failed: %v", werr)
	}
	return st, err
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

// Tabs count as 4 spaces for changelog indentation calculation
// so markdown nested lists look reasonable in the terminal.
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

func humanBytes(n int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
		gb = 1024 * mb
	)
	switch {
	case n >= gb:
		return fmt.Sprintf("%.2f GiB", float64(n)/float64(gb))
	case n >= mb:
		return fmt.Sprintf("%.2f MiB", float64(n)/float64(mb))
	case n >= kb:
		return fmt.Sprintf("%.2f KiB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%d B", n)
	}
}
