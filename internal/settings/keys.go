package settings

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/str"
)

var httpSettingKeys = map[string]struct{}{
	"base-url":                        {},
	"timeout":                         {},
	"proxy":                           {},
	"followredirects":                 {},
	"insecure":                        {},
	"no-cookies":                      {},
	"max-response-size":               {},
	"max-redirects":                   {},
	"forward-credentials-on-redirect": {},
	"sse-max-line-bytes":              {},
	"sse-max-event-bytes":             {},
	"ws-max-message-bytes":            {},
	"ws-compression":                  {},
}

// Keys that set the same option. Merge keeps only the ones from the highest
// scope, and ApplyAll rejects both names when one scope writes both.
var sameOption = map[string]string{
	"insecure":      "http-insecure",
	"http-insecure": "insecure",
	"http-root-ca":  "http-root-cas",
	"http-root-cas": "http-root-ca",
	"grpc-root-ca":  "grpc-root-cas",
	"grpc-root-cas": "grpc-root-ca",
}

// IsHTTPKey reports whether key is a supported HTTP setting key.
func IsHTTPKey(key string) bool {
	k := str.LowerTrim(key)

	if _, ok := httpSettingKeys[k]; ok {
		return true
	}

	return strings.HasPrefix(k, "http-")
}
