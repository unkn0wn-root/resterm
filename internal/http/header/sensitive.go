package header

import "strings"

type Set map[string]struct{}

func NewSet(names ...string) Set {
	s := make(Set, len(names))
	s.Add(names...)
	return s
}

func (s Set) Add(names ...string) {
	for _, name := range names {
		if key := identity(name); key != "" {
			s[key] = struct{}{}
		}
	}
}

func (s Set) Has(name string) bool {
	_, ok := s[identity(name)]
	return ok
}

func identity(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

var sensitive = Set{
	"api-key":                 {},
	"apikey":                  {},
	"authorization":           {},
	"cookie":                  {},
	"cookie2":                 {},
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

var cookies = Set{
	"cookie":  {},
	"cookie2": {},
}

func Sensitive(name string) bool { return sensitive.Has(name) }

func IsCookie(name string) bool { return cookies.Has(name) }
