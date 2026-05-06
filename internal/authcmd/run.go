package authcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

const (
	maxStdoutBytes = 256 << 10
	maxStderrBytes = 32 << 10

	defaultCommandName = "command"
)

type runCancel func(error)

type limitBuf struct {
	buf    bytes.Buffer
	kind   string
	limit  int
	cancel runCancel
	err    error
}

func newLimitBuf(kind string, limit int, cancel runCancel) *limitBuf {
	return &limitBuf{kind: kind, limit: limit, cancel: cancel}
}

func (b *limitBuf) Write(p []byte) (int, error) {
	room := b.limit - b.buf.Len()
	if room <= 0 {
		return 0, b.overflowErr()
	}
	if len(p) > room {
		_, _ = b.buf.Write(p[:room])
		return room, b.overflowErr()
	}
	return b.buf.Write(p)
}

func (b *limitBuf) overflowErr() error {
	if b.err != nil {
		return b.err
	}
	b.err = diag.Newf(diag.ClassAuth, "%s exceeded %s", b.kind, sizeLabel(b.limit))
	if b.cancel != nil {
		b.cancel(b.err)
	}
	return b.err
}

func (b *limitBuf) Bytes() []byte {
	return append([]byte(nil), b.buf.Bytes()...)
}

func (b *limitBuf) String() string {
	return b.buf.String()
}

func (cfg commandConfig) name() string {
	if len(cfg.Argv) == 0 {
		return defaultCommandName
	}
	return filepath.Base(cfg.Argv[0])
}

func run(ctx context.Context, cfg commandConfig) ([]byte, error) {
	if len(cfg.Argv) == 0 {
		return nil, diag.New(diag.ClassAuth, "missing command argv")
	}

	runCtx, cancel := newRunContext(ctx, cfg.Timeout)
	defer cancel(nil)

	cmd := exec.CommandContext(runCtx, cfg.Argv[0], cfg.Argv[1:]...)
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}

	stdout := newLimitBuf("stdout", maxStdoutBytes, cancel)
	stderr := newLimitBuf("stderr", maxStderrBytes, cancel)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err == nil {
		if cause := runCause(runCtx); cause != nil {
			return nil, cause
		}
		return stdout.Bytes(), nil
	}

	return nil, mapRunError(runCtx, cfg, err, stderr.String())
}

func stderrSuffix(text string) string {
	msg := strings.TrimSpace(text)
	if msg == "" {
		return ""
	}
	return fmt.Sprintf(": %s", msg)
}

func sizeLabel(n int) string {
	if n%(1<<10) == 0 {
		return fmt.Sprintf("%d KiB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}

func newRunContext(ctx context.Context, timeout time.Duration) (context.Context, runCancel) {
	baseCtx, cancelCause := context.WithCancelCause(ctx)
	if timeout <= 0 {
		return baseCtx, runCancel(cancelCause)
	}

	timeoutCtx, cancelTimeout := context.WithTimeout(baseCtx, timeout)
	return timeoutCtx, func(err error) {
		if err != nil {
			cancelCause(err)
		}
		cancelTimeout()
		cancelCause(nil)
	}
}

func runCause(runCtx context.Context) error {
	cause := context.Cause(runCtx)
	if ignoreCause(cause) {
		return nil
	}
	return cause
}

func ignoreCause(err error) bool {
	switch {
	case err == nil:
		return true
	case errors.Is(err, context.Canceled):
		return true
	case errors.Is(err, context.DeadlineExceeded):
		return true
	default:
		return false
	}
}

func mapRunError(runCtx context.Context, cfg commandConfig, err error, stderr string) error {
	if cause := runCause(runCtx); cause != nil {
		return cause
	}
	if runCtx.Err() != nil {
		if cfg.Timeout > 0 && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return diag.Newf(
				diag.ClassTimeout,
				"command %q timed out after %s",
				cfg.name(),
				cfg.Timeout,
			)
		}
		return diag.WrapAsf(diag.ClassAuth, runCtx.Err(),
			"run command %q",
			cfg.name(),
		)
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return diag.Newf(
			diag.ClassAuth,
			"command %q exited with status %d%s",
			cfg.name(),
			exitErr.ExitCode(),
			stderrSuffix(stderr),
		)
	}

	return diag.WrapAsf(diag.ClassAuth, err, "run command %q", cfg.name())
}
