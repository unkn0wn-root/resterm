package httpx

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/http/origin"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

type recorder struct {
	*httptest.Server
	got chan http.Header
}

func newRecorder(t *testing.T) *recorder {
	t.Helper()
	r := &recorder{got: make(chan http.Header, 4)}
	r.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		r.got <- req.Header.Clone()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(r.Close)
	return r
}

func (r *recorder) received(t *testing.T) http.Header {
	t.Helper()
	select {
	case h := <-r.got:
		return h
	case <-time.After(5 * time.Second):
		t.Fatal("the redirect target was never reached")
		return nil
	}
}

func redirectTo(t *testing.T, target string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, target, http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func otherHost(rawURL string) string {
	return strings.Replace(rawURL, "127.0.0.1", "localhost", 1)
}

func execute(t *testing.T, req *restfile.Request, opts Options) *Response {
	t.Helper()
	opts.FollowRedirects = true
	if opts.Timeout == 0 {
		opts.Timeout = 10 * time.Second
	}
	resp, err := NewClient(nil).Execute(t.Context(), req, nil, opts)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	return resp
}

func TestRedirectToAnotherHostDropsCredentials(t *testing.T) {
	tests := []struct {
		name   string
		header string
		value  string
	}{
		{name: "manual authorization", header: "Authorization", value: "Bearer secret"},
		{name: "manual cookie", header: "Cookie", value: "session=secret"},
		{name: "proxy authorization", header: "Proxy-Authorization", value: "Basic secret"},
		{name: "api key auth", header: "X-API-Key", value: "secret"},
		{name: "oauth custom header", header: "X-Auth-Token", value: "secret"},
		{name: "command auth header", header: "X-Access-Token", value: "secret"},
		{name: "google api key", header: "X-Goog-Api-Key", value: "secret"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attacker := newRecorder(t)
			origin := redirectTo(t, otherHost(attacker.URL)+"/steal")

			hdr := make(http.Header)
			hdr.Set(tt.header, tt.value)
			hdr.Set("X-Trace-Id", "kept")
			execute(t, &restfile.Request{Method: "GET", URL: origin.URL, Headers: hdr}, Options{})

			got := attacker.received(t)
			if leaked := got.Get(tt.header); leaked != "" {
				t.Errorf("%s reached the redirect target as %q", tt.header, leaked)
			}
			if got.Get("X-Trace-Id") != "kept" {
				t.Error("a header that carries no credential was dropped")
			}
		})
	}
}

func TestRedirectDropsAPIKeyPlacedByAuthDirective(t *testing.T) {
	attacker := newRecorder(t)
	origin := redirectTo(t, otherHost(attacker.URL)+"/steal")

	req := &restfile.Request{
		Method: "GET",
		URL:    origin.URL,
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type: restfile.AuthAPIKey,
				Params: map[string]string{
					authParamName:      "X-API-Key",
					authParamValue:     "secret",
					authParamPlacement: authPlacementHeader,
				},
			},
		},
	}
	execute(t, req, Options{})

	if leaked := attacker.received(t).Get("X-API-Key"); leaked != "" {
		t.Errorf("X-API-Key reached the redirect target as %q", leaked)
	}
}

func TestRedirectToAnotherPortDropsCredentials(t *testing.T) {
	attacker := newRecorder(t)
	origin := redirectTo(t, attacker.URL+"/steal")

	hdr := make(http.Header)
	hdr.Set("Authorization", "Bearer secret")
	hdr.Set("X-API-Key", "secret")
	execute(t, &restfile.Request{Method: "GET", URL: origin.URL, Headers: hdr}, Options{})

	got := attacker.received(t)
	for _, name := range []string{"Authorization", "X-API-Key"} {
		if leaked := got.Get(name); leaked != "" {
			t.Errorf("%s crossed a port boundary as %q", name, leaked)
		}
	}
}

func TestRedirectWithinTheSameOriginKeepsCredentials(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/moved" {
			got = req.Header.Clone()
			return
		}
		http.Redirect(w, req, "/moved", http.StatusFound)
	}))
	defer srv.Close()

	hdr := make(http.Header)
	hdr.Set("Authorization", "Bearer secret")
	hdr.Set("X-API-Key", "secret")
	execute(t, &restfile.Request{Method: "GET", URL: srv.URL, Headers: hdr}, Options{})

	if got.Get("Authorization") != "Bearer secret" || got.Get("X-API-Key") != "secret" {
		t.Fatalf("a same origin redirect lost its credentials: %v", got)
	}
}

