package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/unkn0wn-root/resterm/internal/cli"
	"github.com/unkn0wn-root/resterm/internal/mdterm"
	"github.com/unkn0wn-root/resterm/internal/rtfmt"
	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"github.com/unkn0wn-root/resterm/internal/update"
	str "github.com/unkn0wn-root/resterm/internal/util"
)

const (
	updateRepo         = "unkn0wn-root/resterm"
	updateCheckTimeout = 20 * time.Second
	updateApplyTimeout = 10 * time.Minute
	changelogMaxNotes  = 32 << 10
)

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
	if p.done {
		return
	}
	p.total = total
	p.render(true)
}

func (p *cliProgress) Advance(n int64) {
	if p.done || n <= 0 {
		return
	}
	p.downloaded += n
	p.render(false)
}

func (p *cliProgress) Done(err error) {
	if p.done {
		return
	}
	h := rtfmt.LogHandler(log.Printf, "progress finish write failed: %v")
	// on failure leave the bar at its true position; the caller prints the error
	if err != nil {
		p.done = true
		_ = rtfmt.Fprintln(p.out, h)
		return
	}
	if p.total > 0 {
		p.downloaded = p.total
		p.render(true)
		_ = rtfmt.Fprintln(p.out, h)
	} else {
		_ = rtfmt.Fprintf(p.out, "\r%s: %s\n", h, p.label, byteLabel(p.downloaded))
	}
	p.done = true
}

func (p *cliProgress) render(force bool) {
	if p.done {
		return
	}
	h := rtfmt.LogHandler(log.Printf, "progress write failed: %v")
	if p.total > 0 {
		percent := int((p.downloaded * 100) / p.total)
		if percent > 100 {
			percent = 100
		}
		if !force && percent == p.lastPct {
			return
		}
		p.lastPct = percent
		filled := int((p.downloaded * int64(p.barWidth)) / p.total)
		if filled > p.barWidth {
			filled = p.barWidth
		}
		bar := strings.Repeat("=", filled) + strings.Repeat(" ", p.barWidth-filled)
		_ = rtfmt.Fprintf(p.out, "\r%s: [%s] %3d%%", h, p.label, bar, percent)
		return
	}
	if !force && p.downloaded == 0 {
		return
	}
	_ = rtfmt.Fprintf(p.out, "\r%s: %s", h, p.label, byteLabel(p.downloaded))
}

type cliUpdater struct {
	cl    update.Client
	ver   string
	out   io.Writer
	color termcolor.Config
	width int
}

func newCLIUpdater(cl update.Client, ver string) cliUpdater {
	return cliUpdater{
		cl:  cl,
		ver: str.Trim(ver),
		out: os.Stdout,
		color: termcolor.Resolve(termcolor.Input{
			Mode:   termcolor.ModeAuto,
			TTY:    term.IsTerminal(int(os.Stdout.Fd())),
			Lookup: os.LookupEnv,
		}),
		width: min(cli.DetectWidth(os.Stdout), cli.MaxTextWidth),
	}
}

func (u cliUpdater) check(ctx context.Context) (update.Result, bool, error) {
	ctx, cancel := context.WithTimeout(ctx, updateCheckTimeout)
	defer cancel()

	plat, err := update.Detect()
	if err != nil {
		return update.Result{}, false, err
	}
	return u.cl.Check(ctx, u.ver, plat)
}

func (u cliUpdater) apply(ctx context.Context, res update.Result) error {
	ctx, cancel := context.WithTimeout(ctx, updateApplyTimeout)
	defer cancel()

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate executable: %w", err)
	}
	exe = resolveExecPath(exe)
	_ = rtfmt.Fprintf(
		u.out,
		"Updating resterm %s → %s\n",
		rtfmt.LogHandler(log.Printf, "print update header failed: %v"),
		u.ver,
		u.color.Bold(res.Info.Version),
	)
	prog := newCLIProgress(u.out, "Downloading")
	if err := u.cl.Apply(ctx, res, exe, prog); err != nil {
		return err
	}
	_ = rtfmt.Fprintln(u.out, rtfmt.LogHandler(log.Printf, "print checksum status failed: %v"), "Checksum verified.")
	_ = rtfmt.Fprintln(u.out, rtfmt.LogHandler(log.Printf, "print binary verification failed: %v"), "Binary verified.")
	_ = rtfmt.Fprintf(
		u.out,
		"resterm updated to %s\n",
		rtfmt.LogHandler(log.Printf, "print update notice failed: %v"),
		res.Info.Version,
	)
	return nil
}

func resolveExecPath(path string) string {
	clean := str.Trim(path)
	if clean == "" {
		return path
	}
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		return resolved
	}
	return clean
}

func (u cliUpdater) printNoUpdate() {
	_ = rtfmt.Fprintln(u.out, rtfmt.LogHandler(log.Printf, "print no-update failed: %v"), "resterm is up to date.")
}

func (u cliUpdater) printAvailable(res update.Result) {
	ver := u.color.Bold(res.Info.Version)
	_ = rtfmt.Fprintf(
		u.out,
		"New version available: %s\n",
		rtfmt.LogHandler(log.Printf, "print available failed: %v"),
		ver,
	)
}

func (u cliUpdater) printChangelog(res update.Result) {
	h := rtfmt.LogHandler(log.Printf, "print changelog divider failed: %v")
	divider := mdterm.Rule(u.color, u.width)
	_ = rtfmt.Fprintln(u.out, h, divider)
	body := mdterm.Render(clipNotes(res.Info.Notes), mdterm.Options{Width: u.width, Color: u.color})
	if body == "" {
		_ = rtfmt.Fprintln(
			u.out,
			rtfmt.LogHandler(log.Printf, "print changelog missing failed: %v"),
			"Changelog: not provided",
		)
		_ = rtfmt.Fprintln(u.out, h, divider)
		return
	}
	if err := rtfmt.Fprintln(
		u.out,
		rtfmt.LogHandler(log.Printf, "print changelog header failed: %v"),
		u.color.Bold("Changelog:"),
	); err != nil {
		return
	}
	if err := rtfmt.Fprintln(u.out, rtfmt.LogHandler(log.Printf, "print changelog body failed: %v"), body); err != nil {
		return
	}
	_ = rtfmt.Fprintln(u.out, h, divider)
}

// clipNotes bounds the renderer's input: the inline scanner is quadratic on
// adversarial text, and anything longer is unreadable as a changelog anyway.
func clipNotes(notes string) string {
	notes = str.Trim(notes)
	if len(notes) <= changelogMaxNotes {
		return notes
	}
	cut := notes[:changelogMaxNotes]
	if i := strings.LastIndexByte(cut, '\n'); i > 0 {
		cut = cut[:i]
	}
	return cut + "\n\n[changelog truncated]"
}
