package update

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
)

func (c Client) Apply(ctx context.Context, res Result, exe string, prog Progress) error {
	tmpPath, err := prepareTemp(filepath.Dir(exe))
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := c.stage(ctx, res, tmpPath, prog); err != nil {
		return err
	}

	return commitBinary(tmpPath, exe)
}

func (c Client) stage(ctx context.Context, res Result, path string, prog Progress) error {
	got, err := c.download(ctx, res.Bin, path, prog)
	if err != nil {
		return err
	}
	if got != res.Digest {
		return fmt.Errorf("checksum mismatch: got %x want %x", got, res.Digest)
	}

	if runtime.GOOS != "windows" {
		if err := os.Chmod(path, 0o755); err != nil {
			return fmt.Errorf("chmod new binary: %w", err)
		}
	}

	return verifyVersion(ctx, path, res.Info.Version)
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

func commitBinary(tmp, exe string) error {
	if runtime.GOOS == "windows" {
		return swapBinary(tmp, exe)
	}
	if err := os.Rename(tmp, exe); err != nil {
		return fmt.Errorf("replace binary: %w", err)
	}
	return nil
}

// swapBinary renames the running binary aside before moving tmp into place:
// Windows blocks deleting or overwriting a running executable but allows
// renaming it. The .old file stays behind until the next update overwrites
// it, which only succeeds once every process running the old binary has exited.
func swapBinary(tmp, exe string) error {
	// updaters before 0.46.1 staged the new binary as .new and left the file
	// behind when the update failed halfway.
	// @ToDo: keep this cleanup until updating from those versions is unlikely, then delete it
	_ = os.Remove(exe + ".new")

	old := exe + ".old"
	if err := os.Rename(exe, old); err != nil {
		return fmt.Errorf("move current binary aside (close running resterm instances and retry): %w", err)
	}
	if err := os.Rename(tmp, exe); err != nil {
		err = fmt.Errorf("install new binary: %w", err)
		if rerr := os.Rename(old, exe); rerr != nil {
			err = errors.Join(err, fmt.Errorf("restore old binary: %w", rerr))
		}
		return err
	}
	return nil
}
