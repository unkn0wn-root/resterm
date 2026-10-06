package httpx

import (
	"context"
	"net/http"
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
