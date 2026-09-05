// Package recorder forwards HTTP traffic and exports requests and mocks.
package recorder

import (
	"cmp"
	"errors"
	"fmt"
	"math"
	"net"
	"net/url"
	"strings"
	"time"
)

const (
	DefaultListen            = "127.0.0.1:9000"
	DefaultEntries           = 1000
	DefaultBytes       int64 = 64 << 20
	DefaultBodyLimit   int64 = 4 << 20
	DefaultConcurrency       = 32
	ShutdownTimeout          = 3 * time.Second
)

const (
	dialTimeout         = 10 * time.Second
	keepAlive           = 30 * time.Second
	tlsHandshakeTimeout = 10 * time.Second
	readHeaderTimeout   = 5 * time.Second
	idleTimeout         = 60 * time.Second
)

// Metadata is kept in memory and needs a limit separate from the body limit.
const maxMetadataBytes = 1 << 20

type Config struct {
	Listen             string
	Upstream           string
	MaxEntries         int
	MaxBytes           int64
	BodyLimit          int64
	CaptureConcurrency int
	RedactHeaders      []string
	RedactFields       []string
}

func DefaultConfig() Config {
	return Config{
		Listen:             DefaultListen,
		MaxEntries:         DefaultEntries,
		MaxBytes:           DefaultBytes,
		BodyLimit:          DefaultBodyLimit,
		CaptureConcurrency: DefaultConcurrency,
	}
}

// Resolve fills unset fields with defaults and validates the result.
// Callers must reject explicit zero limits before calling if zero is invalid.
func (c Config) Resolve() (Config, error) {
	c.applyDefaults()
	if err := c.checkLimits(); err != nil {
		return c, err
	}

	upstream, err := parseUpstream(c.Upstream)
	if err != nil {
		return c, err
	}
	c.Upstream = upstream

	if _, _, err := net.SplitHostPort(c.Listen); err != nil {
		return c, fmt.Errorf("invalid listen address: %w", err)
	}
	if err := checkRedactionNames(c.RedactHeaders, c.RedactFields); err != nil {
		return c, err
	}
	return c, nil
}

func (c *Config) applyDefaults() {
	c.Listen = cmp.Or(c.Listen, DefaultListen)
	c.MaxEntries = cmp.Or(c.MaxEntries, DefaultEntries)
	c.MaxBytes = cmp.Or(c.MaxBytes, DefaultBytes)
	c.BodyLimit = cmp.Or(c.BodyLimit, DefaultBodyLimit)
	c.CaptureConcurrency = cmp.Or(c.CaptureConcurrency, DefaultConcurrency)
}

// Leave room for sums of limits without integer overflow.
func (c Config) checkLimits() error {
	switch {
	case c.MaxEntries < 1 || c.MaxEntries > math.MaxInt/2:
		return errors.New("max entries must be positive and well below the integer limit")
	case c.CaptureConcurrency < 1:
		return errors.New("capture concurrency must be positive")
	case c.MaxBytes < 1 || c.MaxBytes >= math.MaxInt/4:
		return errors.New("max bytes must be positive and well below the integer limit")
	case c.BodyLimit < 1:
		return errors.New("body limit must be positive")
	case c.BodyLimit > c.MaxBytes:
		return errors.New("body limit must not exceed the byte limit")
	}
	return nil
}

// Do not include the input in errors. It may contain credentials.
func parseUpstream(raw string) (string, error) {
	const invalid = "upstream must be an http(s) origin without credentials, path, query, or fragment"

	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return "", errors.New(invalid)
	}
	switch {
	case u.Scheme != "http" && u.Scheme != "https",
		u.Hostname() == "",
		u.User != nil,
		u.Opaque != "",
		u.Path != "" && u.Path != "/",
		u.RawQuery != "" || u.ForceQuery,
		u.Fragment != "",
		strings.ContainsAny(raw, "\r\n{}"):
		return "", errors.New(invalid)
	}
	return u.Scheme + "://" + u.Host, nil
}

func checkRedactionNames(groups ...[]string) error {
	for _, names := range groups {
		for _, name := range names {
			if strings.TrimSpace(name) == "" || strings.ContainsAny(name, "\r\n") {
				return errors.New("redaction names must be nonempty single-line names")
			}
		}
	}
	return nil
}
