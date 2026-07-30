package vars

import (
	"os"
	"strings"
)

type RefResolver func(raw string) (resolved string, handled bool, found bool)

// EnvRefResolver resolves values with a case-insensitive "env:" prefix by
// looking up the remainder as an OS environment variable.
func EnvRefResolver(raw string) (string, bool, bool) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) < 4 || !strings.EqualFold(trimmed[:4], "env:") {
		return "", false, false
	}
	key := strings.TrimSpace(trimmed[4:])
	if key == "" {
		return "", true, false
	}
	value, ok := lookupEnv(key)
	return value, true, ok
}

// lookupEnv tries the key as-is first, then uppercased, so lowercase variable
// names can match conventional uppercase OS environment variables.
func lookupEnv(key string) (string, bool) {
	if value, ok := os.LookupEnv(key); ok {
		return value, true
	}
	return os.LookupEnv(strings.ToUpper(key))
}
