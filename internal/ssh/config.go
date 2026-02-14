package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/profileutil"
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
	cfg := baseCfg(p)
	cfg.Name = profileutil.Fallback(cfg.Name, "default")
	if cfg.Host == "" {
		return Cfg{}, errors.New("ssh host is required")
	}

	applyAuth(&cfg, p)

	if err := resolvePaths(&cfg, p); err != nil {
		return Cfg{}, err
	}
	if err := parseCfg(&cfg, p); err != nil {
		return Cfg{}, err
	}

	return cfg, nil
}

func baseCfg(p restfile.SSHProfile) Cfg {
	return Cfg{
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
}

func applyAuth(cfg *Cfg, p restfile.SSHProfile) {
	trimmedAllowEmpty(&cfg.User, p.User)
	rawIfSet(&cfg.Pass, p.Pass)
	rawIfSet(&cfg.KeyPass, p.KeyPass)
}

func parseCfg(cfg *Cfg, p restfile.SSHProfile) error {
	if err := profileutil.ParsePort("ssh", &cfg.Port, &cfg.PortRaw, p.PortStr); err != nil {
		return err
	}
	if err := profileutil.ParseDuration(
		"ssh",
		&cfg.Timeout,
		&cfg.TimeoutRaw,
		p.TimeoutStr,
	); err != nil {
		return err
	}
	if err := profileutil.ParseDuration(
		"ssh",
		&cfg.KeepAlive,
		&cfg.KeepAliveRaw,
		p.KeepAliveStr,
	); err != nil {
		return err
	}
	if err := profileutil.ParseRetries("ssh", &cfg.Retries, &cfg.RetriesRaw, p.RetriesStr); err != nil {
		return err
	}
	return nil
}

func defaultAgent(opt restfile.Opt[bool]) bool {
	if opt.Set {
		return opt.Val
	}
	return true
}

func defaultStrict(opt restfile.Opt[bool]) bool {
	if opt.Set {
		return opt.Val
	}
	return true
}

func defaultKnownHosts() (string, error) {
	return profileutil.ExpandPath("~/.ssh/known_hosts", "cannot resolve home directory for known_hosts")
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
		profileutil.BoolKey(cfg.Strict),
		profileutil.BoolKey(cfg.Agent),
		profileutil.BoolKey(cfg.Persist),
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

func resolvePaths(cfg *Cfg, p restfile.SSHProfile) error {
	if p.Key != "" {
		keyPath, err := profileutil.ExpandPath(p.Key, "cannot resolve home directory for ssh path")
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

	kh, err := profileutil.ExpandPath(cfg.KnownHosts, "cannot resolve home directory for ssh path")
	if err != nil {
		return err
	}
	cfg.KnownHosts = kh
	return nil
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
