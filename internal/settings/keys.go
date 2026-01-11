package settings

import "strings"

// IsHTTPKey reports whether key is a supported HTTP setting key.
func IsHTTPKey(key string) bool {
	k := strings.ToLower(strings.TrimSpace(key))
	switch k {
	case "timeout", "proxy", "followredirects", "insecure":
		return true
	default:
		return strings.HasPrefix(k, "http-")
	}
}
