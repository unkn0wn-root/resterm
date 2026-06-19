package parser

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/unkn0wn-root/resterm/internal/openapi/model"
)

const (
	importUserAgent = "resterm-openapi-import"
	importAccept    = "application/json, application/yaml, application/x-yaml, " +
		"text/yaml, text/plain;q=0.9, */*;q=0.8"
	maxSpecBytes = 50 << 20 // 50 MiB
	FetchTimeout = 30 * time.Second
)

var defaultHTTPClient = &http.Client{Timeout: FetchTimeout}

type Option func(*Loader)

// WithHTTPClient sets the client for fetching remote specs and external refs.
// nil keeps the package default.
func WithHTTPClient(c *http.Client) Option {
	return func(l *Loader) {
		l.client = c
	}
}

func (l *Loader) httpClient() *http.Client {
	if l.client != nil {
		return l.client
	}
	return defaultHTTPClient
}

// ParseSpecURL parses src as an http(s) URL. File paths return false, including
// Windows drive paths like C:\spec.yaml (the "C:" reads as a scheme, not http).
func ParseSpecURL(src string) (*url.URL, bool) {
	u, err := url.Parse(strings.TrimSpace(src))
	if err != nil {
		return nil, false
	}
	switch strings.ToLower(u.Scheme) {
	case "http", "https":
		if u.Host == "" {
			return nil, false
		}
		return u, true
	default:
		return nil, false
	}
}

func (l *Loader) fetchRemote(ctx context.Context, u *url.URL) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("User-Agent", importUserAgent)
	req.Header.Set("Accept", importAccept)

	resp, err := l.httpClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch spec: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("fetch spec: unexpected status %s", resp.Status)
	}

	return readRemoteBody(resp.Body, "spec")
}

// remoteHandler fetches external $refs for libopenapi. We supply our own so the
// fetch uses our client and ctx; libopenapi's default getter ignores both.
func (l *Loader) remoteHandler(ctx context.Context) func(string) (*http.Response, error) {
	client := l.httpClient()
	return func(raw string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", importUserAgent)
		req.Header.Set("Accept", importAccept)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			_ = resp.Body.Close()
			return nil, fmt.Errorf("fetch external ref: unexpected status %s", resp.Status)
		}
		body, err := readRemoteBody(resp.Body, "external ref")
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		resp.Body = io.NopCloser(bytes.NewReader(body))
		resp.ContentLength = int64(len(body))
		return resp, nil
	}
}

func readRemoteBody(r io.Reader, subject string) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxSpecBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read %s body: %w", subject, err)
	}
	if int64(len(body)) > maxSpecBytes {
		return nil, fmt.Errorf("%s exceeds maximum size of %d bytes", subject, maxSpecBytes)
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, fmt.Errorf("%s response body was empty", subject)
	}
	return body, nil
}

// baseDirURL strips the file name (and query/fragment) off u. libopenapi resolves
// relative $refs against BaseURL, so it has to point at the directory, not the spec.
func baseDirURL(u *url.URL) *url.URL {
	c := *u
	c.RawQuery = ""
	c.Fragment = ""
	if i := strings.LastIndex(c.Path, "/"); i >= 0 {
		c.Path = c.Path[:i+1]
	} else {
		c.Path = "/"
	}
	return &c
}

// resolveRelativeServers makes relative server URLs absolute against base, so a
// spec with servers like {url: /v2} fetched from a URL yields a usable base URL.
func resolveRelativeServers(spec *model.Spec, base *url.URL) {
	if spec == nil || base == nil {
		return
	}
	fix := func(servers []model.Server) {
		for i := range servers {
			servers[i].URL = absoluteServerURL(base, servers[i].URL)
		}
	}
	fix(spec.Servers)
	for i := range spec.Operations {
		fix(spec.Operations[i].Servers)
	}
}

func absoluteServerURL(base *url.URL, raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return raw
	}
	ref, err := url.Parse(s)
	if err != nil || ref.IsAbs() {
		return raw
	}
	return base.ResolveReference(ref).String()
}
