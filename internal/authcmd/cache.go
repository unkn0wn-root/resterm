package authcmd

import (
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
)

func Scope(env, ws string) string {
	return joinCacheParts(env, ws)
}

// Entries keyed by cache_key have two parts; entries keyed by profile name
// have four. The last part changes when the command settings change.
func cacheEntryKey(env string, cfg Config) string {
	env = strings.ToLower(trim(env))
	switch {
	case cfg.hasCacheKey():
		return joinCacheParts(env, cfg.CacheKey)
	case cfg.Profile.named():
		name := strings.ToLower(cfg.Profile.Name)
		return joinCacheParts(env, cfg.Profile.Path, name, cfg.cacheSeed().fingerprint())
	default:
		return ""
	}
}

func (seed cacheSeed) fingerprint() string {
	vals := make([]string, len(seedFields))
	for i, f := range seedFields {
		vals[i] = f.value(seed)
	}
	sum := sha256.Sum256([]byte(joinCacheParts(vals...)))
	return hex.EncodeToString(sum[:])
}

func joinCacheParts(parts ...string) string {
	var b strings.Builder
	for _, part := range parts {
		appendCachePart(&b, part)
	}
	return b.String()
}

func appendCachePart(b *strings.Builder, value string) {
	b.WriteString(strconv.Itoa(len(value)))
	b.WriteByte(':')
	b.WriteString(value)
	b.WriteByte('|')
}

func conflictError(cfg Config, field string) error {
	return diag.Newf(
		diag.ClassAuth,
		"@auth command cache_key %q conflicts with seeded %s",
		cfg.CacheKey,
		field,
	)
}