func TestKeepsCredentials(t *testing.T) {
	tests := []struct {
		name     string
		chain    []string
		next     string
		forwards string
		want     bool
	}{
		{
			name:  "back to the origin that owns them",
			chain: []string{"https://a.example.com/x"},
			next:  "https://a.example.com/y",
			want:  true,
		},
		{
			name:  "no forwarding",
			chain: []string{"https://a.example.com/x"},
			next:  "https://cdn.example.net/y",
		},
		{
			name:     "named origin",
			chain:    []string{"https://a.example.com/x"},
			next:     "https://cdn.example.net/y",
			forwards: "https://cdn.example.net",
			want:     true,
		},
		{
			name:     "an origin outside the list stays outside",
			chain:    []string{"https://a.example.com/x"},
			next:     "https://evil.example.net/y",
			forwards: "https://cdn.example.net",
		},
		{
			name:     "a listed origin on another port is another origin",
			chain:    []string{"https://a.example.com/x"},
			next:     "https://cdn.example.net:8443/y",
			forwards: "https://cdn.example.net",
		},
		{
			name:     "any origin",
			chain:    []string{"https://a.example.com/x"},
			next:     "https://anywhere.example.net/y",
			forwards: "true",
			want:     true,
		},
		{
			name:     "any origin still refuses a downgrade",
			chain:    []string{"https://a.example.com/x"},
			next:     "http://a.example.com/y",
			forwards: "true",
		},
		{
			name:     "a listed origin still refuses a downgrade",
			chain:    []string{"https://a.example.com/x"},
			next:     "http://cdn.example.net/y",
			forwards: "http://cdn.example.net",
		},
		{
			name:     "a downgrade partway along a chain",
			chain:    []string{"https://a.example.com/x", "https://trusted.example.net/x"},
			next:     "http://other.example.net/y",
			forwards: "true",
		},
		{
			name: "a downgrade earlier in the chain is not forgotten",
			chain: []string{
				"https://a.example.com/x",
				"http://other.example.net/one",
			},
			next:     "http://other.example.net/two",
			forwards: "true",
		},
		{
			name: "climbing back onto https does not restore trust",
			chain: []string{
				"https://a.example.com/x",
				"http://other.example.net/one",
			},
			next:     "https://a.example.com/y",
			forwards: "true",
		},
		{
			name:     "plain http to plain http is not a downgrade",
			chain:    []string{"http://a.example.com/x"},
			next:     "http://cdn.example.net/y",
			forwards: "http://cdn.example.net",
			want:     true,
		},
		{
			name: "a chain that never had TLS keeps forwarding",
			chain: []string{
				"http://a.example.com/x",
				"http://other.example.net/one",
			},
			next:     "http://other.example.net/two",
			forwards: "true",
			want:     true,
		},
		{
			name:     "an upgrade to https is allowed when the origin is listed",
			chain:    []string{"http://a.example.com/x"},
			next:     "https://cdn.example.net/y",
			forwards: "https://cdn.example.net",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, err := url.Parse(tt.next)
			if err != nil {
				t.Fatal(err)
			}
			forwards := origin.Set{}
			if tt.forwards != "" {
				if forwards, err = ParseForwardCredentials(tt.forwards); err != nil {
					t.Fatalf("ParseForwardCredentials(%q): %v", tt.forwards, err)
				}
			}
			g := redirectGuard{forwardTo: forwards}
			if got := g.keepsCredentials(chain(t, tt.chain...), next); got != tt.want {
				t.Fatalf(
					"keepsCredentials(%v, %s) = %t, want %t",
					tt.chain,
					tt.next,
					got,
					tt.want,
				)
			}
		})
	}
}

func chain(t *testing.T, urls ...string) []*http.Request {
	t.Helper()
	out := make([]*http.Request, len(urls))
	for i, raw := range urls {
		req, err := http.NewRequest(http.MethodGet, raw, nil)
		if err != nil {
			t.Fatal(err)
		}
		out[i] = req
	}
	return out
}

