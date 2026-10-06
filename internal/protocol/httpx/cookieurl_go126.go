//go:build go1.26

package httpx

import (
	"net/http"
	"net/url"
)

// cookieURL returns the URL http.Client uses for req's cookies. Since Go 1.26
// that URL has the Host header in place of the URL's host.
func cookieURL(req *http.Request) *url.URL {
	if req.Host == "" {
		return req.URL
	}
	u := *req.URL
	u.Host = req.Host
	return &u
}
