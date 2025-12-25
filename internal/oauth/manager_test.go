package oauth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/httpclient"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestManagerClientCredentialsBasic(t *testing.T) {
	mgr := NewManager(nil)
	var capturedForm url.Values
	var capturedAuth string
	var callCount int

	mgr.SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
			callCount++
			values, err := url.ParseQuery(req.Body.Text)
			if err != nil {
				t.Fatalf("parse form: %v", err)
			}
			capturedForm = values
			capturedAuth = req.Headers.Get("Authorization")
			if accept := req.Headers.Get("Accept"); accept != "application/json" {
				t.Fatalf("expected Accept header to request json, got %q", accept)
			}
			return &httpclient.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body: []byte(
					`{"access_token":"token-basic","token_type":"Bearer","expires_in":3600}`,
				),
				Headers: http.Header{},
			}, nil
		},
	)

	cfg := Config{
		TokenURL:     "https://auth.local/token",
		ClientID:     "my-client",
		ClientSecret: "my-secret",
		Scope:        "read write",
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	token, err := mgr.Token(ctx, "dev", cfg, httpclient.Options{})
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if token.AccessToken != "token-basic" {
		t.Fatalf("unexpected token %q", token.AccessToken)
	}
	if token.TokenType != "Bearer" {
		t.Fatalf("unexpected token type %q", token.TokenType)
	}

	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("my-client:my-secret"))
	if capturedAuth != expectedAuth {
		t.Fatalf("expected basic auth header, got %q", capturedAuth)
	}
	if capturedForm.Get("grant_type") != "client_credentials" {
		t.Fatalf("expected grant client_credentials, got %q", capturedForm.Get("grant_type"))
	}
	if capturedForm.Get("scope") != "read write" {
		t.Fatalf("expected scope to be preserved, got %q", capturedForm.Get("scope"))
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	if _, err := mgr.Token(ctx2, "dev", cfg, httpclient.Options{}); err != nil {
		t.Fatalf("second token request: %v", err)
	}
	if callCount != 1 {
		t.Fatalf("expected cached token reuse, calls=%d", callCount)
	}
}

func TestManagerClientCredentialsBodyAuth(t *testing.T) {
	var capturedForm url.Values
	var capturedAuth string
	mgr := NewManager(nil)
	mgr.SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
			values, err := url.ParseQuery(req.Body.Text)
			if err != nil {
				t.Fatalf("parse form: %v", err)
			}
			capturedForm = values
			capturedAuth = req.Headers.Get("Authorization")
			if accept := req.Headers.Get("Accept"); accept != "application/json" {
				t.Fatalf("expected Accept header to request json, got %q", accept)
			}
			return &httpclient.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       []byte(`{"access_token":"token-body","token_type":"Bearer"}`),
				Headers:    http.Header{},
			}, nil
		},
	)

	cfg := Config{
		TokenURL:     "https://auth.local/token",
		ClientID:     "client",
		ClientSecret: "secret",
		ClientAuth:   "body",
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	token, err := mgr.Token(ctx, "prod", cfg, httpclient.Options{})
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if token.AccessToken != "token-body" {
		t.Fatalf("unexpected token %q", token.AccessToken)
	}
	if capturedAuth != "" {
		t.Fatalf("expected no Authorization header, got %q", capturedAuth)
	}
	if capturedForm.Get("client_id") != "client" || capturedForm.Get("client_secret") != "secret" {
		t.Fatalf("expected credentials in form, got %v", capturedForm)
	}
}

func TestManagerClientCredentialsExplicitBasicWithEmptySecret(t *testing.T) {
	var capturedForm url.Values
	var capturedAuth string
	mgr := NewManager(nil)
	mgr.SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
			values, err := url.ParseQuery(req.Body.Text)
			if err != nil {
				t.Fatalf("parse form: %v", err)
			}
			capturedForm = values
			capturedAuth = req.Headers.Get("Authorization")
			return &httpclient.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       []byte(`{"access_token":"token-basic","token_type":"Bearer"}`),
				Headers:    http.Header{},
			}, nil
		},
	)

	cfg := Config{
		TokenURL:   "https://auth.local/token",
		ClientID:   "my-client",
		ClientAuth: "basic",
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	token, err := mgr.Token(ctx, "dev", cfg, httpclient.Options{})
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if token.AccessToken != "token-basic" {
		t.Fatalf("unexpected token %q", token.AccessToken)
	}

	expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("my-client:"))
	if capturedAuth != expectedAuth {
		t.Fatalf("expected basic auth header, got %q", capturedAuth)
	}
	if capturedForm.Get("client_id") != "" || capturedForm.Get("client_secret") != "" {
		t.Fatalf(
			"client credentials should not be included in body for basic auth, got %v",
			capturedForm,
		)
	}
}

