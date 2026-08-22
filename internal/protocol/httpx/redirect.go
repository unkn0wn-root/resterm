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

	owner, previous := via[0], via[len(via)-1]
	if origin.Same(owner.URL, req.URL) {
		g.restore(req.Header, owner.Header)
		return nil
	}

	if g.confined {
		return diag.Newf(
			diag.ClassProtocol,
			"refusing to follow a redirect from %s to %s",
			origin.Of(owner.URL),
			origin.Of(req.URL),
		)
	}

	deleteCookies(req.Header)

	if keepsCredentials(previous.URL, req.URL, g.forwardTo) {
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

func keepsCredentials(previous, next *url.URL, forwardTo origin.Set) bool {
	last, dst := origin.Of(previous), origin.Of(next)
	if last.Secure() && !dst.Secure() {
		return false
	}
	return forwardTo.Allows(dst)
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