func TestRedirectLoopStops(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		http.Redirect(w, req, "/next", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewClient(nil).Execute(
		t.Context(),
		&restfile.Request{Method: "GET", URL: srv.URL},
		nil,
		Options{FollowRedirects: true, Timeout: 10 * time.Second},
	)
	if err == nil {
		t.Fatal("Execute() = nil, want the redirect limit")
	}
	if !strings.Contains(err.Error(), "stopped after") {
		t.Fatalf("Execute() = %v, want the redirect limit", err)
	}
}

func TestRedirectsDisabledReturnTheRedirectItself(t *testing.T) {
	attacker := newRecorder(t)
	origin := redirectTo(t, otherHost(attacker.URL)+"/steal")

	resp, err := NewClient(nil).Execute(
		context.Background(),
		&restfile.Request{Method: "GET", URL: origin.URL},
		nil,
		Options{Timeout: 10 * time.Second},
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusFound)
	}
}

func TestRedirectKeepsCookiesTheJarOwns(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		switch req.URL.Path {
		case "/moved":
			got = req.Header.Clone()
		default:
			http.SetCookie(w, &http.Cookie{Name: "jar", Value: "kept", Path: "/"})
			http.Redirect(w, req, "/moved", http.StatusFound)
		}
	}))
	defer srv.Close()

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	execute(t, &restfile.Request{Method: "GET", URL: srv.URL}, Options{CookieJar: jar})

	if v := got.Get("Cookie"); !strings.Contains(v, "jar=kept") {
		t.Fatalf("Cookie = %q, want the jar cookie", v)
	}
}

func TestRedirectLimit(t *testing.T) {
	tests := []struct {
		name string
		opts Options
		want int
	}{
		{name: "not following", want: 0},
		{name: "unset takes the default", opts: Options{FollowRedirects: true}, want: DefaultMaxRedirects},
		{
			name: "explicit count",
			opts: Options{FollowRedirects: true, MaxRedirects: restfile.OptOf(3)},
			want: 3,
		},
		{
			name: "zero follows none",
			opts: Options{FollowRedirects: true, MaxRedirects: restfile.OptOf(0)},
			want: 0,
		},
		{
			name: "not following wins over a count",
			opts: Options{MaxRedirects: restfile.OptOf(5)},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := redirectLimit(tt.opts); got != tt.want {
				t.Fatalf("redirectLimit() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestRedirectStopsAtTheConfiguredCount(t *testing.T) {
	var hops atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		hops.Add(1)
		http.Redirect(w, req, "/next", http.StatusFound)
	}))
	defer srv.Close()

	_, err := NewClient(nil).Execute(
		t.Context(),
		&restfile.Request{Method: "GET", URL: srv.URL},
		nil,
		Options{FollowRedirects: true, MaxRedirects: restfile.OptOf(3), Timeout: 10 * time.Second},
	)
	if err == nil || !strings.Contains(err.Error(), "stopped after following 3 redirects") {
		t.Fatalf("Execute() = %v, want the configured redirect limit", err)
	}
	if got := hops.Load(); got != 4 {
		t.Fatalf("the server saw %d requests, want 4", got)
	}
}

func TestMaxRedirectsZeroFollowsNone(t *testing.T) {
	attacker := newRecorder(t)
	origin := redirectTo(t, otherHost(attacker.URL)+"/steal")

	resp, err := NewClient(nil).Execute(
		t.Context(),
		&restfile.Request{Method: "GET", URL: origin.URL},
		nil,
		Options{FollowRedirects: true, MaxRedirects: restfile.OptOf(0), Timeout: 10 * time.Second},
	)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("StatusCode = %d, want %d", resp.StatusCode, http.StatusFound)
	}
}