func TestManagerTokenFormEncodedResponse(t *testing.T) {
	mgr := NewManager(nil)
	var accept string
	mgr.SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
			accept = req.Headers.Get("Accept")
			return &httpclient.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body: []byte(
					"access_token=form-token&token_type=bearer&refresh_token=refresh-form&expires_in=7200",
				),
				Headers: http.Header{},
			}, nil
		},
	)

	cfg := Config{
		TokenURL:     "https://auth.local/token",
		ClientID:     "form-client",
		ClientSecret: "form-secret",
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	token, err := mgr.Token(ctx, "dev", cfg, httpclient.Options{})
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if accept != "application/json" {
		t.Fatalf("expected Accept application/json, got %q", accept)
	}
	if token.AccessToken != "form-token" {
		t.Fatalf("unexpected access token %q", token.AccessToken)
	}
	if token.TokenType != "bearer" {
		t.Fatalf("unexpected token type %q", token.TokenType)
	}
	if token.Expiry.IsZero() {
		t.Fatalf("expected expiry to be set from expires_in")
	}
}

func TestManagerMergeCachedConfig(t *testing.T) {
	mgr := NewManager(nil)
	base := Config{
		TokenURL:     "https://auth.local/token",
		AuthURL:      "https://auth.local/auth",
		ClientID:     "client",
		ClientSecret: "secret",
		Scope:        "scope-a",
		GrantType:    "authorization_code",
		CacheKey:     "github",
		Extra:        map[string]string{"audience": "https://api.local"},
	}
	mgr.storeToken("github", base, Token{AccessToken: "cached"})

	merged := mgr.MergeCachedConfig("dev", Config{
		CacheKey:     "github",
		ClientSecret: "override-secret",
		Scope:        "",
		Extra:        map[string]string{"resource": "res-1"},
	})

	if merged.TokenURL != base.TokenURL || merged.AuthURL != base.AuthURL ||
		merged.ClientID != base.ClientID {
		t.Fatalf("expected base URLs/IDs to be inherited: %#v", merged)
	}
	if merged.ClientSecret != "override-secret" {
		t.Fatalf("override values should win, got %q", merged.ClientSecret)
	}
	if merged.Scope != base.Scope {
		t.Fatalf("scope should be inherited, got %q", merged.Scope)
	}
	if merged.Extra["audience"] != "https://api.local" || merged.Extra["resource"] != "res-1" {
		t.Fatalf("expected merged extras, got %#v", merged.Extra)
	}
}

func TestManagerRefreshToken(t *testing.T) {
	mgr := NewManager(nil)
	var grants []string

	mgr.SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
			values, err := url.ParseQuery(req.Body.Text)
			if err != nil {
				t.Fatalf("parse form: %v", err)
			}
			grant := values.Get("grant_type")
			grants = append(grants, grant)
			switch grant {
			case "client_credentials":
				return &httpclient.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body: []byte(
						`{"access_token":"token-initial","token_type":"Bearer","expires_in":1,"refresh_token":"refresh-1"}`,
					),
					Headers: http.Header{},
				}, nil
			case "refresh_token":
				if values.Get("refresh_token") != "refresh-1" {
					t.Fatalf("unexpected refresh token %q", values.Get("refresh_token"))
				}
				return &httpclient.Response{
					Status:     "200 OK",
					StatusCode: 200,
					Body: []byte(
						`{"access_token":"token-refreshed","token_type":"Bearer","expires_in":3600}`,
					),
					Headers: http.Header{},
				}, nil
			default:
				return &httpclient.Response{
					Status:     "400",
					StatusCode: 400,
					Body:       []byte("{}"),
					Headers:    http.Header{},
				}, nil
			}
		},
	)

	cfg := Config{
		TokenURL:     "https://auth.local/token",
		ClientID:     "client",
		ClientSecret: "secret",
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	token1, err := mgr.Token(ctx, "stage", cfg, httpclient.Options{})
	if err != nil {
		t.Fatalf("token1: %v", err)
	}
	if token1.AccessToken != "token-initial" {
		t.Fatalf("unexpected first token %q", token1.AccessToken)
	}

	ctx2, cancel2 := context.WithTimeout(context.Background(), time.Second)
	defer cancel2()
	token2, err := mgr.Token(ctx2, "stage", cfg, httpclient.Options{})
	if err != nil {
		t.Fatalf("token2: %v", err)
	}
	if token2.AccessToken != "token-refreshed" {
		t.Fatalf("expected refreshed token, got %q", token2.AccessToken)
	}

	if len(grants) != 2 || grants[0] != "client_credentials" || grants[1] != "refresh_token" {
		t.Fatalf("unexpected grants sequence %v", grants)
	}
}

