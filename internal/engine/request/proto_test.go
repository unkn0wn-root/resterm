package request

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/protocol/grpcx"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"github.com/unkn0wn-root/resterm/internal/vars"
)

func TestPrepareGRPCRequestExpandsTemplKeepMsg(t *testing.T) {
	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{
		"userId": "123",
		"token":  "abcd",
	}))

	req := &restfile.Request{
		Method: "GRPC",
		Body:   restfile.BodySource{Text: "{\"id\":\"{{userId}}\"}"},
		GRPC: &restfile.GRPCRequest{
			Target:     " localhost:50051 ",
			FullMethod: "/pkg.Service/GetUser",
			Message:    "{\"id\":\"{{userId}}\"}",
			Metadata: []restfile.MetadataPair{
				{Key: "authorization", Value: "Bearer {{token}}"},
			},
		},
	}

	if err := prepareGRPCRequest(req, resolver, grpcx.Options{}); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}

	if req.URL != "localhost:50051" {
		t.Fatalf("expected URL to be trimmed target, got %q", req.URL)
	}
	if strings.Contains(req.GRPC.Message, "{{") {
		t.Fatalf("expected message templates to be expanded, got %q", req.GRPC.Message)
	}
	if req.GRPC.MessageFile != "" {
		t.Fatalf("expected message file to be cleared when inline body provided")
	}
	want := "Bearer abcd"
	found := false
	for _, pair := range req.GRPC.Metadata {
		if pair.Key == "authorization" && pair.Value == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected metadata to be expanded to %q", want)
	}
}

func TestPrepareGRPCRequestUsesBodyOverride(t *testing.T) {
	resolver := vars.NewResolver()
	req := &restfile.Request{
		Method: "GRPC",
		Body:   restfile.BodySource{Text: "{\"name\":\"sam\"}"},
		GRPC: &restfile.GRPCRequest{
			Target:  "localhost:50051",
			Service: "UserService",
			Method:  "Create",
		},
	}

	if err := prepareGRPCRequest(req, resolver, grpcx.Options{}); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.FullMethod != "/UserService/Create" {
		t.Fatalf("expected full method to be inferred, got %q", req.GRPC.FullMethod)
	}
	if req.GRPC.Message != "{\"name\":\"sam\"}" {
		t.Fatalf("expected body override to populate grpc message, got %q", req.GRPC.Message)
	}
}

func TestPrepareGRPCRequestNormalizesSchemedTarget(t *testing.T) {
	resolver := vars.NewResolver()
	req := &restfile.Request{
		Method: "GRPC",
		GRPC: &restfile.GRPCRequest{
			Target:     "grpc://localhost:8082",
			FullMethod: "/pkg.Service/Call",
		},
	}

	if err := prepareGRPCRequest(req, resolver, grpcx.Options{}); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.Target != "localhost:8082" {
		t.Fatalf("expected target to be normalized, got %q", req.GRPC.Target)
	}
	if req.URL != "localhost:8082" {
		t.Fatalf("expected URL to mirror normalized target, got %q", req.URL)
	}
}

func TestPrepareGRPCRequestNormalizesSecureSchemes(t *testing.T) {
	resolver := vars.NewResolver()
	req := &restfile.Request{
		Method: "GRPC",
		GRPC: &restfile.GRPCRequest{
			Target:     "grpcs://api.example.com:8443",
			FullMethod: "/pkg.Service/Call",
		},
	}

	if err := prepareGRPCRequest(req, resolver, grpcx.Options{}); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.Target != "api.example.com:8443" {
		t.Fatalf("expected target to drop grpcs scheme, got %q", req.GRPC.Target)
	}
	if v, ok := req.GRPC.Plaintext.Get(); !ok || v {
		t.Fatalf("expected secure scheme to enforce TLS, got %+v", req.GRPC.Plaintext)
	}
}

func TestNormalizeGRPCTargetPreservesQuery(t *testing.T) {
	req := &restfile.Request{
		Method: "GRPC",
		GRPC: &restfile.GRPCRequest{
			Target:     "grpc://localhost:9000/service?alt=blue",
			FullMethod: "/svc.Method",
		},
	}

	if err := prepareGRPCRequest(req, vars.NewResolver(), grpcx.Options{}); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.Target != "localhost:9000/service?alt=blue" {
		t.Fatalf("expected query to be preserved, got %q", req.GRPC.Target)
	}
}

func TestPrepareGRPCRequestExpandsDescriptorSet(t *testing.T) {
	resolver := vars.NewResolver(
		vars.NewMapProvider(
			"doc",
			map[string]string{"grpc.descriptor": "./testdata/example.protoset"},
		),
	)
	req := &restfile.Request{
		Method: "GRPC",
		GRPC: &restfile.GRPCRequest{
			Target:        "localhost:50051",
			FullMethod:    "/pkg.Svc/Call",
			DescriptorSet: "{{grpc.descriptor}}",
		},
	}

	if err := prepareGRPCRequest(req, resolver, grpcx.Options{}); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.DescriptorSet != "./testdata/example.protoset" {
		t.Fatalf("expected descriptor set to be expanded, got %q", req.GRPC.DescriptorSet)
	}
}

func TestPrepareGRPCRequestExpandsMessageFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "msg.json")
	if err := os.WriteFile(path, []byte(`{"id":"{{userId}}"}`), 0o600); err != nil {
		t.Fatalf("write message file: %v", err)
	}

	resolver := vars.NewResolver(vars.NewMapProvider("env", map[string]string{
		"userId": "abc",
	}))
	req := &restfile.Request{
		Method: "GRPC",
		Body: restfile.BodySource{
			FilePath: "msg.json",
			Options:  restfile.BodyOptions{ExpandTemplates: true},
		},
		GRPC: &restfile.GRPCRequest{
			Target:     "localhost:50051",
			FullMethod: "/pkg.Service/Get",
		},
	}

	if err := prepareGRPCRequest(req, resolver, grpcx.Options{BaseDir: dir}); err != nil {
		t.Fatalf("prepareGRPCRequest returned error: %v", err)
	}
	if req.GRPC.MessageFile != "msg.json" {
		t.Fatalf("expected message file to be preserved, got %q", req.GRPC.MessageFile)
	}
	if req.GRPC.Message != "" {
		t.Fatalf("expected inline message to stay empty, got %q", req.GRPC.Message)
	}
	if v, ok := req.GRPC.MessageExpanded.Get(); !ok || v != `{"id":"abc"}` {
		t.Fatalf("expected expanded message, got %+v", req.GRPC.MessageExpanded)
	}
}
