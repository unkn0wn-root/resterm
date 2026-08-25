package settings

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/util"
)

var httpSettingKeys = map[string]struct{}{
	"base-url":        {},
	"timeout":         {},
	"proxy":           {},
	"followredirects": {},
	"insecure":        {},
	"no-cookies":      {},
}

// IsHTTPKey reports whether key is a supported HTTP setting key.
func IsHTTPKey(key string) bool {
	k := util.LowerTrim(key)

	if _, ok := httpSettingKeys[k]; ok {
		return true
	}

	return strings.HasPrefix(k, "http-")
}
