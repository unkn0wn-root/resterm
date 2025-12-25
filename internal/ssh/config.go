package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const (
	defaultPort    = 22
	defaultTimeout = 15 * time.Second
	defaultTTL     = 10 * time.Minute
)

type Cfg struct {
	Name       string
	Host       string
	Port       int
	User       string
	Pass       string
	KeyPath    string
	KeyPass    string
	Agent      bool
	KnownHosts string
	Strict     bool
	Persist    bool
	Timeout    time.Duration
	KeepAlive  time.Duration
	Retries    int

	PortRaw      string
	TimeoutRaw   string
	KeepAliveRaw string
	RetriesRaw   string
	Label        string
}

func NormalizeProfile(p restfile.SSHProfile) (Cfg, error) {
	cfg := Cfg{
		Name:         strings.TrimSpace(p.Name),
		Host:         strings.TrimSpace(p.Host),
		Port:         defaultPort,
		Agent:        defaultAgent(p.Agent),
		KnownHosts:   strings.TrimSpace(p.KnownHosts),
		Strict:       defaultStrict(p.Strict),
		Persist:      p.Persist.Set && p.Persist.Val,
		Timeout:      defaultTimeout,
		KeepAlive:    0,
		Retries:      0,
		PortRaw:      strings.TrimSpace(p.PortStr),
		TimeoutRaw:   strings.TrimSpace(p.TimeoutStr),
		KeepAliveRaw: strings.TrimSpace(p.KeepAliveStr),
		RetriesRaw:   strings.TrimSpace(p.RetriesStr),
	}

	cfg.Name = fallback(cfg.Name, "default")
	if cfg.Host == "" {
		return Cfg{}, errors.New("ssh host is required")
	}

	trimmedAllowEmpty(&cfg.User, p.User)
	rawIfSet(&cfg.Pass, p.Pass)
	rawIfSet(&cfg.KeyPass, p.KeyPass)

	if err := resolvePaths(&cfg, p); err != nil {
		return Cfg{}, err
	}
	if err := parsePort(&cfg, p.PortStr); err != nil {
		return Cfg{}, err
	}
	if err := parseDuration(
		&cfg.Timeout,
		&cfg.TimeoutRaw,
		p.TimeoutStr,
		defaultTimeout,
	); err != nil {
		return Cfg{}, err
	}
	if err := parseDuration(&cfg.KeepAlive, &cfg.KeepAliveRaw, p.KeepAliveStr, 0); err != nil {
		return Cfg{}, err
	}
	if err := parseRetries(&cfg, p.RetriesStr); err != nil {
		return Cfg{}, err
	}

	return cfg, nil
}

func parsePort(cfg *Cfg, raw string) error {
	val := strings.TrimSpace(raw)
	if val == "" {
		return nil
	}

	cfg.PortRaw = val
	n, err := strconv.Atoi(val)
	if err != nil || n <= 0 || n > 65535 {
		return fmt.Errorf("invalid ssh port: %q", val)
	}

	cfg.Port = n
	return nil
}

func parseDuration(target *time.Duration, rawOut *string, raw string, def time.Duration) error {
	val := strings.TrimSpace(raw)
	if val == "" {
		if def > 0 && *target == 0 {
			*target = def
		}
		return nil
	}

	*rawOut = val
	dur, err := time.ParseDuration(val)
	if err != nil || dur < 0 {
		return fmt.Errorf("invalid ssh duration: %q", val)
	}

	*target = dur
	return nil
}

func parseRetries(cfg *Cfg, raw string) error {
	val := strings.TrimSpace(raw)
	if val == "" {
		return nil
	}

	cfg.RetriesRaw = val
	n, err := strconv.Atoi(val)
	if err != nil || n < 0 {
		return fmt.Errorf("invalid ssh retries: %q", val)
	}

	cfg.Retries = n
	return nil
}

func defaultAgent(opt restfile.SSHOpt[bool]) bool {
	if opt.Set {
		return opt.Val
	}
	return true
}

func defaultStrict(opt restfile.SSHOpt[bool]) bool {
	if opt.Set {
		return opt.Val
	}
	return true
}

func defaultKnownHosts() (string, error) {
	home := userHomeDir()
	if home == "" {
		return "", errors.New("cannot resolve home directory for known_hosts")
	}
	return expandPath(filepath.Join(home, ".ssh", "known_hosts"))
}

func userHomeDir() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return dir
}

func cacheKey(cfg Cfg) string {
	parts := []string{
		cfg.Label,
		cfg.Name,
		cfg.Host,
		strconv.Itoa(cfg.Port),
		cfg.User,
		authFingerprint(cfg),
		cfg.KnownHosts,
		boolKey(cfg.Strict),
		boolKey(cfg.Agent),
		boolKey(cfg.Persist),
		cfg.Timeout.String(),
		cfg.KeepAlive.String(),
		strconv.Itoa(cfg.Retries),
	}
	return strings.Join(parts, "|")
}

func authFingerprint(cfg Cfg) string {
	var parts []string

	if cfg.KeyPath != "" {
		parts = append(parts, "key:"+cfg.KeyPath)
	}

	if cfg.Pass != "" {
		parts = append(parts, "pass:"+hashSecret(cfg.Pass))
	}
	if cfg.KeyPass != "" {
		parts = append(parts, "keypass:"+hashSecret(cfg.KeyPass))
	}

	if len(parts) == 0 {
		return "noauth"
	}
	return strings.Join(parts, ",")
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func boolKey(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func expandPath(p string) (string, error) {
	path := strings.TrimSpace(p)
	if path == "" {
		return "", nil
	}

	if strings.HasPrefix(path, "~") {
		home := userHomeDir()
		if home == "" {
			return "", errors.New("cannot resolve home directory for ssh path")
		}
		path = filepath.Join(home, strings.TrimPrefix(path, "~"))
	}

	path = os.ExpandEnv(path)
	return filepath.Clean(path), nil
}

func resolvePaths(cfg *Cfg, p restfile.SSHProfile) error {
	if p.Key != "" {
		keyPath, err := expandPath(p.Key)
		if err != nil {
			return err
		}
		cfg.KeyPath = keyPath
	}

	if cfg.KnownHosts == "" {
		kh, err := defaultKnownHosts()
		if err != nil {
			return err
		}
		cfg.KnownHosts = kh
		return nil
	}

	kh, err := expandPath(cfg.KnownHosts)
	if err != nil {
		return err
	}
	cfg.KnownHosts = kh
	return nil
}

func fallback(val, def string) string {
	if val == "" {
		return def
	}
	return val
}

func trimmedAllowEmpty(target *string, val string) {
	if val == "" {
		return
	}
	*target = strings.TrimSpace(val)
}

func rawIfSet(target *string, val string) {
	if val == "" {
		return
	}
	*target = val
}
