package vars

import (
	"os"
	"strings"
)

type RefResolver func(raw string) (resolved string, handled bool, found bool)

// EnvRefResolver resolves values with a case-insensitive "env:" prefix by
// looking up the remainder as an OS environment variable.
func EnvRefResolver(raw string) (string, bool, bool) {
	key, handled := envRefKey(raw)
	if !handled {
		return "", false, false
	}
	if key == "" {
		return "", true, false
	}
	value, ok := lookupEnv(key)
	return value, true, ok
}

func envRefKey(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if len(trimmed) < 4 || !strings.EqualFold(trimmed[:4], "env:") {
		return "", false
	}
	return strings.TrimSpace(trimmed[4:]), true
}

// lookupEnv tries the key as-is first, then uppercased, so lowercase variable
// names can match conventional uppercase OS environment variables.
func lookupEnv(key string) (string, bool) {
	if value, ok := os.LookupEnv(key); ok {
		return value, true
	}
	return os.LookupEnv(strings.ToUpper(key))
}