func TestMaxRedirectsSetting(t *testing.T) {
	tests := []struct {
		raw     string
		want    int
		wantErr bool
	}{
		{raw: "20", want: 20},
		{raw: "0", want: 0},
		{raw: "none", want: 0},
		{raw: "Off", want: 0},
		{raw: "-1", wantErr: true},
		{raw: "many", wantErr: true},
		{raw: "unlimited", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			var opts Options
			err := ApplyOptionSettings(&opts, map[string]string{"max-redirects": tt.raw})
			if (err != nil) != tt.wantErr {
				t.Fatalf("ApplyOptionSettings(%q) error = %v, wantErr %t", tt.raw, err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if got, ok := opts.MaxRedirects.Get(); !ok || got != tt.want {
				t.Fatalf("MaxRedirects = %d (set %t), want %d", got, ok, tt.want)
			}
		})
	}
}

func TestRedirectDropsHeadersAuthNamed(t *testing.T) {
	tests := []struct {
		name   string
		header string
		req    func(url, header string) *restfile.Request
		opts   Options
	}{
		{
			name:   "apikey auth with a custom name",
			header: "X-Registry-Token",
			req: func(url, hdr string) *restfile.Request {
				return &restfile.Request{
					Method: "GET",
					URL:    url,
					Metadata: restfile.RequestMetadata{Auth: &restfile.AuthSpec{
						Type: restfile.AuthAPIKey,
						Params: map[string]string{
							authParamName:      hdr,
							authParamValue:     "secret",
							authParamPlacement: authPlacementHeader,
						},
					}},
				}
			},
		},
		{
			name:   "header auth with a custom name",
			header: "X-Tenant-Credential",
			req: func(url, hdr string) *restfile.Request {
				return &restfile.Request{
					Method: "GET",
					URL:    url,
					Metadata: restfile.RequestMetadata{Auth: &restfile.AuthSpec{
						Type: restfile.AuthHeader,
						Params: map[string]string{
							authParamHeader: hdr,
							authParamValue:  "secret",
						},
					}},
				}
			},
		},
		{
			name:   "header the caller reports as a credential",
			header: "X-Registry-Token",
			req: func(url, hdr string) *restfile.Request {
				h := make(http.Header)
				h.Set(hdr, "secret")
				return &restfile.Request{Method: "GET", URL: url, Headers: h}
			},
			opts: Options{CredentialHeaders: []string{"x-registry-token"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attacker := newRecorder(t)
			origin := redirectTo(t, otherHost(attacker.URL)+"/steal")

			execute(t, tt.req(origin.URL, tt.header), tt.opts)

			if leaked := attacker.received(t).Get(tt.header); leaked != "" {
				t.Errorf("%s reached the redirect target as %q", tt.header, leaked)
			}
		})
	}
}

func TestRedirectKeepsAuthNamedHeadersWithinTheOrigin(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/moved" {
			got = req.Header.Clone()
			return
		}
		http.Redirect(w, req, "/moved", http.StatusFound)
	}))
	defer srv.Close()

	hdr := make(http.Header)
	hdr.Set("X-Registry-Token", "secret")
	execute(
		t,
		&restfile.Request{Method: "GET", URL: srv.URL, Headers: hdr},
		Options{CredentialHeaders: []string{"X-Registry-Token"}},
	)

	if got.Get("X-Registry-Token") != "secret" {
		t.Fatalf("a same origin redirect lost its credential: %v", got)
	}
}

func TestBuildHTTPRequestReportsAuthPlacedHeaders(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://api.example.com/v1",
		Metadata: restfile.RequestMetadata{Auth: &restfile.AuthSpec{
			Type: restfile.AuthAPIKey,
			Params: map[string]string{
				authParamName:      "X-Registry-Token",
				authParamValue:     "secret",
				authParamPlacement: authPlacementHeader,
			},
		}},
	}

	_, opts, _, err := NewClient(nil).BuildHTTPRequest(t.Context(), req, nil, Options{})
	if err != nil {
		t.Fatalf("BuildHTTPRequest: %v", err)
	}
	if !slices.Contains(opts.CredentialHeaders, "X-Registry-Token") {
		t.Fatalf("CredentialHeaders = %v, want the auth placed name", opts.CredentialHeaders)
	}
}