func TestManagerAuthorizationCodePKCE(t *testing.T) {
	mgr := NewManager(nil)
	var tokenForm url.Values
	mgr.SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
			values, err := url.ParseQuery(req.Body.Text)
			if err != nil {
				t.Fatalf("parse form: %v", err)
			}
			tokenForm = values
			return &httpclient.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body: []byte(
					`{"access_token":"code-token","token_type":"Bearer","refresh_token":"refresh-code"}`,
				),
				Headers: http.Header{},
			}, nil
		},
	)

	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		redirect := q.Get("redirect_uri")
		if redirect == "" {
			t.Fatalf("missing redirect_uri")
		}
		if q.Get("response_type") != "code" {
			t.Fatalf("expected response_type=code, got %s", q.Get("response_type"))
		}
		if q.Get("code_challenge") == "" ||
			!strings.EqualFold(q.Get("code_challenge_method"), "S256") {
			t.Fatalf("expected code_challenge S256")
		}
		state := q.Get("state")
		http.Redirect(
			w,
			r,
			redirect+"?code=test-code&state="+url.QueryEscape(state),
			http.StatusFound,
		)
	}))
	defer authSrv.Close()

	cfg := Config{
		TokenURL:  authSrv.URL + "/token",
		AuthURL:   authSrv.URL + "/auth",
		ClientID:  "demo-client",
		Scope:     "read",
		GrantType: "authorization_code",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var authLink string
	launchBrowser = func(link string) error {
		authLink = link
		go http.Get(link) // nolint: errcheck
		return nil
	}
	t.Cleanup(func() {
		launchBrowser = openBrowser
	})

	token, err := mgr.Token(ctx, "dev", cfg, httpclient.Options{})
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	if token.AccessToken != "code-token" {
		t.Fatalf("unexpected access token %q", token.AccessToken)
	}
	if tokenForm.Get("code") != "test-code" {
		t.Fatalf("expected code to be forwarded")
	}
	if tokenForm.Get("redirect_uri") == "" ||
		!strings.Contains(tokenForm.Get("redirect_uri"), "127.0.0.1") {
		t.Fatalf("expected redirect_uri to be set, got %q", tokenForm.Get("redirect_uri"))
	}
	if tokenForm.Get("code_verifier") == "" {
		t.Fatalf("expected code_verifier to be set")
	}

	parsed, err := url.Parse(authLink)
	if err != nil {
		t.Fatalf("parse auth link: %v", err)
	}
	if parsed.Query().Get("redirect_uri") == "" || parsed.Query().Get("state") == "" {
		t.Fatalf("authorization URL missing redirect or state")
	}
}

func TestManagerAuthorizationCodeStateMismatch(t *testing.T) {
	mgr := NewManager(nil)
	mgr.SetRequestFunc(
		func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
			t.Fatalf("token exchange should not be called on state mismatch")
			return nil, nil
		},
	)

	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirect := r.URL.Query().Get("redirect_uri")
		http.Redirect(w, r, redirect+"?code=bad&state=other", http.StatusFound)
	}))
	defer authSrv.Close()

	cfg := Config{
		TokenURL:  authSrv.URL + "/token",
		AuthURL:   authSrv.URL + "/auth",
		ClientID:  "demo-client",
		Scope:     "read",
		GrantType: "authorization_code",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	launchBrowser = func(link string) error {
		go http.Get(link) // nolint: errcheck
		return nil
	}
	t.Cleanup(func() {
		launchBrowser = openBrowser
	})

	if _, err := mgr.Token(ctx, "dev", cfg, httpclient.Options{}); err == nil {
		t.Fatalf("expected state mismatch error")
	}
}

func TestPrepareRedirectPreservesQuery(t *testing.T) {
	redirect, ln, err := prepareRedirect("http://127.0.0.1:0/oauth/callback?foo=bar")
	if err != nil {
		t.Fatalf("prepare redirect: %v", err)
	}
	defer func() {
		if cerr := ln.Close(); cerr != nil {
			t.Fatalf("close listener: %v", cerr)
		}
	}()

	if redirect.Path != "/oauth/callback" {
		t.Fatalf("unexpected path %q", redirect.Path)
	}
	if redirect.RawQuery != "foo=bar" {
		t.Fatalf("expected query to be preserved, got %q", redirect.RawQuery)
	}
	if !strings.HasPrefix(redirect.Host, "127.0.0.1:") {
		t.Fatalf("unexpected host %q", redirect.Host)
	}
}
