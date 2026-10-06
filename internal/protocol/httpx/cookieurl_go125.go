//go:build !go1.26

package httpx

import (
	"net/http"
	"net/url"
)

// cookieURL returns the URL http.Client uses for req's cookies. Before Go 1.26
// the client ignores the Host header there.
func cookieURL(req *http.Request) *url.URL {
	return req.URL
}
