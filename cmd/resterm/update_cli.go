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
