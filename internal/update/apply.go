package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

var (
	ErrPendingSwap = errors.New("update staged; restart required to complete")
)

type SwapStatus struct {
	Pending bool
	NewPath string
}

type Progress interface {
	Start(total int64)
	Advance(n int64)
	Finish()
}

func (c Client) Download(ctx context.Context, a Asset, dst string, prog Progress) (int64, error) {
	if c.http == nil {
		return 0, errNilHTTPClient
	}
	if a.URL == "" {
		return 0, fmt.Errorf("empty asset url")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return 0, fmt.Errorf("build asset request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	res, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("download asset: %w", err)
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download asset failed: %s", res.Status)
	}

	f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return 0, fmt.Errorf("open temp file: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	var reader io.Reader = res.Body
	if prog != nil {
		total := a.Size
		if total <= 0 && res.ContentLength > 0 {
			total = res.ContentLength
		}
		prog.Start(total)
		defer prog.Finish()
		reader = io.TeeReader(res.Body, progressWriter{progress: prog})
	}

	n, err := io.Copy(f, reader)
	if err != nil {
		return n, fmt.Errorf("write asset: %w", err)
	}

	if a.Size > 0 && n != a.Size {
		return n, fmt.Errorf("download size mismatch: got %d want %d", n, a.Size)
	}
	return n, nil
}

type progressWriter struct {
	progress Progress
}

func (w progressWriter) Write(p []byte) (int, error) {
	if len(p) > 0 {
		w.progress.Advance(int64(len(p)))
	}
	return len(p), nil
}

func (c Client) FetchChecksum(ctx context.Context, a Asset) (string, error) {
	if !strings.HasSuffix(a.Name, ".sha256") {
		return "", fmt.Errorf("not a checksum asset")
	}
	if c.http == nil {
		return "", errNilHTTPClient
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.URL, nil)
	if err != nil {
		return "", fmt.Errorf("build checksum request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	res, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("download checksum: %w", err)
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download checksum failed: %s", res.Status)
	}

	line, err := readFirstToken(res.Body)
	if err != nil {
		return "", err
	}
	if len(line) != 64 {
		return "", fmt.Errorf("unexpected checksum length: %d", len(line))
	}
	return strings.ToLower(line), nil
}

func readFirstToken(r io.Reader) (string, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() {
		return "", fmt.Errorf("empty checksum body")
	}

	txt := scanner.Text()
	fields := strings.Fields(txt)
	if len(fields) == 0 {
		return "", fmt.Errorf("invalid checksum line")
	}
	return fields[0], nil
}

func verifyChecksum(path, want string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open for checksum: %w", err)
	}
	defer func() {
		_ = f.Close()
	}()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("hash binary: %w", err)
	}

	got := hex.EncodeToString(h.Sum(nil))
	if got != want {
		return fmt.Errorf("checksum mismatch: got %s want %s", got, want)
	}
	return nil
}

func verifyVersion(ctx context.Context, path, want string) error {
	if want == "" {
		return nil
	}

	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, path, "--version")
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("version command failed: %w", err)
	}
	if !strings.Contains(string(out), want) {
		return fmt.Errorf("version mismatch: output does not contain %s", want)
	}
	return nil
}

func Apply(ctx context.Context, c Client, res Result, exe string) (SwapStatus, error) {
	return apply(ctx, c, res, exe, nil)
}

func ApplyWithProgress(
	ctx context.Context,
	c Client,
	res Result,
	exe string,
	prog Progress,
) (SwapStatus, error) {
	return apply(ctx, c, res, exe, prog)
}

func apply(
	ctx context.Context,
	c Client,
	res Result,
	exe string,
	prog Progress,
) (SwapStatus, error) {
	tmpPath, err := prepareTemp(filepath.Dir(exe))
	if err != nil {
		return SwapStatus{}, err
	}
	defer func() {
		_ = os.Remove(tmpPath)
	}()

	if err := stageBinary(ctx, c, res, tmpPath, prog); err != nil {
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

func stageBinary(ctx context.Context, c Client, res Result, path string, prog Progress) error {
	if _, err := c.Download(ctx, res.Bin, path, prog); err != nil {
		return err
	}

	if res.HasSum {
		sum, err := c.FetchChecksum(ctx, res.Sum)
		if err != nil {
			return err
		}
		if err := verifyChecksum(path, sum); err != nil {
			return err
		}
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
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return nil
}
