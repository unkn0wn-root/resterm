package request

import (
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func grpcAuthRequest(auth *restfile.AuthSpec) *restfile.Request {
	return &restfile.Request{
		GRPC:     &restfile.GRPCRequest{Target: "localhost:50051", FullMethod: "/pkg.Svc/Call"},
		Metadata: restfile.RequestMetadata{Auth: auth},
	}
}

func TestApplyGRPCAuthInjectsHeaders(t *testing.T) {
	res := vars.NewResolver(vars.NewMapProvider("env", map[string]string{"token": "t0ps3cret"}))

	tests := []struct {
		name  string
		auth  *restfile.AuthSpec
		hdr   string
		value string
	}{
		{
			name:  "bearer",
			auth:  &restfile.AuthSpec{Type: "bearer", Params: map[string]string{"token": "{{token}}"}},
			hdr:   "Authorization",
			value: "Bearer t0ps3cret",
		},
		{
			name: "basic",
			auth: &restfile.AuthSpec{Type: "basic", Params: map[string]string{
				"username": "sam",
				"password": "pw",
			}},
			hdr:   "Authorization",
			value: "Basic c2FtOnB3",
		},
		{
			name: "apikey defaults to a header",
			auth: &restfile.AuthSpec{Type: "apikey", Params: map[string]string{
				"value": "{{token}}",
			}},
			hdr:   "X-API-Key",
			value: "t0ps3cret",
		},
		{
			name: "header",
			auth: &restfile.AuthSpec{Type: "header", Params: map[string]string{
				"header": "X-Token",
				"value":  "{{token}}",
			}},
			hdr:   "X-Token",
			value: "t0ps3cret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := grpcAuthRequest(tt.auth)
			if err := applyGRPCAuth(req, res); err != nil {
				t.Fatalf("apply grpc auth: %v", err)
			}
			if got := req.Headers.Get(tt.hdr); got != tt.value {
				t.Fatalf("%s = %q, want %q", tt.hdr, got, tt.value)
			}
		})
	}
}

func TestApplyGRPCAuthRejectsQueryPlacement(t *testing.T) {
	req := grpcAuthRequest(&restfile.AuthSpec{Type: "apikey", Params: map[string]string{
		"placement": "query",
		"name":      "key",
		"value":     "abc",
	}})

	err := applyGRPCAuth(req, vars.NewResolver())
	if err == nil {
		t.Fatal("expected query placement to be rejected")
	}
	if !strings.Contains(err.Error(), "not supported for grpc") {
		t.Fatalf("err = %v, want it to name grpc", err)
	}
}

func TestApplyGRPCAuthKeepsExplicitHeader(t *testing.T) {
	req := grpcAuthRequest(&restfile.AuthSpec{
		Type:   "bearer",
		Params: map[string]string{"token": "from-auth"},
	})
	req.Headers = map[string][]string{"Authorization": {"Bearer from-user"}}

	if err := applyGRPCAuth(req, vars.NewResolver()); err != nil {
		t.Fatalf("apply grpc auth: %v", err)
	}
	if got := req.Headers.Get("Authorization"); got != "Bearer from-user" {
		t.Fatalf("Authorization = %q, want the user value kept", got)
	}
}

func TestApplyGRPCAuthSkipsNonGRPCRequests(t *testing.T) {
	req := &restfile.Request{
		Metadata: restfile.RequestMetadata{Auth: &restfile.AuthSpec{
			Type:   "bearer",
			Params: map[string]string{"token": "abc"},
		}},
	}

	if err := applyGRPCAuth(req, vars.NewResolver()); err != nil {
		t.Fatalf("apply grpc auth: %v", err)
	}
	if got := req.Headers.Get("Authorization"); got != "" {
		t.Fatalf("Authorization = %q, want http to keep owning this", got)
	}
}

func TestGRPCAuthValueIsRegisteredAsSecret(t *testing.T) {
	auth := &restfile.AuthSpec{
		Type:   "bearer",
		Params: map[string]string{"token": "t0ps3cret"},
	}
	req := grpcAuthRequest(auth)
	before := CloneRequest(req)

	if err := applyGRPCAuth(req, vars.NewResolver()); err != nil {
		t.Fatalf("apply grpc auth: %v", err)
	}

	secs := InjectedAuthSecrets(auth, before, req)
	var found bool
	for _, s := range secs {
		if s == "t0ps3cret" {
			found = true
		}
	}
	if !found {
		t.Fatalf("secrets = %v, want the bare token registered for redaction", secs)
	}
}
