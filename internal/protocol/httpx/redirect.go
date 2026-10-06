package httpx

import (
	"net/http"
	"net/url"
	"slices"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/http/header"
	"github.com/unkn0wn-root/resterm/internal/http/origin"
)

const DefaultMaxRedirects = 10

const refererHeader = "Referer"

// net/http can forward custom credential headers to another origin. The guard
// removes them unless the user allows the target origin.
type redirectGuard struct {
	limit     int
	confined  bool
	creds     header.Set
	forwardTo origin.Set
}

func newRedirectGuard(opts Options) redirectGuard {
	return redirectGuard{
		limit:     redirectLimit(opts),
		confined:  opts.ConfineToOrigin,
		creds:     header.NewSet(opts.CredentialHeaders...),
		forwardTo: opts.ForwardCredentials,
	}
}

func redirectPolicy(opts Options) func(*http.Request, []*http.Request) error {
	return newRedirectGuard(opts).check
}

func (g redirectGuard) check(req *http.Request, via []*http.Request) error {
	if g.limit <= 0 {
		return http.ErrUseLastResponse
	}
	if len(via) > g.limit {
		return diag.Newf(
			diag.ClassProtocol,
			"stopped after following %d redirects",
			g.limit,
		)
	}

	// Credentials, cookies and confinement belong to the origin that owns the
	// request. The Referer belongs to the single hop being taken, so it is
	// compared against the previous URL instead.
	owner, previous := via[0], via[len(via)-1]
	// net/http keeps a Host header through a redirect that is not a full URL,
	// even one that moves to another server. Only the server the request was
	// sent to gets it.
	if !origin.Same(owner.URL, req.URL) {
		req.Host = ""
	}
	if !sameVirtualHost(owner, req) {
		if g.confined {
			from, to := origin.Of(owner.URL), origin.Of(req.URL)
			if from == to {
				from, to = authority(owner), authority(req)
			}
			return diag.Newf(diag.ClassProtocol, "refusing to follow a redirect from %s to %s", from, to)
		}
		deleteCookies(req.Header)
	}

	if !sameVirtualHost(previous, req) {
		narrowReferer(req.Header, owner.Header, previous.URL)
	}

	if g.keepsCredentials(via, req) {
		g.restore(req.Header, owner.Header)
		return nil
	}
	g.strip(req.Header)
	return nil
}

func (g redirectGuard) credential(name string) bool {
	return g.creds.Has(name) || header.Sensitive(name)
}

func (g redirectGuard) strip(h http.Header) {
	for name := range h {
		if g.credential(name) {
			// Header.Del can change the name, so delete this key directly.
			delete(h, name)
		}
	}
}

func (g redirectGuard) restore(dst, initial http.Header) {
	for name, values := range initial {
		if _, present := dst[name]; present || header.IsCookie(name) {
			continue
		}
		if g.credential(name) {
			dst[name] = slices.Clone(values)
		}
	}
}

func (g redirectGuard) keepsCredentials(via []*http.Request, next *http.Request) bool {
	if leftTLS(via, next.URL) {
		return false
	}
	return sameVirtualHost(via[0], next) || g.forwardTo.Allows(origin.Of(next.URL))
}

// authority is the origin a request is sent to. A Host header replaces the
// URL's host, because two virtual hosts on one address are separate
// applications.
func authority(req *http.Request) origin.Origin {
	if req.Host == "" {
		return origin.Of(req.URL)
	}
	u := *req.URL
	u.Host = req.Host
	return origin.Of(&u)
}

// sameVirtualHost reports whether b goes to the same server as a and sends the
// same Host. Two virtual hosts on one address are separate applications.
func sameVirtualHost(a, b *http.Request) bool {
	x := authority(a)
	return origin.Same(a.URL, b.URL) && x.Valid() && x == authority(b)
}

// reaches applies the credential rules to this redirect.
// net/http links earlier requests through Response.Request.
func (g redirectGuard) reaches(hop *http.Request) bool {
	var via []*http.Request
	for r := hop.Response; r != nil && r.Request != nil; r = r.Request.Response {
		via = append(via, r.Request)
	}
	if len(via) == 0 {
		return true
	}
	slices.Reverse(via)
	return g.keepsCredentials(via, hop)
}

// leftTLS reports whether the chain has gone from an https hop to a plain http
// one. Trust does not come back if a later hop returns to https, because
// whoever sat on the plain http hop picked every redirect after it.
func leftTLS(via []*http.Request, next *url.URL) bool {
	secure := false
	for _, hop := range via {
		o := origin.Of(hop.URL)
		if secure && !o.Secure() {
			return true
		}
		secure = secure || o.Secure()
	}
	return secure && !origin.Of(next).Secure()
}

// narrowReferer cuts the Referer down to an origin, the way a browser does
// across a site boundary. net/http fills it in from the previous URL before this
// policy runs, and that URL can carry a credential in its query. A Referer the
// request set itself is left alone, and a hop without one does not gain one.
func narrowReferer(dst, initial http.Header, previous *url.URL) {
	if dst.Get(refererHeader) == "" || initial.Get(refererHeader) != "" {
		return
	}
	if o := origin.Of(previous); o.Valid() {
		dst.Set(refererHeader, o.String()+"/")
		return
	}
	dst.Del(refererHeader)
}

func redirectLimit(opts Options) int {
	if !opts.FollowRedirects {
		return 0
	}
	return opts.MaxRedirects.Or(DefaultMaxRedirects)
}

func deleteCookies(h http.Header) {
	for name := range h {
		if header.IsCookie(name) {
			delete(h, name)
		}
	}
}
