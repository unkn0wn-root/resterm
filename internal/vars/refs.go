package vars

import (
	"os"
	"strings"
)

type RefResolver func(raw string) (resolved string, handled bool, found bool)

// EnvRefResolver resolves values prefixed with "env:" by looking up the
// remainder as an OS environment variable. The prefix match is case-insensitive;
// the env-var key is tried as-is first, then uppercased as a fallback.
func EnvRefResolver(raw string) (string, bool, bool) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) < 4 || !strings.EqualFold(trimmed[:4], "env:") {
		return "", false, false
	}
	key := strings.TrimSpace(trimmed[4:])
	if key == "" {
		return "", true, false
	}
	if value, ok := os.LookupEnv(key); ok {
		return value, true, true
	}
	if value, ok := os.LookupEnv(strings.ToUpper(key)); ok {
		return value, true, true
	}
	return "", true, false
}