func TestBuildHTTPRequestIgnoresQueryPlacedAuth(t *testing.T) {
	req := &restfile.Request{
		Method: "GET",
		URL:    "https://api.example.com/v1",
		Metadata: restfile.RequestMetadata{Auth: &restfile.AuthSpec{
			Type: restfile.AuthAPIKey,
			Params: map[string]string{
				authParamName:      "api_key",
				authParamValue:     "secret",
				authParamPlacement: authPlacementQuery,
			},
		}},
	}

	_, opts, _, err := NewClient(nil).BuildHTTPRequest(t.Context(), req, nil, Options{})
	if err != nil {
		t.Fatalf("BuildHTTPRequest: %v", err)
	}
	if len(opts.CredentialHeaders) != 0 {
		t.Fatalf("CredentialHeaders = %v, want none", opts.CredentialHeaders)
	}
}

func TestRedirectDropsAnAuthHeaderTheRequestSuppliedItself(t *testing.T) {
	tests := []struct {
		name string
		auth *restfile.AuthSpec
	}{
		{
			name: "apikey auth",
			auth: &restfile.AuthSpec{
				Type: restfile.AuthAPIKey,
				Params: map[string]string{
					authParamName:      "X-Registry-Token",
					authParamValue:     "from-directive",
					authParamPlacement: authPlacementHeader,
				},
			},
		},
		{
			name: "header auth",
			auth: &restfile.AuthSpec{
				Type: restfile.AuthHeader,
				Params: map[string]string{
					authParamHeader: "X-Registry-Token",
					authParamValue:  "from-directive",
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attacker := newRecorder(t)
			origin := redirectTo(t, otherHost(attacker.URL)+"/steal")

			hdr := make(http.Header)
			hdr.Set("X-Registry-Token", "written-by-hand")
			execute(t, &restfile.Request{
				Method:   "GET",
				URL:      origin.URL,
				Headers:  hdr,
				Metadata: restfile.RequestMetadata{Auth: tt.auth},
			}, Options{})

			if leaked := attacker.received(t).Get("X-Registry-Token"); leaked != "" {
				t.Errorf("X-Registry-Token reached the redirect target as %q", leaked)
			}
		})
	}
}

func TestResolveAuthReportsTheHeaderItTargets(t *testing.T) {
	tests := []struct {
		name     string
		auth     *restfile.AuthSpec
		existing string
		want     []string
		wantSet  bool
	}{
		{
			name: "apikey places its value",
			auth: &restfile.AuthSpec{Type: restfile.AuthAPIKey, Params: map[string]string{
				authParamName:      "X-Registry-Token",
				authParamValue:     "secret",
				authParamPlacement: authPlacementHeader,
			}},
			want:    []string{"X-Registry-Token"},
			wantSet: true,
		},
		{
			name: "apikey defers to the request",
			auth: &restfile.AuthSpec{Type: restfile.AuthAPIKey, Params: map[string]string{
				authParamName:      "X-Registry-Token",
				authParamValue:     "secret",
				authParamPlacement: authPlacementHeader,
			}},
			existing: "X-Registry-Token",
			want:     []string{"X-Registry-Token"},
		},
		{
			name: "header auth defers to the request",
			auth: &restfile.AuthSpec{Type: restfile.AuthHeader, Params: map[string]string{
				authParamHeader: "X-Tenant-Credential",
				authParamValue:  "secret",
			}},
			existing: "X-Tenant-Credential",
			want:     []string{"X-Tenant-Credential"},
		},
		{
			name: "bearer defers to the request",
			auth: &restfile.AuthSpec{Type: restfile.AuthBearer, Params: map[string]string{
				authParamToken: "secret",
			}},
			existing: "Authorization",
			want:     []string{"Authorization"},
		},
		{
			name: "apikey in the query targets no header",
			auth: &restfile.AuthSpec{Type: restfile.AuthAPIKey, Params: map[string]string{
				authParamName:      "api_key",
				authParamValue:     "secret",
				authParamPlacement: authPlacementQuery,
			}},
			wantSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existing := make(http.Header)
			if tt.existing != "" {
				existing.Set(tt.existing, "written-by-hand")
			}

			plan, err := ResolveAuth(tt.auth, nil, existing, diag.ComponentHTTP)
			if err != nil {
				t.Fatalf("ResolveAuth: %v", err)
			}
			if !slices.Equal(plan.Targets, tt.want) {
				t.Fatalf("Headers = %v, want %v", plan.Targets, tt.want)
			}
			if got := len(plan.Values) > 0; got != tt.wantSet {
				t.Fatalf("placed a value = %t, want %t", got, tt.wantSet)
			}
		})
	}
}

