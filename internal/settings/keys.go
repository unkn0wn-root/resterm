package settings

import "strings"

var httpSettingKeys = map[string]struct{}{
	"timeout":         {},
	"proxy":           {},
	"followredirects": {},
	"insecure":        {},
	"no-cookies":      {},
}

// IsHTTPKey reports whether key is a supported HTTP setting key.
func IsHTTPKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))

	if _, ok := httpSettingKeys[k]; ok {
		return true
	}

	return strings.HasPrefix(k, "http-")
}
