package update

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
)

var (
	ErrPendingSwap = errors.New("update staged; restart required to complete")
)

type SwapStatus struct {
	Pending bool
	NewPath string
}

func ApplyWithProgress(
	ctx context.Context,
	c Client,
	res Result,
	exe string,
	prog Progress,
) (SwapStatus, error) {
	return c.apply(ctx, res, exe, prog)
}

func (c Client) apply(ctx context.Context, res Result, exe string, prog Progress) (SwapStatus, error) {
	if !res.HasSum {
		return SwapStatus{}, ErrNoChecksum
	}

	want, err := c.fetchChecksum(ctx, res.Sum, res.Bin.Name)
	if err != nil {
		return SwapStatus{}, err
	}

	tmpPath, err := prepareTemp(filepath.Dir(exe))
	if err != nil {
		return SwapStatus{}, err
	}
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := c.stage(ctx, res, want, tmpPath, prog); err != nil {
		return SwapStatus{}, err
	}

	return commitBinary(tmpPath, exe)
}

func prepareTemp(dir string) (string, error) {
	pat := "resterm-update-*"
	if runtime.GOOS == "windows" {
		pat = "resterm-update-*.exe"
	}

	tmp, err := os.CreateTemp(dir, pat)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("close temp file: %w", err)
	}
	return path, nil
}

func (c Client) stage(ctx context.Context, res Result, want [sha256.Size]byte, path string, prog Progress) error {
	got, err := c.download(ctx, res.Bin, path, prog)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("checksum mismatch: got %x want %x", got, want)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o755); err != nil {
			return fmt.Errorf("chmod new binary: %w", err)
		}
	}

	return verifyVersion(ctx, path, res.Info.Version)
}

// Windows can't replace a running executable, so we write it as .new
// and rely on the startup code to swap them before relaunching.
func commitBinary(tmpPath, exe string) (SwapStatus, error) {
	if runtime.GOOS == "windows" {
		dst := exe + ".new"
		if err := copyFile(tmpPath, dst); err != nil {
			return SwapStatus{}, err
		}
		return SwapStatus{Pending: true, NewPath: dst}, ErrPendingSwap
	}

	if err := os.Rename(tmpPath, exe); err != nil {
		return SwapStatus{}, fmt.Errorf("replace binary: %w", err)
	}
	return SwapStatus{}, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open src: %w", err)
	}
	defer func() {
		_ = in.Close()
	}()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("open dst: %w", err)
	}

	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return fmt.Errorf("copy: %w", err)
	}
	if err := out.Close(); err != nil {
		return fmt.Errorf("close dst: %w", err)
	}
	return nil
}
