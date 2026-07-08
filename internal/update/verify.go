package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// parseDigest decodes an api asset digest like "sha256:9f86d081...".
// Anything but sha256 is rejected so a future algorithm change fails loudly.
func parseDigest(v string) ([sha256.Size]byte, error) {
	var sum [sha256.Size]byte
	if v == "" {
		return sum, ErrNoDigest
	}

	enc, ok := strings.CutPrefix(v, "sha256:")
	if !ok {
		return sum, fmt.Errorf("unsupported digest %q", v)
	}
	raw, err := hex.DecodeString(enc)
	if err != nil || len(raw) != sha256.Size {
		return sum, fmt.Errorf("invalid sha256 digest %q", v)
	}
	return [sha256.Size]byte(raw), nil
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
