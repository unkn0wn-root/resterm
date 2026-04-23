package ssh

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/connprofile"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

const (
	defaultPort    = 22
	defaultTimeout = 15 * time.Second
	defaultTTL     = 10 * time.Minute
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

	PortRaw      string
	TimeoutRaw   string
	KeepAliveRaw string
	RetriesRaw   string
	Label        string
}

type endpoint struct {
	host string
	port int
}

type sessionKey struct {
	label       string
	name        string
	host        string
	port        int
	user        string
	keyPath     string
	passHash    string
	keyPassHash string
	knownHosts  string
	strict      bool
	agent       bool
	persist     bool
	timeout     time.Duration
	keepAlive   time.Duration
	retries     int
}

type execConfig struct {
	Config
	ep    endpoint
	auth  authSpec
	hk    hostKeySpec
	retry int
	key   sessionKey
}

func NormalizeProfile(p restfile.SSHProfile) (Config, error) {
	cfg := baseCfg(p)
	cfg.Name = connprofile.Fallback(cfg.Name, "default")
	if cfg.Host == "" {
		return Config{}, errors.New("ssh host is required")
	}

	applyAuth(&cfg, p)

	if err := resolvePaths(&cfg, p); err != nil {
		return Config{}, err
	}
	if err := parseCfg(&cfg, p); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func baseCfg(p restfile.SSHProfile) Config {
	cfg := Config{
		Name:         p.Name,
		Host:         p.Host,
		Port:         defaultPort,
		Agent:        defaultOpt(p.Agent, true),
		KnownHosts:   p.KnownHosts,
		Strict:       defaultOpt(p.Strict, true),
		Persist:      p.Persist.Set && p.Persist.Val,
		Timeout:      defaultTimeout,
		KeepAlive:    0,
		Retries:      0,
		PortRaw:      p.PortStr,
		TimeoutRaw:   p.TimeoutStr,
		KeepAliveRaw: p.KeepAliveStr,
		RetriesRaw:   p.RetriesStr,
	}
	trimConfigStrings(&cfg)
	return cfg
}

func applyAuth(cfg *Config, p restfile.SSHProfile) {
	trimmedAllowEmpty(&cfg.User, p.User)
	rawIfSet(&cfg.Pass, p.Pass)
	rawIfSet(&cfg.KeyPass, p.KeyPass)
}

func parseCfg(cfg *Config, p restfile.SSHProfile) error {
	if err := connprofile.ParsePort("ssh", &cfg.Port, &cfg.PortRaw, p.PortStr); err != nil {
		return err
	}
	if err := connprofile.ParseDuration(
		"ssh",
		&cfg.Timeout,
		&cfg.TimeoutRaw,
		p.TimeoutStr,
	); err != nil {
		return err
	}
	if err := connprofile.ParseDuration(
		"ssh",
		&cfg.KeepAlive,
		&cfg.KeepAliveRaw,
		p.KeepAliveStr,
	); err != nil {
		return err
	}
	if err := connprofile.ParseRetries(
		"ssh",
		&cfg.Retries,
		&cfg.RetriesRaw,
		p.RetriesStr,
	); err != nil {
		return err
	}
	return nil
}

func defaultOpt(opt restfile.Opt[bool], def bool) bool {
	if opt.Set {
		return opt.Val
	}
	return def
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

	sp := authSpecFor(cfg)
	return execConfig{
		Config: cfg,
		ep: endpoint{
			host: cfg.Host,
			port: cfg.Port,
		},
		auth:  sp,
		hk:    hostKeySpecFor(cfg),
		retry: cfg.Retries,
		key:   sessionKeyFor(cfg),
	}, nil
}

func (cfg Config) normalize() Config {
	trimConfigStrings(&cfg)
	defaultZero(&cfg.Name, "default")
	defaultZero(&cfg.Port, defaultPort)
	defaultZero(&cfg.Timeout, defaultTimeout)
	return cfg
}

func trimConfigStrings(cfg *Config) {
	if cfg == nil {
		return
	}
	trimStrings(
		&cfg.Name,
		&cfg.Host,
		&cfg.PortRaw,
		&cfg.User,
		&cfg.KeyPath,
		&cfg.KnownHosts,
		&cfg.TimeoutRaw,
		&cfg.KeepAliveRaw,
		&cfg.RetriesRaw,
		&cfg.Label,
	)
}

func trimStrings(fields ...*string) {
	for _, f := range fields {
		if f != nil {
			*f = strings.TrimSpace(*f)
		}
	}
}

func defaultZero[T comparable](v *T, def T) {
	var zero T
	if v != nil && *v == zero {
		*v = def
	}
}

// sessionKeyFor expects cfg to have already passed through Config.normalize.
func sessionKeyFor(cfg Config) sessionKey {
	return sessionKey{
		label:       cfg.Label,
		name:        cfg.Name,
		host:        cfg.Host,
		port:        cfg.Port,
		user:        cfg.User,
		keyPath:     cfg.KeyPath,
		passHash:    hashIfSet(cfg.Pass),
		keyPassHash: hashIfSet(cfg.KeyPass),
		knownHosts:  cfg.KnownHosts,
		strict:      cfg.Strict,
		agent:       cfg.Agent,
		persist:     cfg.Persist,
		timeout:     cfg.Timeout,
		keepAlive:   cfg.KeepAlive,
		retries:     cfg.Retries,
	}
}

func hashIfSet(secret string) string {
	if secret == "" {
		return ""
	}
	return hashSecret(secret)
}

func hashSecret(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
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
