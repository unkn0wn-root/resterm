package httpx

import (
	"context"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestHostHeaderReachesTheServer(t *testing.T) {
	for _, h2 := range []bool{false, true} {
		t.Run(map[bool]string{false: "http/1.1", true: "http/2"}[h2], func(t *testing.T) {
			var mu sync.Mutex
			var hosts []string
			var srv *httptest.Server
			srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				hosts = append(hosts, r.Host)
				mu.Unlock()
				switch r.URL.Path {
				case "/path":
					http.Redirect(w, r, "/full", http.StatusFound)
				case "/full":
					http.Redirect(w, r, srv.URL+"/end", http.StatusFound)
				}
			}))
			srv.EnableHTTP2 = h2
			srv.StartTLS()
			t.Cleanup(srv.Close)

			req := &restfile.Request{
				Method:  "GET",
				URL:     srv.URL + "/path",
				Headers: http.Header{"Host": {"api.internal"}},
			}
			resp := execute(t, req, Options{InsecureSkipVerify: true})

			mu.Lock()
			defer mu.Unlock()
			urlHost := mustParseURL(t, srv.URL).Host
			if want := []string{"api.internal", "api.internal", urlHost}; !slices.Equal(hosts, want) {
				t.Fatalf("server saw Host %q, want %q", hosts, want)
			}
			if resp.ReqHost != urlHost || resp.RequestHeaders.Get("Host") != "" {
				t.Fatalf(
					"reported Host %q and header %q, want the last hop's Host only",
					resp.ReqHost,
					resp.RequestHeaders.Get("Host"),
				)
			}
		})
	}
}

func TestWebSocketRejectsAHostItCannotSend(t *testing.T) {
	for _, tt := range []struct {
		host   string
		reject bool
	}{
		{host: "api.internal", reject: true},
		{host: "127.0.0.1:1"},
	} {
		req := &restfile.Request{
			Method:    "GET",
			URL:       "ws://127.0.0.1:1/socket",
			Headers:   http.Header{"Host": {tt.host}},
			WebSocket: &restfile.WebSocketRequest{},
		}
		_, _, err := NewClient(nil).StartWebSocket(context.Background(), req, vars.NewResolver(), Options{})
		if got := err != nil && strings.Contains(err.Error(), "Host header"); got != tt.reject {
			t.Fatalf("Host %q: err = %v, want rejected = %v", tt.host, err, tt.reject)
		}
	}
}

// Two virtual hosts on one address are separate applications, so credentials
// written for one do not follow a redirect to the other.
func TestCredentialsStayWithTheHostHeader(t *testing.T) {
	for _, tt := range []struct {
		name string
		// host, location and landing take the server's host:port.
		host     func(addr string) string
		location func(addr string) string
		landing  func(addr string) string
		keep     bool
	}{
		{
			name:     "redirect to a path keeps the Host",
			location: func(string) string { return "/landing" },
			landing:  func(string) string { return "private.example" },
			keep:     true,
		},
		{
			name:     "redirect to the same address by URL",
			location: func(addr string) string { return "http://" + addr + "/landing" },
			landing:  func(addr string) string { return addr },
		},
		// net/http keeps the Host through a network-path reference.
		{
			name:     "redirect to another server by network path",
			location: func(addr string) string { return "//" + otherHost(addr) + "/landing" },
			landing:  otherHost,
		},
		// The Host stays the same, but the request now goes to another server.
		{
			name:     "redirect to the server the Host names",
			host:     otherHost,
			location: func(addr string) string { return "http://" + otherHost(addr) + "/landing" },
			landing:  otherHost,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := make(chan *http.Request, 1)
			var addr string
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/start" {
					w.Header().Set("Location", tt.location(addr))
					w.WriteHeader(http.StatusFound)
					return
				}
				got <- r.Clone(r.Context())
			}))
			t.Cleanup(srv.Close)
			addr = mustParseURL(t, srv.URL).Host
			host := "private.example"
			if tt.host != nil {
				host = tt.host(addr)
			}

			jar, err := cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			jar.SetCookies(mustParseURL(t, "http://private.example"), []*http.Cookie{{Name: "jar", Value: "private"}})
			req := &restfile.Request{
				Method:  "GET",
				URL:     srv.URL + "/start?api_key=secret",
				Headers: http.Header{"Host": {host}, "Cookie": {"session=secret"}},
				Metadata: restfile.RequestMetadata{Auth: &restfile.AuthSpec{
					Type:   restfile.AuthBasic,
					Params: map[string]string{authParamUsername: "alice", authParamPassword: "pw"},
				}},
			}
			execute(t, req, Options{CookieJar: jar})

			r := <-got
			if want := tt.landing(addr); r.Host != want {
				t.Fatalf("landing Host = %q, want %q", r.Host, want)
			}
			auth, cookie := r.Header.Get("Authorization"), r.Header.Get("Cookie")
			if tt.keep != (auth != "") || tt.keep != strings.Contains(cookie, "session=secret") {
				t.Fatalf(
					"Authorization %q and Cookie %q reached the landing hop, want kept = %v",
					auth,
					cookie,
					tt.keep,
				)
			}
			if !tt.keep && cookie != "" {
				t.Fatalf("Cookie %q reached another application", cookie)
			}
			if ref := r.Header.Get("Referer"); tt.keep != strings.Contains(ref, "api_key") {
				t.Fatalf("Referer %q reached the landing hop, want the whole URL = %v", ref, tt.keep)
			}
		})
	}
}
