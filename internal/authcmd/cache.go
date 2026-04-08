package authcmd

import (
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/errdef"
)

func cacheKey(env string, cfg Config) string {
	if !cfg.usesCache() {
		return ""
	}

	var b strings.Builder
	appendCachePart(&b, strings.ToLower(trim(env)))
	appendCachePart(&b, cfg.cacheKey())
	return b.String()
}

func appendCachePart(b *strings.Builder, value string) {
	b.WriteString(strconv.Itoa(len(value)))
	b.WriteByte(':')
	b.WriteString(value)
	b.WriteByte('|')
}

func effectiveExpiry(cred credential, cfg Config) time.Time {
	if !cred.Expiry.IsZero() {
		return cred.Expiry
	}
	if cfg.TTL > 0 && !cred.FetchedAt.IsZero() {
		return cred.FetchedAt.Add(cfg.TTL)
	}
	return time.Time{}
}

func validAt(cred credential, cfg Config, now time.Time) bool {
	expiry := effectiveExpiry(cred, cfg)
	if expiry.IsZero() {
		return true
	}
	if now.Before(expiry) {
		return true
	}
	return false
}

func mergeCfg(base, cur Config) Config {
	merged := cur
	if len(merged.Argv) == 0 && len(base.Argv) > 0 {
		merged.Argv = append([]string(nil), base.Argv...)
	}
	merged.Dir = inheritIfEmpty(merged.Dir, base.Dir)
	if merged.Format == "" {
		merged.Format = base.Format
	}
	merged.Header = inheritIfEmpty(merged.Header, base.Header)
	merged.Scheme = inheritIfEmpty(merged.Scheme, base.Scheme)
	merged.TokenPath = inheritIfEmpty(merged.TokenPath, base.TokenPath)
	merged.TypePath = inheritIfEmpty(merged.TypePath, base.TypePath)
	merged.ExpiryPath = inheritIfEmpty(merged.ExpiryPath, base.ExpiryPath)
	merged.ExpiresInPath = inheritIfEmpty(merged.ExpiresInPath, base.ExpiresInPath)
	merged.TTL = inheritIfZero(merged.TTL, base.TTL)
	merged.Timeout = inheritIfZero(merged.Timeout, base.Timeout)
	return merged
}

func inheritIfEmpty(cur, base string) string {
	if cur != "" {
		return cur
	}
	return base
}

func inheritIfZero(cur, base time.Duration) time.Duration {
	if cur != 0 {
		return cur
	}
	return base
}

func sameSeed(base, cur Config) (string, bool) {
	switch {
	case !slices.Equal(base.Argv, cur.Argv):
		return "argv", false
	case base.Dir != cur.Dir:
		return "dir", false
	case effectiveFormat(base) != effectiveFormat(cur):
		return "format", false
	case base.TokenPath != cur.TokenPath:
		return "token_path", false
	case base.TypePath != cur.TypePath:
		return "type_path", false
	case base.ExpiryPath != cur.ExpiryPath:
		return "expiry_path", false
	case base.ExpiresInPath != cur.ExpiresInPath:
		return "expires_in_path", false
	case base.TTL != cur.TTL:
		return "ttl", false
	default:
		return "", true
	}
}

func conflictError(cfg Config, field string) error {
	return errdef.New(
		errdef.CodeHTTP,
		"@auth command cache_key %q conflicts with seeded %s",
		cfg.CacheKey,
		field,
	)
}