func TestForwardCredentialsAllowsTheNamedOrigin(t *testing.T) {
	attacker := newRecorder(t)
	trusted := newRecorder(t)

	for _, tt := range []struct {
		name   string
		target *recorder
		want   string
	}{
		{name: "named origin", target: trusted, want: "Bearer secret"},
		{name: "any other origin", target: attacker},
	} {
		t.Run(tt.name, func(t *testing.T) {
			origin := redirectTo(t, otherHost(tt.target.URL)+"/next")
			forward, err := ParseForwardCredentials(otherHost(trusted.URL))
			if err != nil {
				t.Fatalf("ParseForwardCredentials: %v", err)
			}

			hdr := make(http.Header)
			hdr.Set("Authorization", "Bearer secret")
			execute(
				t,
				&restfile.Request{Method: "GET", URL: origin.URL, Headers: hdr},
				Options{ForwardCredentials: forward},
			)

			if got := tt.target.received(t).Get("Authorization"); got != tt.want {
				t.Fatalf("Authorization = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestForwardCredentialsSetting(t *testing.T) {
	tests := []struct {
		raw     string
		want    string
		wantErr bool
	}{
		{raw: "true", want: "any origin"},
		{raw: "ALL", want: "any origin"},
		{raw: "false", want: "none"},
		{raw: "off", want: "none"},
		{raw: "https://cdn.example.com", want: "https://cdn.example.com"},
		{
			raw:  "https://a.example.com,https://b.example.com",
			want: "https://a.example.com, https://b.example.com",
		},
		{raw: "cdn.example.com", wantErr: true},
		{raw: "https://cdn.example.com/assets", wantErr: true},
		{raw: "", wantErr: true},
		{raw: "   ", wantErr: true},
		{raw: " , ; ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			var opts Options
			err := ApplyOptionSettings(&opts, map[string]string{
				"forward-credentials-on-redirect": tt.raw,
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("ApplyOptionSettings(%q) error = %v, wantErr %t", tt.raw, err, tt.wantErr)
			}
			if err == nil && opts.ForwardCredentials.String() != tt.want {
				t.Fatalf("ForwardCredentials = %q, want %q", opts.ForwardCredentials, tt.want)
			}
		})
	}
}

func TestForwardCredentialsLeavesCookiesToTheJar(t *testing.T) {
	trusted := newRecorder(t)
	origin := redirectTo(t, otherHost(trusted.URL)+"/next")

	forward, err := ParseForwardCredentials(otherHost(trusted.URL))
	if err != nil {
		t.Fatalf("ParseForwardCredentials: %v", err)
	}

	hdr := make(http.Header)
	hdr.Set("Authorization", "Bearer secret")
	hdr.Set("Cookie", "session=secret")
	execute(
		t,
		&restfile.Request{Method: "GET", URL: origin.URL, Headers: hdr},
		Options{ForwardCredentials: forward},
	)

	got := trusted.received(t)
	if got.Get("Authorization") != "Bearer secret" {
		t.Fatalf("Authorization = %q, want the forwarded credential", got.Get("Authorization"))
	}
	if leaked := got.Get("Cookie"); leaked != "" {
		t.Fatalf("Cookie = %q, want the jar to decide where cookies go", leaked)
	}
}

func TestForwardCredentialsRefusesADowngradePartwayAlong(t *testing.T) {
	last := newRecorder(t)
	middle := redirectTo(t, otherHost(last.URL)+"/last")
	first := redirectTo(t, middle.URL+"/next")

	forward, err := ParseForwardCredentials("true")
	if err != nil {
		t.Fatal(err)
	}

	hdr := make(http.Header)
	hdr.Set("Authorization", "Bearer secret")
	hdr.Set("X-Api-Key", "secret")
	execute(
		t,
		&restfile.Request{Method: "GET", URL: first.URL, Headers: hdr},
		Options{ForwardCredentials: forward},
	)

	got := last.received(t)
	if got.Get("Authorization") == "" || got.Get("X-Api-Key") == "" {
		t.Fatalf("an allowed plain HTTP chain lost its credentials: %v", got)
	}
}

func hop(t *testing.T, opts Options, from, to string, copied http.Header) (*http.Request, error) {
	t.Helper()

	initial, err := http.NewRequest(http.MethodGet, from, nil)
	if err != nil {
		t.Fatal(err)
	}
	initial.Header = copied.Clone()

	next, err := http.NewRequest(http.MethodGet, to, nil)
	if err != nil {
		t.Fatal(err)
	}
	next.Header = copied.Clone()

	return next, redirectPolicy(opts)(next, []*http.Request{initial})
}

func TestRedirectDropsCookiesEvenWhenTheOriginIsTrusted(t *testing.T) {
	forward, err := ParseForwardCredentials("https://cdn.example.com")
	if err != nil {
		t.Fatal(err)
	}

	copied := make(http.Header)
	copied.Set("Cookie", "session=secret")
	copied.Set("Authorization", "Bearer secret")

	next, err := hop(
		t,
		Options{FollowRedirects: true, ForwardCredentials: forward},
		"https://example.com/x",
		"https://cdn.example.com/y",
		copied,
	)
	if err != nil {
		t.Fatalf("redirect policy: %v", err)
	}
	if leaked := next.Header.Get("Cookie"); leaked != "" {
		t.Errorf("Cookie = %q, want the jar to decide where cookies go", leaked)
	}
	if next.Header.Get("Authorization") != "Bearer secret" {
		t.Error("the trusted origin lost the credential it was trusted with")
	}
}

func TestRedirectKeepsCookiesWithinTheOrigin(t *testing.T) {
	copied := make(http.Header)
	copied.Set("Cookie", "session=secret")

	next, err := hop(
		t,
		Options{FollowRedirects: true},
		"https://example.com/x",
		"https://example.com/y",
		copied,
	)
	if err != nil {
		t.Fatalf("redirect policy: %v", err)
	}
	if next.Header.Get("Cookie") != "session=secret" {
		t.Fatal("a redirect inside the origin lost its cookies")
	}
}

func TestConfinedRedirectIsRefused(t *testing.T) {
	opts := Options{FollowRedirects: true, ConfineToOrigin: true}

	if _, err := hop(t, opts, "https://idp.example.com/token", "https://idp.example.com/v2", nil); err != nil {
		t.Fatalf("a redirect inside the origin was refused: %v", err)
	}

	_, err := hop(t, opts, "https://idp.example.com/token", "https://cdn.example.net/token", nil)
	if err == nil {
		t.Fatal("a redirect off the origin was followed")
	}
	if !strings.Contains(err.Error(), "https://cdn.example.net") {
		t.Fatalf("error = %v, want it to name the destination", err)
	}
}

func TestRedirectRefusesADowngradePartwayAlongAChain(t *testing.T) {
	last := newRecorder(t)
	middle := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, last.URL+"/last", http.StatusFound)
	}))
	defer middle.Close()
	first := redirectTo(t, middle.URL+"/next")

	forward, err := ParseForwardCredentials("true")
	if err != nil {
		t.Fatal(err)
	}

	hdr := make(http.Header)
	hdr.Set("Authorization", "Bearer secret")
	hdr.Set("X-Api-Key", "secret")
	execute(t, &restfile.Request{Method: "GET", URL: first.URL, Headers: hdr}, Options{
		ForwardCredentials: forward,
		InsecureSkipVerify: true,
	})

	got := last.received(t)
	for _, name := range []string{"Authorization", "X-Api-Key"} {
		if leaked := got.Get(name); leaked != "" {
			t.Errorf("%s left TLS partway along the chain as %q", name, leaked)
		}
	}
}

func TestRedirectNarrowsTheRefererToTheOrigin(t *testing.T) {
	target := newRecorder(t)
	first := redirectTo(t, otherHost(target.URL)+"/next")

	execute(t, &restfile.Request{
		Method: "GET",
		URL:    first.URL + "/tokens",
		Metadata: restfile.RequestMetadata{Auth: &restfile.AuthSpec{
			Type: restfile.AuthAPIKey,
			Params: map[string]string{
				authParamName:      "api_key",
				authParamValue:     "secret",
				authParamPlacement: authPlacementQuery,
			},
		}},
	}, Options{})

	got := target.received(t).Get("Referer")
	if strings.Contains(got, "secret") || strings.Contains(got, "/tokens") {
		t.Fatalf("Referer = %q, want the path and query left behind", got)
	}
	if want := first.URL + "/"; got != want {
		t.Fatalf("Referer = %q, want %q", got, want)
	}
}

func TestRedirectKeepsTheRefererTheRequestSet(t *testing.T) {
	target := newRecorder(t)
	first := redirectTo(t, otherHost(target.URL)+"/next")

	hdr := make(http.Header)
	hdr.Set("Referer", "https://docs.example.com/guide")
	execute(t, &restfile.Request{Method: "GET", URL: first.URL + "/start", Headers: hdr}, Options{})

	if got := target.received(t).Get("Referer"); got != "https://docs.example.com/guide" {
		t.Fatalf("Referer = %q, want the value the request set", got)
	}
}

func TestRedirectWithinTheOriginKeepsTheWholeReferer(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.URL.Path == "/moved" {
			got = req.Header.Clone()
			return
		}
		http.Redirect(w, req, "/moved", http.StatusFound)
	}))
	defer srv.Close()

	execute(t, &restfile.Request{Method: "GET", URL: srv.URL + "/start?api_key=secret"}, Options{})

	if want := srv.URL + "/start?api_key=secret"; got.Get("Referer") != want {
		t.Fatalf("Referer = %q, want %q", got.Get("Referer"), want)
	}
}

