package authcmd

import (
	"slices"
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

type commandConfig struct {
	Argv    []string
	Dir     string
	Timeout time.Duration
}

type extractConfig struct {
	Format        Format
	TokenPath     string
	TypePath      string
	ExpiryPath    string
	ExpiresInPath string
}

type outputConfig struct {
	Header string
	Scheme string
	TTL    time.Duration
}

type cacheSeed struct {
	command commandConfig
	extract extractConfig
	ttl     time.Duration
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

func (cfg Config) WithBaseTimeout(base time.Duration) Config {
	cfg.Timeout = cfg.timeoutFor(base)
	return cfg
}

func (cfg Config) HeaderName() string {
	return cfg.output().headerName()
}

func (cfg Config) normalize() Config {
	cfg.Dir = trim(cfg.Dir)
	cfg.Format = normalizeConfigFormat(cfg.Format)
	cfg.Header = trim(cfg.Header)
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

func (cfg Config) cacheSeed() cacheSeed {
	return cacheSeed{
		command: cfg.command(),
		extract: cfg.extract(),
		ttl:     cfg.TTL,
	}
}

func (cfg Config) inheritedFrom(base Config) Config {
	merged := cfg
	merged.Header = inheritIfZero(merged.Header, base.Header)
	merged.Scheme = inheritIfZero(merged.Scheme, base.Scheme)
	return cfg.cacheSeed().inheritedFrom(base.cacheSeed()).apply(merged)
}

func (cfg Config) cacheSeedDiff(other Config) (string, bool) {
	return cfg.cacheSeed().diff(other.cacheSeed())
}

func (cfg Config) command() commandConfig {
	return commandConfig{
		Argv:    append([]string(nil), cfg.Argv...),
		Dir:     cfg.Dir,
		Timeout: cfg.Timeout,
	}
}

func (cfg Config) extract() extractConfig {
	return extractConfig{
		Format:        cfg.Format,
		TokenPath:     cfg.TokenPath,
		TypePath:      cfg.TypePath,
		ExpiryPath:    cfg.ExpiryPath,
		ExpiresInPath: cfg.ExpiresInPath,
	}
}

func (cfg Config) output() outputConfig {
	return outputConfig{
		Header: cfg.Header,
		Scheme: cfg.Scheme,
		TTL:    cfg.TTL,
	}
}

func (cfg Config) hasCacheKey() bool {
	return cfg.CacheKey != ""
}

func (cfg Config) timeoutFor(base time.Duration) time.Duration {
	if cfg.Timeout > 0 && (base <= 0 || cfg.Timeout < base) {
		return cfg.Timeout
	}
	return base
}

func normalizeConfigFormat(raw Format) Format {
	switch strings.ToLower(strings.TrimSpace(string(raw))) {
	case "":
		return ""
	case string(FormatText):
		return FormatText
	case string(FormatJSON):
		return FormatJSON
	default:
		// Preserve unknown values after trimming so validation can reject them.
		return Format(trim(string(raw)))
	}
}

func (seed cacheSeed) inheritedFrom(base cacheSeed) cacheSeed {
	seed.command = seed.command.inheritedFrom(base.command)
	seed.extract = seed.extract.inheritedFrom(base.extract)
	seed.ttl = inheritIfZero(seed.ttl, base.ttl)
	return seed
}

func (seed cacheSeed) apply(cfg Config) Config {
	cfg.Argv = seed.command.Argv
	cfg.Dir = seed.command.Dir
	cfg.Timeout = seed.command.Timeout
	cfg.Format = seed.extract.Format
	cfg.TokenPath = seed.extract.TokenPath
	cfg.TypePath = seed.extract.TypePath
	cfg.ExpiryPath = seed.extract.ExpiryPath
	cfg.ExpiresInPath = seed.extract.ExpiresInPath
	cfg.TTL = seed.ttl
	return cfg
}

func (seed cacheSeed) diff(other cacheSeed) (string, bool) {
	switch {
	case !slices.Equal(seed.command.Argv, other.command.Argv):
		return "argv", false
	case seed.command.Dir != other.command.Dir:
		return "dir", false
	case effectiveFormatValue(seed.extract.Format) != effectiveFormatValue(other.extract.Format):
		return "format", false
	case seed.extract.TokenPath != other.extract.TokenPath:
		return "token_path", false
	case seed.extract.TypePath != other.extract.TypePath:
		return "type_path", false
	case seed.extract.ExpiryPath != other.extract.ExpiryPath:
		return "expiry_path", false
	case seed.extract.ExpiresInPath != other.extract.ExpiresInPath:
		return "expires_in_path", false
	case seed.ttl != other.ttl:
		return "ttl", false
	default:
		return "", true
	}
}

func (cmd commandConfig) inheritedFrom(base commandConfig) commandConfig {
	if len(cmd.Argv) == 0 && len(base.Argv) > 0 {
		cmd.Argv = append([]string(nil), base.Argv...)
	}
	cmd.Dir = inheritIfZero(cmd.Dir, base.Dir)
	cmd.Timeout = inheritIfZero(cmd.Timeout, base.Timeout)
	return cmd
}

func (cfg extractConfig) inheritedFrom(base extractConfig) extractConfig {
	if cfg.Format == "" {
		cfg.Format = base.Format
	}
	cfg.TokenPath = inheritIfZero(cfg.TokenPath, base.TokenPath)
	cfg.TypePath = inheritIfZero(cfg.TypePath, base.TypePath)
	cfg.ExpiryPath = inheritIfZero(cfg.ExpiryPath, base.ExpiryPath)
	cfg.ExpiresInPath = inheritIfZero(cfg.ExpiresInPath, base.ExpiresInPath)
	return cfg
}

func inheritIfZero[T comparable](cur, base T) T {
	var zero T
	if cur != zero {
		return cur
	}
	return base
}

func trim(raw string) string {
	return strings.TrimSpace(raw)
}
