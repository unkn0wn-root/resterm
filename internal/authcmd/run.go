package authcmd

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/errdef"
)

const (
	maxStdoutBytes = 256 << 10
	maxStderrBytes = 32 << 10
)

type runOutput struct {
	stdout []byte
	stderr string
}

type limitBuf struct {
	buf    bytes.Buffer
	kind   string
	limit  int
	cancel context.CancelFunc
	err    error
}

func newLimitBuf(kind string, limit int, cancel context.CancelFunc) *limitBuf {
	return &limitBuf{kind: kind, limit: limit, cancel: cancel}
}

func (b *limitBuf) Write(p []byte) (int, error) {
	room := b.limit - b.buf.Len()
	if room <= 0 {
		b.setErr()
		return 0, b.err
	}
	if len(p) > room {
		_, _ = b.buf.Write(p[:room])
		b.setErr()
		return room, b.err
	}
	return b.buf.Write(p)
}

func (b *limitBuf) setErr() {
	if b.err != nil {
		return
	}
	b.err = errdef.New(errdef.CodeHTTP, "%s exceeded %s", b.kind, sizeLabel(b.limit))
	if b.cancel != nil {
		b.cancel()
	}
}

func (b *limitBuf) Bytes() []byte {
	return append([]byte(nil), b.buf.Bytes()...)
}

func (b *limitBuf) String() string {
	return b.buf.String()
}

func run(ctx context.Context, cfg Config) (runOutput, error) {
	if len(cfg.Argv) == 0 {
		return runOutput{}, errdef.New(errdef.CodeHTTP, "missing command argv")
	}

	var (
		runCtx context.Context
		cancel context.CancelFunc
	)
	if cfg.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	cmd := exec.CommandContext(runCtx, cfg.Argv[0], cfg.Argv[1:]...)
	if cfg.Dir != "" {
		cmd.Dir = cfg.Dir
	}

	stdout := newLimitBuf("stdout", maxStdoutBytes, cancel)
	stderr := newLimitBuf("stderr", maxStderrBytes, cancel)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if limErr := firstRunErr(stdout.err, stderr.err); limErr != nil {
		return runOutput{}, limErr
	}
	if err == nil {
		return runOutput{stdout: stdout.Bytes(), stderr: stderr.String()}, nil
	}

	if runCtx.Err() != nil {
		if cfg.Timeout > 0 && errors.Is(runCtx.Err(), context.DeadlineExceeded) {
			return runOutput{}, errdef.New(
				errdef.CodeHTTP,
				"command %q timed out after %s",
				cfg.commandName(),
				cfg.Timeout,
			)
		}
		return runOutput{}, errdef.Wrap(errdef.CodeHTTP, runCtx.Err(), "run command %q", cfg.commandName())
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return runOutput{}, errdef.New(
			errdef.CodeHTTP,
			"command %q exited with status %d%s",
			cfg.commandName(),
			exitErr.ExitCode(),
			stderrSuffix(stderr.String()),
		)
	}

	return runOutput{}, errdef.Wrap(errdef.CodeHTTP, err, "run command %q", cfg.commandName())
}

func firstRunErr(errs ...error) error {
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
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
