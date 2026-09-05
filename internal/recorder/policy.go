package recorder

import (
	"strings"
	"unicode"

	"github.com/unkn0wn-root/resterm/internal/http/header"
)

const redacted = "REDACTED"

// Names ignore case and separators: client_secret and clientSecret both match.
var defaultSecretFields = []string{
	"accesskey",
	"accesstoken",
	"apikey",
	"authorization",
	"authorizationcode",
	"clientsecret",
	"code",
	"credential",
	"credentials",
	"idtoken",
	"passwd",
	"password",
	"pwd",
	"refreshtoken",
	"secret",
	"secretkey",
	"session",
	"sessionid",
	"sessiontoken",
	"sig",
	"signature",
	"token",
}

// Drop whole cookies to avoid retaining secrets in their attributes.
var cookieHeaders = header.NewSet("Cookie", "Cookie2", "Set-Cookie", "Set-Cookie2")

// Drop connection headers and values that must be rebuilt on replay.
var hopHeaders = header.NewSet(
	"Connection",
	"Content-Length",
	"Date",
	"Forwarded",
	"Host",
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Proxy-Connection",
	"TE",
	"Trailer",
	"Transfer-Encoding",
	"Upgrade",
	"X-Forwarded-For",
	"X-Forwarded-Host",
	"X-Forwarded-Proto",
)

var locationHeaders = header.NewSet("Location", "Content-Location")

// Body checksums and validators become invalid after decoding or redaction.
var digestHeaders = []string{"ETag", "Content-MD5", "Digest", "Content-Digest", "Repr-Digest"}

type policy struct {
	headers header.Set
	fields  map[string]bool
}

func newPolicy(c Config) policy {
	p := policy{
		headers: header.NewSet(c.RedactHeaders...),
		fields:  make(map[string]bool, len(defaultSecretFields)+len(c.RedactFields)),
	}
	for _, name := range defaultSecretFields {
		p.fields[name] = true
	}
	for _, name := range c.RedactFields {
		p.fields[fieldName(name)] = true
	}
	return p
}

func (p policy) secretField(name string) bool { return p.fields[fieldName(name)] }

func (p policy) secretHeader(name string) bool {
	return header.Sensitive(name) || p.headers.Has(name) ||
		p.secretField(name) || cookieHeaders.Has(name)
}

func fieldName(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || r == '_' || r == '.' {
			return -1
		}
		return unicode.ToLower(r)
	}, name)
}
