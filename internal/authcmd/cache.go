package authcmd

import (
	"strconv"
	"strings"
	"time"
)

func cacheKey(env string, cfg Config) string {
	if !cfg.usesCache() {
		return ""
	}

	var b strings.Builder
	appendCachePart(&b, strings.ToLower(trim(env)))
	appendCachePart(&b, cfg.cacheKey())
	appendCachePart(&b, cfg.Dir)
	appendCachePart(&b, string(cfg.Format))
	appendCachePart(&b, cfg.TokenPath)
	appendCachePart(&b, cfg.TypePath)
	appendCachePart(&b, cfg.ExpiryPath)
	appendCachePart(&b, cfg.ExpiresInPath)
	for _, arg := range cfg.Argv {
		appendCachePart(&b, arg)
	}
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

func validAt(cred credential, cfg Config, now time.Time) (valid bool, purge bool) {
	expiry := effectiveExpiry(cred, cfg)
	if expiry.IsZero() {
		return true, false
	}
	if now.Before(expiry) {
		return true, false
	}
	return false, !cred.Expiry.IsZero()
}
