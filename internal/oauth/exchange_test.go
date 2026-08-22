package oauth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/protocol/httpx"
)

func TestTokenExchangeIgnoresTheRequestRedirectTrust(t *testing.T) {
	var reached atomic.Bool
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Store(true)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"leaked","token_type":"Bearer"}`))
	}))
	defer elsewhere.Close()

	idp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, elsewhere.URL+"/token", http.StatusTemporaryRedirect)
	}))
	defer idp.Close()

	forward, err := httpx.ParseForwardCredentials("true")
	if err != nil {
		t.Fatal(err)
	}

	mgr := NewManager(httpx.NewClient(nil))
	_, err = mgr.Token(context.Background(), "dev", Config{
		TokenURL:     idp.URL + "/token",
		ClientID:     "id",
		ClientSecret: "secret",
		GrantType:    GrantClientCredentials,
	}, httpx.Options{
		FollowRedirects:    true,
		ForwardCredentials: forward,
	})

	if err == nil {
		t.Fatal("the token exchange followed a redirect off its origin")
	}
	if !strings.Contains(err.Error(), "refusing to follow a redirect") {
		t.Fatalf("error = %v, want the confinement to be the reason", err)
	}
	if reached.Load() {
		t.Fatal("the redirect target received the token request")
	}
}
