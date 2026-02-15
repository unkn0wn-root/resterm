package profileutil

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/duration"
)

func Fallback(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

func BoolKey(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func ParsePort(label string, target *int, rawOut *string, raw string) error {
	val := strings.TrimSpace(raw)
	if val == "" {
		return nil
	}

	*rawOut = val
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 || n > 65535 {
		return fmt.Errorf("invalid %s port: %q", label, val)
	}
	*target = n
	return nil
}

func ParseDuration(
	label string,
	target *time.Duration,
	rawOut *string,
	raw string,
) error {
	val := strings.TrimSpace(raw)
	if val == "" {
		return nil
	}

	*rawOut = val
	dur, ok := duration.Parse(val)
	if !ok || dur < 0 {
		return fmt.Errorf("invalid %s duration: %q", label, val)
	}
	*target = dur
	return nil
}

func ParseRetries(label string, target *int, rawOut *string, raw string) error {
	val := strings.TrimSpace(raw)
	if val == "" {
		return nil
	}

	*rawOut = val
	n, err := strconv.Atoi(val)
	if err != nil || n < 0 {
		return fmt.Errorf("invalid %s retries: %q", label, val)
	}
	*target = n
	return nil
}

func ExpandPath(path, homeErr string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", nil
	}
	if strings.HasPrefix(p, "~") {
		home, tail, err := resolveTildeHome(p)
		if err != nil {
			if strings.TrimSpace(homeErr) == "" {
				homeErr = "cannot resolve home directory"
			}
			return "", errors.New(homeErr)
		}
		if tail == "" {
			p = home
		} else {
			p = filepath.Join(home, tail)
		}
	}
	p = os.ExpandEnv(p)
	return filepath.Clean(p), nil
}

func resolveTildeHome(path string) (home string, tail string, err error) {
	if path == "~" {
		home, err = os.UserHomeDir()
		return home, "", err
	}

	rest := strings.TrimPrefix(path, "~")
	if strings.HasPrefix(rest, "/") || strings.HasPrefix(rest, "\\") {
		home, err = os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		return home, strings.TrimLeft(rest, `/\`), nil
	}

	username := rest
	if i := strings.IndexAny(rest, `/\`); i >= 0 {
		username = rest[:i]
		tail = strings.TrimLeft(rest[i:], `/\`)
	}
	if username == "" {
		home, err = os.UserHomeDir()
		if err != nil {
			return "", "", err
		}
		return home, tail, nil
	}

	usr, err := user.Lookup(username)
	if err != nil {
		return "", "", err
	}
	return usr.HomeDir, tail, nil
}
