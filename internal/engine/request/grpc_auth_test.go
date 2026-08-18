package request

import (
	"strings"
	"testing"

	xplain "github.com/unkn0wn-root/resterm/internal/explain"
	"github.com/unkn0wn-root/resterm/internal/http/header"
	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func grpcAuthRequest(auth *restfile.AuthSpec, meta ...restfile.MetadataPair) *restfile.Request {
	return &restfile.Request{
		GRPC: &restfile.GRPCRequest{
			Target:     "localhost:50051",
			FullMethod: "/pkg.Svc/Call",
			Metadata:   meta,
		},
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

func TestApplyGRPCAuthKeepsLowercaseExplicitHeaderWithoutExpandingAuth(t *testing.T) {
	req := grpcAuthRequest(&restfile.AuthSpec{
		Type:   "bearer",
		Params: map[string]string{"token": "{{missing}}"},
	})
	req.Headers = map[string][]string{"authorization": {"Bearer from-user"}}

	if err := applyGRPCAuth(req, vars.NewResolver()); err != nil {
		t.Fatalf("apply grpc auth: %v", err)
	}
	if got := header.Value(req.Headers, "Authorization"); got != "Bearer from-user" {
		t.Fatalf("authorization = %q, want the user value kept", got)
	}
	if len(req.Headers) != 1 {
		t.Fatalf("auth added a second authorization header: %v", req.Headers)
	}
}

func TestApplyGRPCAuthKeepsParsedExplicitHeaderWithoutExpandingAuth(t *testing.T) {
	doc := parser.Parse("auth.http", []byte(`
# @auth bearer {{missing}}
# @grpc pkg.Service/Call
# @grpc-plaintext true
GRPC grpc://127.0.0.1:8082
Authorization: Bearer from-user

{}
`))
	if len(doc.Errors) != 0 || len(doc.Requests) != 1 {
		t.Fatalf("parse errors=%v requests=%d", doc.Errors, len(doc.Requests))
	}
	req := doc.Requests[0]
	if err := applyGRPCAuth(req, vars.NewResolver()); err != nil {
		t.Fatalf("apply grpc auth with headers %v: %v", req.Headers, err)
	}
	if got := req.Headers.Get("Authorization"); got != "Bearer from-user" {
		t.Fatalf("authorization = %q, want the parsed value kept: %v", got, req.Headers)
	}
}

func TestApplyGRPCAuthKeepsExplicitMetadata(t *testing.T) {
	// The broken token proves overridden auth params are never expanded.
	req := grpcAuthRequest(&restfile.AuthSpec{
		Type:   "bearer",
		Params: map[string]string{"token": "{{missing}}"},
	})
	req.GRPC.Metadata = []restfile.MetadataPair{{Key: "authorization", Value: "Bearer from-user"}}

	if err := applyGRPCAuth(req, vars.NewResolver()); err != nil {
		t.Fatalf("apply grpc auth: %v", err)
	}
	if got := req.Headers.Get("Authorization"); got != "" {
		t.Fatalf("Authorization = %q, want no injected duplicate", got)
	}
}

func TestSetRequestHeaderIfMissingHonoursGRPCMetadata(t *testing.T) {
	req := grpcAuthRequest(nil)
	req.GRPC.Metadata = []restfile.MetadataPair{{Key: "x-api-key", Value: "explicit"}}

	if setRequestHeaderIfMissing(req, "X-API-Key", "injected") {
		t.Fatal("expected explicit metadata to block the injection")
	}
	if got := req.Headers.Get("X-API-Key"); got != "" {
		t.Fatalf("X-API-Key = %q, want empty", got)
	}
}

func TestExplainAuthPreviewHonoursExplicitGRPCMetadata(t *testing.T) {
	tests := []struct {
		name string
		auth *restfile.AuthSpec
	}{
		{
			name: "command",
			auth: &restfile.AuthSpec{
				Type:   "command",
				Params: map[string]string{"argv": `["demo-auth"]`},
			},
		},
		{
			name: "oauth2",
			auth: &restfile.AuthSpec{
				Type:   "oauth2",
				Params: map[string]string{"token_url": "http://token.test"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e, _ := newPreviewTestEngine(t)
			req := grpcAuthRequest(tt.auth)
			req.GRPC.Metadata = []restfile.MetadataPair{{Key: "authorization", Value: "Bearer from-user"}}

			out, err := e.prepareExplainAuthPreview(nil, req, vars.NewResolver(), testEnv("").Resolve())
			if err != nil {
				t.Fatalf("prepare auth preview: %v", err)
			}
			if out.status != xplain.StageOK {
				t.Fatalf("status = %v, want StageOK", out.status)
			}
			if len(out.notes) != 1 || !strings.Contains(out.notes[0], "@grpc-metadata") {
				t.Fatalf("notes = %v, want the @grpc-metadata note", out.notes)
			}
			if got := req.Headers.Get("Authorization"); got != "" {
				t.Fatalf("Authorization = %q, want no preview injection", got)
			}
		})
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
