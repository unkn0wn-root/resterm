package main

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/muesli/termenv"

	"github.com/unkn0wn-root/resterm/internal/termcolor"
	"github.com/unkn0wn-root/resterm/internal/update"
)

func TestCLIUpdaterCheckDev(t *testing.T) {
	cl, err := update.NewClient(&http.Client{}, updateRepo)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	u := newCLIUpdater(cl, "dev")
	if _, _, err := u.check(context.Background()); !errors.Is(err, update.ErrDevBuild) {
		t.Fatalf("expected ErrDevBuild, got %v", err)
	}
}

func TestCLIProgressDone(t *testing.T) {
	var buf bytes.Buffer
	p := newCLIProgress(&buf, "Downloading")
	p.Start(100)
	p.Advance(100)
	p.Done(nil)

	out := buf.String()
	if !strings.Contains(out, "100%") {
		t.Fatalf("bar not completed on success: %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("progress line not terminated: %q", out)
	}
}

func TestCLIProgressDoneError(t *testing.T) {
	var buf bytes.Buffer
	p := newCLIProgress(&buf, "Downloading")
	p.Start(100)
	p.Advance(45)
	p.Done(errors.New("boom"))

	out := buf.String()
	if strings.Contains(out, "100%") {
		t.Fatalf("bar forced to 100%% on failure: %q", out)
	}
	if !strings.Contains(out, "45%") {
		t.Fatalf("bar lost its true position: %q", out)
	}
	if !strings.HasSuffix(out, "\n") {
		t.Fatalf("progress line not terminated: %q", out)
	}
}

func TestPrintChangelog(t *testing.T) {
	var buf bytes.Buffer
	u := cliUpdater{out: &buf, width: 40}
	res := update.Result{Info: update.Info{Notes: "## Changes\n* fix parser\n"}}
	u.printChangelog(res)

	div := strings.Repeat("─", 40)
	want := div + "\nChangelog:\nChanges\n-------\n• fix parser\n" + div + "\n"
	if got := buf.String(); got != want {
		t.Fatalf("printChangelog =\n%q\nwant\n%q", got, want)
	}
}

func TestPrintChangelogEmpty(t *testing.T) {
	// "```" is non-empty but renders to nothing; both must hit the fallback.
	for _, notes := range []string{"", "```"} {
		var buf bytes.Buffer
		u := cliUpdater{out: &buf, width: 10}
		u.printChangelog(update.Result{Info: update.Info{Notes: notes}})

		div := strings.Repeat("─", 10)
		want := div + "\nChangelog: not provided\n" + div + "\n"
		if got := buf.String(); got != want {
			t.Fatalf("printChangelog(%q) = %q, want %q", notes, got, want)
		}
	}
}

func TestClipNotes(t *testing.T) {
	if s := "short"; clipNotes(s) != s {
		t.Fatalf("short notes modified: %q", clipNotes(s))
	}
	long := strings.Repeat("aaaa bbbb\n", 8<<10)
	got := clipNotes(long)
	if len(got) > changelogMaxNotes+40 {
		t.Fatalf("clipped notes too long: %d", len(got))
	}
	if !strings.HasSuffix(got, "[changelog truncated]") {
		t.Fatalf("missing truncation marker: %q", got[len(got)-40:])
	}
}

func TestPrintChangelogColor(t *testing.T) {
	var buf bytes.Buffer
	u := cliUpdater{
		out:   &buf,
		width: 60,
		color: termcolor.Config{Enabled: true, Profile: termenv.ANSI},
	}
	res := update.Result{Info: update.Info{Notes: "## What's Changed\n* item"}}
	u.printChangelog(res)

	got := buf.String()
	if !strings.Contains(got, "\x1b[2m") {
		t.Fatalf("divider not faint: %q", got)
	}
	if !strings.Contains(got, "\x1b[36;1mWhat's Changed\x1b[0m") {
		t.Fatalf("h2 not styled: %q", got)
	}
}