func TestRedirectNeverRestoresCredentialsAfterADowngrade(t *testing.T) {
	last := newRecorder(t)
	middle := redirectTo(t, otherHost(last.URL)+"/last")
	first := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, middle.URL+"/next", http.StatusFound)
	}))
	defer first.Close()

	forward, err := ParseForwardCredentials("true")
	if err != nil {
		t.Fatal(err)
	}

	hdr := make(http.Header)
	hdr.Set("Authorization", "Bearer secret")
	hdr.Set("X-Api-Key", "secret")
	execute(t, &restfile.Request{Method: "GET", URL: first.URL, Headers: hdr}, Options{
		ForwardCredentials: forward,
		InsecureSkipVerify: true,
	})

	got := last.received(t)
	for _, name := range []string{"Authorization", "X-Api-Key"} {
		if leaked := got.Get(name); leaked != "" {
			t.Errorf("%s came back a hop after the chain left TLS, as %q", name, leaked)
		}
	}
}

func TestRedirectWithinTheOriginKeepsTheWholeRefererMidChain(t *testing.T) {
	got := make(chan http.Header, 1)
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/two" {
			got <- r.Header.Clone()
			return
		}
		http.Redirect(w, r, "/two", http.StatusFound)
	}))
	t.Cleanup(target.Close)
	first := redirectTo(t, otherHost(target.URL)+"/one?secret=shh")

	execute(t, &restfile.Request{Method: "GET", URL: first.URL + "/start"}, Options{})

	want := otherHost(target.URL) + "/one?secret=shh"
	select {
	case hdr := <-got:
		if hdr.Get("Referer") != want {
			t.Fatalf("Referer = %q, want the whole URL of the same-origin hop %q", hdr.Get("Referer"), want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the redirect target was never reached")
	}
}

func TestRedirectBackToTheOwnerOriginNarrowsTheReferer(t *testing.T) {
	got := make(chan http.Header, 1)
	var owner, hop *httptest.Server
	owner = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/back" {
			got <- r.Header.Clone()
			return
		}
		http.Redirect(w, r, otherHost(hop.URL)+"/hop?secret=shh", http.StatusFound)
	}))
	t.Cleanup(owner.Close)
	hop = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, owner.URL+"/back", http.StatusFound)
	}))
	t.Cleanup(hop.Close)

	execute(t, &restfile.Request{Method: "GET", URL: owner.URL + "/start"}, Options{})

	want := otherHost(hop.URL) + "/"
	select {
	case hdr := <-got:
		if hdr.Get("Referer") != want {
			t.Fatalf("Referer = %q, want only the origin the hop came from %q", hdr.Get("Referer"), want)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the redirect target was never reached")
	}
}
