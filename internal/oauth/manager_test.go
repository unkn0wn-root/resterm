package oauth

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/url"
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

	mgr.SetRequestFunc(func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
		callCount++
		values, err := url.ParseQuery(req.Body.Text)
		if err != nil {
			t.Fatalf("parse form: %v", err)
		}
		capturedForm = values
		capturedAuth = req.Headers.Get("Authorization")
		return &httpclient.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Body:       []byte(`{"access_token":"token-basic","token_type":"Bearer","expires_in":3600}`),
			Headers:    http.Header{},
		}, nil
	})

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
	mgr.SetRequestFunc(func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
		values, err := url.ParseQuery(req.Body.Text)
		if err != nil {
			t.Fatalf("parse form: %v", err)
		}
		capturedForm = values
		capturedAuth = req.Headers.Get("Authorization")
		return &httpclient.Response{
			Status:     "200 OK",
			StatusCode: 200,
			Body:       []byte(`{"access_token":"token-body","token_type":"Bearer"}`),
			Headers:    http.Header{},
		}, nil
	})

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

func TestManagerRefreshToken(t *testing.T) {
	mgr := NewManager(nil)
	var grants []string

	mgr.SetRequestFunc(func(ctx context.Context, req *restfile.Request, opts httpclient.Options) (*httpclient.Response, error) {
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
				Body:       []byte(`{"access_token":"token-initial","token_type":"Bearer","expires_in":1,"refresh_token":"refresh-1"}`),
				Headers:    http.Header{},
			}, nil
		case "refresh_token":
			if values.Get("refresh_token") != "refresh-1" {
				t.Fatalf("unexpected refresh token %q", values.Get("refresh_token"))
			}
			return &httpclient.Response{
				Status:     "200 OK",
				StatusCode: 200,
				Body:       []byte(`{"access_token":"token-refreshed","token_type":"Bearer","expires_in":3600}`),
				Headers:    http.Header{},
			}, nil
		default:
			return &httpclient.Response{Status: "400", StatusCode: 400, Body: []byte("{}"), Headers: http.Header{}}, nil
		}
	})

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
