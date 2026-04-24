package authcmd

import (
	"strconv"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/errdef"
)

func Scope(env, ws string) string {
	var b strings.Builder
	appendCachePart(&b, env)
	appendCachePart(&b, ws)
	return b.String()
}

func cacheEntryKey(env string, cfg Config) string {
	if !cfg.hasCacheKey() {
		return ""
	}

	var b strings.Builder
	appendCachePart(&b, strings.ToLower(trim(env)))
	appendCachePart(&b, cfg.CacheKey)
	return b.String()
}

func appendCachePart(b *strings.Builder, value string) {
	b.WriteString(strconv.Itoa(len(value)))
	b.WriteByte(':')
	b.WriteString(value)
	b.WriteByte('|')
}

func conflictError(cfg Config, field string) error {
	return errdef.New(
		errdef.CodeAuth,
		"@auth command cache_key %q conflicts with seeded %s",
		cfg.CacheKey,
		field,
	)
}
