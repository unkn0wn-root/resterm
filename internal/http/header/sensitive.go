package header

import (
	"strings"

	"github.com/unkn0wn-root/resterm/internal/redact"
)

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

var setCookies = Set{
	"set-cookie":  {},
	"set-cookie2": {},
}

func Sensitive(name string) bool { return sensitive.Has(name) }

// RedactLines masks the value of each line of text that names a sensitive header.
// It keeps the whitespace before the value and the line break after it.
func RedactLines(text string) string {
	var b strings.Builder
	done, end := 0, 0
	for line := range strings.Lines(text) {
		start := end
		end += len(line)
		name, value, ok := strings.Cut(line, ":")
		if !ok || !Sensitive(name) {
			continue
		}
		if b.Len() == 0 {
			b.Grow(len(text))
		}
		value = strings.TrimRight(value, "\r\n")
		pad := len(value) - len(strings.TrimLeft(value, " \t"))
		at := start + len(name) + 1
		b.WriteString(text[done : at+pad])
		b.WriteString(redact.Mask)
		done = at + len(value)
	}
	if b.Len() == 0 {
		return text
	}
	b.WriteString(text[done:])
	return b.String()
}

func IsCookie(name string) bool { return cookies.Has(name) }

// CarriesCookie reports whether the named header carries cookies in a request
// or a response. Unlike IsCookie, which matches only the request headers that
// redirects strip, it also matches Set-Cookie and Set-Cookie2.
func CarriesCookie(name string) bool { return cookies.Has(name) || setCookies.Has(name) }
