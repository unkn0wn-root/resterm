package ui

import "strings"

var sensitiveHistoryHeaders = map[string]struct{}{
	"api-key":                 {},
	"apikey":                  {},
	"authorization":           {},
	"proxy-authorization":     {},
	"x-access-token":          {},
	"x-amz-security-token":    {},
	"x-api-key":               {},
	"x-apikey":                {},
	"x-auth-email":            {},
	"x-auth-key":              {},
	"x-auth-token":            {},
	"x-aws-access-token":      {},
	"x-aws-secret-access-key": {},
	"x-client-secret":         {},
	"x-csrf-token":            {},
	"x-goog-api-key":          {},
	"x-refresh-token":         {},
	"x-secret-key":            {},
	"x-token":                 {},
	"x-xsrf-token":            {},
}

func shouldMaskHistoryHeader(name string) bool {
	if name == "" {
		return false
	}
	_, ok := sensitiveHistoryHeaders[strings.ToLower(name)]
	return ok
}
