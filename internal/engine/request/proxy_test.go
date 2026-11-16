package request

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	engcfg "github.com/unkn0wn-root/resterm/internal/engine"
	"github.com/unkn0wn-root/resterm/internal/redact"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestExecuteSendsThroughTemplatedProxy(t *testing.T) {
	t.Setenv("RESTERM_TEST_PROXY_PASS", leakedSecret)
	var (
		hits atomic.Int32
		auth atomic.Value
	)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.IsAbs() {
			hits.Add(1)
			auth.Store(r.Header.Get("Proxy-Authorization"))
		}
	}))
	defer proxy.Close()

	env := envWith(t, "dev", map[string]string{
		"p":          proxy.URL,
		"x":          "edge",
		"bad":        "localhost",
		"proxy.pass": "env:RESTERM_TEST_PROXY_PASS",
	})
	host := strings.TrimPrefix(proxy.URL, "http://")
	basic := "Basic " + base64.StdEncoding.EncodeToString([]byte("ops:"+leakedSecret))
	for _, tt := range []struct {
		name   string
		proxy  string
		hits   int32
		auth   string
		err    string
		secret bool
	}{
		{name: "whole value", proxy: "{{p}}", hits: 1},
		{name: "with path", proxy: "{{p}}/{{x}}", hits: 1},
		{name: "undefined", proxy: "{{nope}}", err: "expand proxy"},
		{name: "not a URL", proxy: "{{bad}}", err: `invalid proxy "localhost"`},
		{name: "secret", proxy: "http://ops:{{proxy.pass}}@" + host, hits: 1, auth: basic, secret: true},
		{name: "secret not a URL", proxy: "ops:{{proxy.pass}}@proxy", err: redact.Mask, secret: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			hits.Store(0)
			auth.Store("")
			req := &restfile.Request{
				Method:   http.MethodGet,
				URL:      "http://example.test/x",
				Settings: map[string]string{"proxy": tt.proxy},
			}
			res, err := New(engcfg.Config{}, nil).ExecuteWith(nil, req, env, ExecOptions{})
			if err != nil {
				t.Fatalf("ExecuteWith() error = %v", err)
			}
			if tt.err == "" && (res.Err != nil || res.Response == nil || res.Response.StatusCode != http.StatusOK) {
				t.Fatalf("result error = %v, response = %+v, want 200", res.Err, res.Response)
			}
			if tt.err != "" && (res.Err == nil || !strings.Contains(res.Err.Error(), tt.err)) {
				t.Fatalf("result error = %v, want it to contain %q", res.Err, tt.err)
			}
			if got := hits.Load(); got != tt.hits {
				t.Fatalf("proxy hits = %d, want %d", got, tt.hits)
			}
			if got := auth.Load(); got != tt.auth {
				t.Fatalf("Proxy-Authorization = %q, want %q", got, tt.auth)
			}
			if tt.secret {
				assertSecretStaysRedactable(t, res)
			}
		})
	}
}
