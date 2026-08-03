package ssh

import (
	"errors"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/connprofile"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const (
	defaultPort    = 22
	defaultTimeout = 15 * time.Second
)

type Config struct {
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
	Label      string
}

type execConfig struct {
	Config
	auth authSpec
	hk   hostKeySpec
	key  sessionKey
}

func NormalizeProfile(p restfile.SSHProfile) (Config, error) {
	cfg := baseCfg(p)
	cfg.Name = connprofile.Fallback(cfg.Name, connprofile.DefaultName)
	if cfg.Host == "" {
		return Config{}, errors.New("ssh host is required")
	}

	if err := resolvePaths(&cfg, p); err != nil {
		return Config{}, err
	}
	if err := parseCfg(&cfg, p); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func baseCfg(p restfile.SSHProfile) Config {
	return Config{
		Name:       strings.TrimSpace(p.Name),
		Host:       strings.TrimSpace(p.Host),
		Port:       defaultPort,
		User:       strings.TrimSpace(p.User),
		Pass:       p.Pass,
		KeyPass:    p.KeyPass,
		Agent:      p.Agent.Or(true),
		KnownHosts: strings.TrimSpace(p.KnownHosts),
		Strict:     p.Strict.Or(true),
		Persist:    p.Persist.Or(false),
		Timeout:    defaultTimeout,
	}
}

func parseCfg(cfg *Config, p restfile.SSHProfile) error {
	if err := connprofile.ParsePort("ssh", &cfg.Port, p.PortStr); err != nil {
		return err
	}
	if err := connprofile.ParseDuration("ssh", &cfg.Timeout, p.TimeoutStr); err != nil {
		return err
	}
	if err := connprofile.ParseDuration("ssh", &cfg.KeepAlive, p.KeepAliveStr); err != nil {
		return err
	}
	return connprofile.ParseRetries("ssh", &cfg.Retries, p.RetriesStr)
}

func defaultKnownHosts() (string, error) {
	return connprofile.ExpandPath(
		"~/.ssh/known_hosts",
		"cannot resolve home directory for known_hosts",
	)
}

func prepareExecConfig(cfg Config) (execConfig, error) {
	cfg = cfg.normalize()
	if cfg.Host == "" {
		return execConfig{}, errors.New("ssh host required")
	}
	if cfg.Port <= 0 || cfg.Port > 65535 {
		return execConfig{}, errors.New("ssh port out of range")
	}
	if cfg.Timeout < 0 {
		return execConfig{}, errors.New("ssh timeout out of range")
	}
	if cfg.KeepAlive < 0 {
		return execConfig{}, errors.New("ssh keepalive out of range")
	}
	if cfg.Retries < 0 {
		return execConfig{}, errors.New("ssh retries out of range")
	}

	return execConfig{
		Config: cfg,
		auth:   authSpecFor(cfg),
		hk:     hostKeySpecFor(cfg),
		key:    sessionKeyFor(cfg),
	}, nil
}

func (cfg Config) normalize() Config {
	trimStrings(&cfg.Name, &cfg.Host, &cfg.User, &cfg.KeyPath, &cfg.KnownHosts, &cfg.Label)
	defaultZero(&cfg.Name, connprofile.DefaultName)
	defaultZero(&cfg.Port, defaultPort)
	defaultZero(&cfg.Timeout, defaultTimeout)
	return cfg
}

func trimStrings(fields ...*string) {
	for _, f := range fields {
		*f = strings.TrimSpace(*f)
	}
}

func defaultZero[T comparable](v *T, def T) {
	var zero T
	if *v == zero {
		*v = def
	}
}

func resolvePaths(cfg *Config, p restfile.SSHProfile) error {
	if p.Key != "" {
		keyPath, err := connprofile.ExpandPath(p.Key, "cannot resolve home directory for ssh path")
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

	kh, err := connprofile.ExpandPath(cfg.KnownHosts, "cannot resolve home directory for ssh path")
	if err != nil {
		return err
	}
	cfg.KnownHosts = kh
	return nil
}
