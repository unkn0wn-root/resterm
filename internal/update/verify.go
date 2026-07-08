package update

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"
)

var ErrNoChecksum = errors.New("release has no checksum asset")

const checksumMaxBytes = 4 << 10

func (c Client) fetchChecksum(ctx context.Context, a Asset, bin string) ([sha256.Size]byte, error) {
	res, err := c.get(ctx, a.URL, "checksum")
	if err != nil {
		var sum [sha256.Size]byte
		return sum, err
	}
	defer func() {
		_ = res.Body.Close()
	}()

	return parseChecksum(res.Body, bin)
}

// parseChecksum reads the first line of a sha256sum-style body: a hex digest
// optionally followed by a file name, which must match bin when present.
func parseChecksum(r io.Reader, bin string) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte

	scanner := bufio.NewScanner(io.LimitReader(r, checksumMaxBytes))
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return sum, fmt.Errorf("read checksum: %w", err)
		}
		return sum, fmt.Errorf("empty checksum body")
	}

	fields := strings.Fields(scanner.Text())
	switch len(fields) {
	case 1:
	case 2:
		if name := strings.TrimPrefix(fields[1], "*"); name != bin {
			return sum, fmt.Errorf("checksum names %q, want %q", name, bin)
		}
	default:
		return sum, fmt.Errorf("invalid checksum line")
	}

	raw, err := hex.DecodeString(fields[0])
	if err != nil || len(raw) != sha256.Size {
		return sum, fmt.Errorf("invalid sha256 digest %q", fields[0])
	}
	copy(sum[:], raw)
	return sum, nil
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
