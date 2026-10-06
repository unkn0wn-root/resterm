package httpx

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"io"
	"maps"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/http/digest"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

const (
	digestUser = "alice"
	digestPass = "s3cret"
)

var digestParamRE = regexp.MustCompile(`(\w+)=(?:"([^"]*)"|([^,\s]+))`)

func md5Hex(s string) string {
	sum := md5.Sum([]byte(s))
	return hex.EncodeToString(sum[:])
}

func challenge(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Digest realm="test", qop="auth", nonce="n0nce", opaque="op"`)
	w.WriteHeader(http.StatusUnauthorized)
}

// verdict returns "-" for no auth, "ok" for valid Digest auth, or "bad".
func verdict(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if h == "" {
		return "-"
	}
	p := map[string]string{}
	for _, m := range digestParamRE.FindAllStringSubmatch(strings.TrimPrefix(h, "Digest "), -1) {
		p[m[1]] = m[2] + m[3]
	}
	ha1 := md5Hex(digestUser + ":test:" + digestPass)
	ha2 := md5Hex(r.Method + ":" + r.URL.RequestURI())
	want := md5Hex(strings.Join([]string{ha1, "n0nce", p["nc"], p["cnonce"], "auth", ha2}, ":"))
	if p["uri"] != r.URL.RequestURI() || p["response"] != want || p["opaque"] != "op" {
		return "bad"
	}
	return "ok"
}

type hits struct {
	mu  sync.Mutex
	got []string
}

func (h *hits) add(r *http.Request) string {
	v := verdict(r)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.got = append(h.got, r.Method+" "+r.URL.Path+" "+v)
	return v
}

func (h *hits) want(t *testing.T, want ...string) {
	t.Helper()
	h.mu.Lock()
	defer h.mu.Unlock()
	if !slices.Equal(h.got, want) {
		t.Fatalf("server saw %q, want %q", h.got, want)
	}
}

func protected(t *testing.T, h *hits) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.add(r) != "ok" {
			challenge(w)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func digestRequest(method, rawURL string) *restfile.Request {
	return &restfile.Request{
		Method: method,
		URL:    rawURL,
		Metadata: restfile.RequestMetadata{Auth: &restfile.AuthSpec{
			Type:   restfile.AuthDigest,
			Params: map[string]string{authParamUsername: digestUser, authParamPassword: digestPass},
		}},
	}
}

func TestDigestAnswersTheChallenge(t *testing.T) {
	var mu sync.Mutex
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		bodies = append(bodies, string(b))
		mu.Unlock()
		if verdict(r) != "ok" {
			challenge(w)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}))
	t.Cleanup(srv.Close)

	req := digestRequest("POST", srv.URL+"/items?x=1")
	req.Body.Text = `{"a":1}`
	resp := execute(t, req, Options{})

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("status = %d, want 201", resp.StatusCode)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(bodies, []string{`{"a":1}`, `{"a":1}`}) {
		t.Fatalf("bodies = %q, want the body sent twice", bodies)
	}
	if got := resp.RequestHeaders.Get("Authorization"); !strings.HasPrefix(got, "Digest ") {
		t.Fatalf("reported request Authorization = %q, want the answer that was sent", got)
	}
}

func TestDigestChallengeCookiesSurviveRedirects(t *testing.T) {
	for _, host := range []string{"", "api.internal"} {
		t.Run("Host="+host, func(t *testing.T) {
			var retry, next atomic.Pointer[http.Request]
			mux := http.NewServeMux()
			mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
				if verdict(r) != "ok" {
					http.SetCookie(w, &http.Cookie{Name: "sid", Value: "fresh"})
					challenge(w)
					return
				}
				retry.Store(r)
				http.Redirect(w, r, "/next", http.StatusFound)
			})
			mux.HandleFunc("GET /next", func(w http.ResponseWriter, r *http.Request) {
				next.Store(r)
			})
			srv := httptest.NewServer(mux)
			t.Cleanup(srv.Close)

			jar, err := cookiejar.New(nil)
			if err != nil {
				t.Fatal(err)
			}
			u := mustParseURL(t, srv.URL)
			if host != "" {
				u.Host = host
			}
			jar.SetCookies(u, []*http.Cookie{{Name: "sid", Value: "stale"}, {Name: "theme", Value: "dark"}})

			req := digestRequest("GET", srv.URL+"/login")
			req.Headers = http.Header{"Cookie": {"sid=stale; lang=en"}}
			if host != "" {
				req.Headers.Set("Host", host)
			}
			if resp := execute(t, req, Options{CookieJar: jar}); resp.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want 200", resp.StatusCode)
			}

			checkCookies := func(stage string, r *http.Request, want map[string]string) {
				t.Helper()
				if r == nil {
					t.Fatalf("%s: no request received", stage)
				}
				cookies, err := http.ParseCookie(r.Header.Get("Cookie"))
				if err != nil {
					t.Fatalf("%s: %v", stage, err)
				}
				got := map[string]string{}
				for _, c := range cookies {
					got[c.Name] += c.Value
				}
				if !maps.Equal(got, want) {
					t.Fatalf("%s cookies = %v, want %v", stage, got, want)
				}
			}
			want := map[string]string{"lang": "en", "theme": "dark", "sid": "fresh"}
			checkCookies("retry", retry.Load(), want)
			checkCookies("redirect", next.Load(), want)

			execute(t, &restfile.Request{
				Method: "GET", URL: srv.URL + "/next", Headers: http.Header{"Host": {host}},
			}, Options{CookieJar: jar})
			checkCookies("later request", next.Load(), map[string]string{"theme": "dark", "sid": "fresh"})
		})
	}
}

func TestDigestDoesNotWaitForTheChallengeBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if verdict(r) == "ok" {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = io.WriteString(w, "data: hi\n\n")
			return
		}
		challenge(w)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)

	t.Run("http", func(t *testing.T) {
		resp, err := NewClient(
			nil,
		).Execute(t.Context(), digestRequest("GET", srv.URL), nil, Options{Timeout: 5 * time.Second})
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("Execute = %v, %v, want 200", resp, err)
		}
	})
	t.Run("sse", func(t *testing.T) {
		req := digestRequest("GET", srv.URL)
		req.SSE = &restfile.SSERequest{}
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
		defer cancel()
		handle, fallback, err := NewClient(nil).StartSSE(ctx, req, vars.NewResolver(), Options{})
		if err != nil || fallback != nil {
			t.Fatalf("StartSSE = %v, %v, want the stream", fallback, err)
		}
		<-handle.Session.Done()
	})
}

type trackedBody struct {
	io.Reader
	closed *atomic.Int32
}

func (b trackedBody) Close() error {
	b.closed.Add(1)
	return nil
}

func TestDigestClosesEveryRequestBody(t *testing.T) {
	srv := protected(t, &hits{})
	creds := Options{digest: &digest.Credentials{Username: digestUser, Password: digestPass}}

	for _, tt := range []struct {
		name    string
		getBody error
	}{
		{name: "answered"},
		{name: "replay fails", getBody: errors.New("no replay")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var opened, closed atomic.Int32
			body := func() io.ReadCloser {
				opened.Add(1)
				return trackedBody{strings.NewReader("payload"), &closed}
			}
			req, err := http.NewRequest(http.MethodPut, srv.URL, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Body, req.ContentLength = body(), int64(len("payload"))
			req.GetBody = func() (io.ReadCloser, error) {
				if tt.getBody != nil {
					return nil, tt.getBody
				}
				return body(), nil
			}

			resp, err := withDigest(&http.Client{}, creds).Do(req)
			if tt.getBody != nil {
				if !errors.Is(err, tt.getBody) {
					t.Fatalf("err = %v, want the GetBody error", err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				_ = resp.Body.Close()
			}
			if o, c := opened.Load(), closed.Load(); o != c {
				t.Fatalf("opened %d bodies, closed %d", o, c)
			}
		})
	}
}

func TestDigestAnswersEachRedirectHopForItsOwnURI(t *testing.T) {
	h := &hits{}
	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		if h.add(r) != "ok" {
			challenge(w)
			return
		}
		http.Redirect(w, r, "/b", http.StatusSeeOther)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		if h.add(r) != "ok" {
			challenge(w)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/old", func(w http.ResponseWriter, r *http.Request) {
		h.add(r)
		http.Redirect(w, r, "/b", http.StatusMovedPermanently)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	t.Run("challenge then redirect", func(t *testing.T) {
		h.got = nil
		if resp := execute(t, digestRequest("POST", srv.URL+"/a"), Options{}); resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		h.want(t, "POST /a -", "POST /a ok", "GET /b -", "GET /b ok")
	})
	t.Run("redirect then challenge", func(t *testing.T) {
		h.got = nil
		if resp := execute(t, digestRequest("GET", srv.URL+"/old"), Options{}); resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		h.want(t, "GET /old -", "GET /b -", "GET /b ok")
	})
}

func TestDigestFollowsTheCredentialRules(t *testing.T) {
	t.Run("another origin", func(t *testing.T) {
		for _, tt := range []struct {
			name    string
			forward bool
			want    []string
		}{
			{name: "not allowed", want: []string{"GET /x -"}},
			{name: "allowed", forward: true, want: []string{"GET /x -", "GET /x ok"}},
		} {
			t.Run(tt.name, func(t *testing.T) {
				h := &hits{}
				other := otherHost(protected(t, h).URL)
				first := redirectTo(t, other+"/x")
				var opts Options
				if tt.forward {
					forward, err := ParseForwardCredentials(other)
					if err != nil {
						t.Fatal(err)
					}
					opts.ForwardCredentials = forward
				}
				execute(t, digestRequest("GET", first.URL), opts)
				h.want(t, tt.want...)
			})
		}
	})

	t.Run("left tls", func(t *testing.T) {
		h := &hits{}
		plain := protected(t, h)
		first := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, plain.URL+"/x", http.StatusFound)
		}))
		t.Cleanup(first.Close)
		forward, err := ParseForwardCredentials("true")
		if err != nil {
			t.Fatal(err)
		}
		execute(t, digestRequest("GET", first.URL), Options{ForwardCredentials: forward, InsecureSkipVerify: true})
		h.want(t, "GET /x -")
	})
}

func TestDigestSendsAuthorizationAsWritten(t *testing.T) {
	var got atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.Store(r.Header.Values("Authorization"))
		challenge(w)
	}))
	t.Cleanup(srv.Close)
	req := digestRequest("GET", srv.URL)
	req.Headers = http.Header{"Authorization": {"Bearer mine"}}
	if resp := execute(t, req, Options{}); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want the original 401", resp.StatusCode)
	}
	if v := got.Load().([]string); !slices.Equal(v, []string{"Bearer mine"}) {
		t.Fatalf("Authorization = %q, want the explicit header", v)
	}
}

func TestDigestOverridesURLUserInfo(t *testing.T) {
	h := &hits{}
	u := mustParseURL(t, protected(t, h).URL)
	u.User = url.UserPassword("bob", "pw")
	if resp := execute(t, digestRequest("GET", u.String()+"/x"), Options{}); resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	h.want(t, "GET /x -", "GET /x ok")
}

func TestDigestAnswersOnce(t *testing.T) {
	h := &hits{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.add(r)
		challenge(w)
	}))
	t.Cleanup(srv.Close)
	if resp := execute(t, digestRequest("GET", srv.URL), Options{}); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
	h.want(t, "GET / -", "GET / ok")
}

func TestDigestAnswersEveryAttempt(t *testing.T) {
	h := &hits{}
	srv := protected(t, h)
	plan, err := NewClient(nil).PrepareAttempts(t.Context(), digestRequest("GET", srv.URL), nil, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		resp, err := plan.Execute(t.Context())
		if err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("attempt = %v %v, want 200", resp, err)
		}
	}
	h.want(t, "GET / -", "GET / ok", "GET / -", "GET / ok")
}

func TestDigestIsRejectedForWebSocket(t *testing.T) {
	req := digestRequest("GET", "ws://127.0.0.1:1/socket")
	req.WebSocket = &restfile.WebSocketRequest{}
	_, _, err := NewClient(nil).StartWebSocket(t.Context(), req, vars.NewResolver(), Options{})
	if err == nil || !strings.Contains(err.Error(), "digest auth is not supported for websocket") {
		t.Fatalf("err = %v, want digest rejected", err)
	}
}
