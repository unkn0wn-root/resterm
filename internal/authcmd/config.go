package authcmd

import (
	"path/filepath"
	"strings"
	"time"
)

type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"

	defaultHeader = "Authorization"
	defaultScheme = "Bearer"
)

type Result struct {
	Header string
	Value  string
	Token  string
	Type   string
	Expiry time.Time
}

type credential struct {
	Token     string
	Type      string
	Expiry    time.Time
	FetchedAt time.Time
}

type Config struct {
	Argv          []string
	Dir           string
	Format        Format
	Header        string
	Scheme        string
	TokenPath     string
	TypePath      string
	ExpiryPath    string
	ExpiresInPath string
	CacheKey      string
	TTL           time.Duration
	Timeout       time.Duration
}

func (cfg Config) normalize() Config {
	cfg.Dir = trim(cfg.Dir)
	cfg.Format = normalizeConfigFormat(cfg.Format)
	cfg.Header = trim(cfg.Header)
	if cfg.Header == "" {
		cfg.Header = defaultHeader
	}
	cfg.Scheme = trim(cfg.Scheme)
	cfg.TokenPath = trim(cfg.TokenPath)
	cfg.TypePath = trim(cfg.TypePath)
	cfg.ExpiryPath = trim(cfg.ExpiryPath)
	cfg.ExpiresInPath = trim(cfg.ExpiresInPath)
	cfg.CacheKey = trim(cfg.CacheKey)

	if len(cfg.Argv) > 0 {
		argv := append([]string(nil), cfg.Argv...)
		argv[0] = trim(argv[0])
		cfg.Argv = argv
	}
	return cfg
}

func (cfg Config) WithBaseTimeout(base time.Duration) Config {
	cfg.Timeout = cfg.timeoutFor(base)
	return cfg
}

func (cfg Config) headerName() string {
	if cfg.Header != "" {
		return cfg.Header
	}
	return defaultHeader
}

func (cfg Config) cacheKey() string {
	return cfg.CacheKey
}

func (cfg Config) usesCache() bool {
	return cfg.cacheKey() != ""
}

func (cfg Config) timeoutFor(base time.Duration) time.Duration {
	if cfg.Timeout > 0 && (base <= 0 || cfg.Timeout < base) {
		return cfg.Timeout
	}
	return base
}

func (cfg Config) commandName() string {
	if len(cfg.Argv) == 0 {
		return "command"
	}
	name := filepath.Base(cfg.Argv[0])
	if name == "" {
		return "command"
	}
	return name
}

func normalizeConfigFormat(raw Format) Format {
	switch strings.ToLower(strings.TrimSpace(string(raw))) {
	case "", string(FormatText):
		return FormatText
	case string(FormatJSON):
		return FormatJSON
	default:
		// Preserve unknown values after trimming so validate() can reject them.
		return Format(trim(string(raw)))
	}
}

func trim(raw string) string {
	return strings.TrimSpace(raw)
}
