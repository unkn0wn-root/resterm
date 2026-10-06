package httpx

import (
	"bytes"
	"io"
	"net/http"

	"github.com/unkn0wn-root/resterm/internal/http/digest"
)

// Digest headers depend on the method and URI, so each redirect needs its own header.
type digestTransport struct {
	base  http.RoundTripper
	creds digest.Credentials
	jar   http.CookieJar
	guard redirectGuard
	// Challenge cookies replace request cookies throughout the redirect chain.
	// Match net/http's redirect behavior (go.dev/issue/17494).
	cookies map[string]bool
}

// Copy the client so callers can inspect its original transport.
// Each send needs fresh cookie tracking.
func withDigest(client *http.Client, opts Options) *http.Client {
	if opts.digest == nil {
		return client
	}
	base := client.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	cp := *client
	cp.Transport = &digestTransport{
		base:    base,
		creds:   *opts.digest,
		jar:     client.Jar,
		guard:   newRedirectGuard(opts),
		cookies: make(map[string]bool),
	}
	return &cp
}

func (t *digestTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if len(t.cookies) > 0 {
		req = t.clone(req)
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized ||
		req.Header.Get(authorizationHeader) != "" || !t.guard.reaches(req) {
		return resp, err
	}
	ch, ok := digest.Pick(resp.Header.Values("WWW-Authenticate"))
	if !ok {
		return resp, nil
	}
	// Close without draining: an endless response body would block the retry.
	_ = resp.Body.Close()

	// The client never sees this 401, so save its cookies here.
	if set := resp.Cookies(); t.jar != nil && len(set) > 0 {
		t.jar.SetCookies(cookieURL(req), set)
		for _, c := range set {
			t.cookies[c.Name] = true
		}
	}
	retry := t.clone(req)

	// The first send closed req.Body; buffered bodies can be replayed with GetBody.
	var body []byte
	if req.GetBody != nil {
		rc, err := req.GetBody()
		if err != nil {
			return nil, err
		}
		body, err = io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			return nil, err
		}
		retry.Body = io.NopCloser(bytes.NewReader(body))
	}
	retry.Header.Set(authorizationHeader, ch.Authorization(t.creds, req.Method, req.URL.RequestURI(), body))
	return t.base.RoundTrip(retry)
}

// Replace request cookies that a challenge updated with the jar's current values.
func (t *digestTransport) clone(req *http.Request) *http.Request {
	out := req.Clone(req.Context())
	if len(t.cookies) == 0 {
		return out
	}
	out.Header.Del("Cookie")
	for _, c := range req.Cookies() {
		if !t.cookies[c.Name] {
			out.AddCookie(c)
		}
	}
	for _, c := range t.jar.Cookies(cookieURL(req)) {
		if t.cookies[c.Name] {
			out.AddCookie(c)
		}
	}
	return out
}
