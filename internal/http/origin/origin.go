package origin

import (
	"fmt"
	"net"
	"net/url"
	"slices"
	"strings"
)

type Origin struct {
	scheme string
	host   string
	port   string
}

var httpSchemes = map[string]string{
	"http":  "http",
	"https": "https",
	"ws":    "http",
	"wss":   "https",
}

var defaultPorts = map[string]string{
	"http":  "80",
	"https": "443",
}

func Of(u *url.URL) Origin {
	if u == nil {
		return Origin{}
	}

	scheme, dialed := httpSchemes[strings.ToLower(u.Scheme)]
	host := strings.ToLower(u.Hostname())
	if !dialed || host == "" {
		return Origin{}
	}
	port := u.Port()
	if port == "" {
		port = defaultPorts[scheme]
	}
	return Origin{scheme: scheme, host: host, port: port}
}

func Parse(raw string) (Origin, error) {
	text := strings.TrimSpace(raw)
	u, err := url.Parse(text)
	if err != nil {
		return Origin{}, fmt.Errorf("%q is not an origin: %w", raw, err)
	}

	switch {
	case u.User != nil:
		return Origin{}, fmt.Errorf("%q is not an origin: it carries user information", raw)
	case u.Path != "" && u.Path != "/", u.RawQuery != "", u.Fragment != "":
		return Origin{}, fmt.Errorf("%q is not an origin: it names more than a host", raw)
	}

	o := Of(u)
	if !o.Valid() {
		return Origin{}, fmt.Errorf(
			"%q is not an origin: expected a scheme and host such as https://cdn.example.com",
			raw,
		)
	}
	return o, nil
}

func (o Origin) Valid() bool { return o.scheme != "" }

func (o Origin) Secure() bool { return o.scheme == "https" }

func (o Origin) String() string {
	if !o.Valid() {
		return ""
	}
	u := url.URL{Scheme: o.scheme, Host: hostLiteral(o.host)}
	if o.port != defaultPorts[o.scheme] {
		u.Host = net.JoinHostPort(o.host, o.port)
	}
	// The host is stored unescaped, so the zone id of a link-local IPv6
	// address needs its percent sign back before the text is a URL again.
	return u.String()
}

func hostLiteral(host string) string {
	if strings.ContainsRune(host, ':') {
		return "[" + host + "]"
	}
	return host
}

func Same(a, b *url.URL) bool {
	x := Of(a)
	return x.Valid() && x == Of(b)
}

type Set struct {
	all     bool
	origins []Origin
}

func Any() Set { return Set{all: true} }

func NewSet(origins ...Origin) Set {
	kept := make([]Origin, 0, len(origins))
	for _, o := range origins {
		if o.Valid() && !slices.Contains(kept, o) {
			kept = append(kept, o)
		}
	}
	if len(kept) == 0 {
		return Set{}
	}
	return Set{origins: kept}
}

func ParseSet(raw string) (Set, error) {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})

	origins := make([]Origin, 0, len(fields))
	for _, field := range fields {
		o, err := Parse(field)
		if err != nil {
			return Set{}, err
		}
		origins = append(origins, o)
	}
	return NewSet(origins...), nil
}

func (s Set) Empty() bool { return !s.all && len(s.origins) == 0 }

func (s Set) Allows(o Origin) bool {
	if !o.Valid() {
		return false
	}
	return s.all || slices.Contains(s.origins, o)
}

func (s Set) String() string {
	switch {
	case s.all:
		return "any origin"
	case len(s.origins) == 0:
		return "none"
	}
	names := make([]string, len(s.origins))
	for i, o := range s.origins {
		names[i] = o.String()
	}
	return strings.Join(names, ", ")
}
