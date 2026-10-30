package oauth

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

// A browser may hold a second connection to the callback server that never
// sends a request. The login must not wait for it.
func TestAuthCodeDoesNotWaitForAConnectionWithoutARequest(t *testing.T) {
	mgr := NewManager(nil)
	mgr.SetRequestFunc(
		func(context.Context, *restfile.Request, httpx.Options) (*httpx.Response, error) {
			return &httpx.Response{
				Status:     "200 OK",
				StatusCode: http.StatusOK,
				Body:       []byte(`{"access_token":"code-token","token_type":"Bearer"}`),
				Headers:    http.Header{},
			}, nil
		},
	)
	launchBrowser = func(link string) error {
		auth, err := url.Parse(link)
		if err != nil {
			t.Fatal(err)
		}
		q := auth.Query()
		callback, err := url.Parse(q.Get("redirect_uri"))
		if err != nil {
			t.Fatal(err)
		}
		idle, err := net.Dial("tcp", callback.Host)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = idle.Close() })
		// Accept is FIFO, so once the callback is answered, idle was accepted.
		client := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}}
		res, err := client.Get(callback.String() + "?code=test-code&state=" + url.QueryEscape(q.Get("state")))
		if err != nil {
			t.Fatal(err)
		}
		return res.Body.Close()
	}
	t.Cleanup(func() { launchBrowser = openBrowser })

	cfg := Config{
		TokenURL:  "http://127.0.0.1:1/token",
		AuthURL:   "http://127.0.0.1:1/auth",
		ClientID:  "demo-client",
		GrantType: "authorization_code",
	}
	// Under the five seconds net/http lets a new connection sit before
	// http.Server.Shutdown treats it as idle.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	token, err := mgr.Token(ctx, "dev", cfg, httpx.Options{})
	if err != nil || ctx.Err() != nil {
		t.Fatalf("Token = %v, ctx = %v, want both nil", err, ctx.Err())
	}
	if token.AccessToken != "code-token" {
		t.Fatalf("access token = %q", token.AccessToken)
	}
}

// A clean shutdown must leave errCh empty, or wait can end the login with no
// code when it picks errCh over codeCh.
func TestCodeServerSendsNoErrorAfterACleanShutdown(t *testing.T) {
	redirect, ln, err := prepareRedirect("")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = ln.Close() }()
	s := newCodeServer(redirect, "state")
	s.serve(ln)
	s.shutdown(context.Background())
	<-s.srv.Done()
	// The goroutine that reports Serve errors also wakes on Done, so give it time to send.
	select {
	case err := <-s.errCh:
		t.Fatalf("errCh got %v after a clean shutdown, want nothing", err)
	case <-time.After(100 * time.Millisecond):
	}
}
