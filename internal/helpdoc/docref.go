package helpdoc

import (
	"net/url"
	"path"
	"strings"
	"unicode"
)

const docsHost = "resterm.app"

// DocRef points at a page on the docs site. Page is a slug from site/src/docs/nav.ts.
// Important to remember that released binaries keep these links, so renaming
// a page needs a redirect on the site.
type DocRef struct {
	Page    string
	Heading string
}

func Manual() DocRef {
	return DocRef{}
}

func (d DocRef) URL() string {
	u := url.URL{
		Scheme:   "https",
		Host:     docsHost,
		Path:     path.Join("/docs", d.Page) + "/",
		Fragment: d.anchor(),
	}
	return u.String()
}

func (d DocRef) anchor() string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(d.Heading)) {
		switch {
		case unicode.IsLetter(r), unicode.IsNumber(r), r == '-', r == '_':
			b.WriteRune(r)
		case unicode.IsSpace(r):
			b.WriteByte('-')
		}
	}
	return strings.Trim(b.String(), "-")
}
