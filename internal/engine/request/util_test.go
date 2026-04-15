package request

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestCloneRequestNorm(t *testing.T) {
	in := &restfile.Request{
		Metadata: restfile.RequestMetadata{
			Name: "  list users  ",
			Tags: []string{" api ", "", " smoke "},
		},
		Method: " post ",
		URL:    " https://example.com/users ",
		Body: restfile.BodySource{
			Text: "  keep me  ",
			GraphQL: &restfile.GraphQLBody{
				Query:         "  query Users  ",
				QueryFile:     " users.graphql ",
				VariablesFile: " vars.json ",
				OperationName: " Users ",
			},
		},
		GRPC: &restfile.GRPCRequest{
			Target:      " localhost:7443 ",
			Message:     "  {\"id\":1}  ",
			MessageFile: " msg.json ",
		},
		WebSocket: &restfile.WebSocketRequest{
			Steps: []restfile.WebSocketStep{{
				File: " payload.bin ",
			}},
		},
	}

	got := CloneRequest(in)
	if got == in {
		t.Fatal("CloneRequest() returned the original pointer")
	}
	if in.Method != " post " {
		t.Fatalf("input method changed to %q", in.Method)
	}
	if got.Method != "POST" {
		t.Fatalf("clone method = %q", got.Method)
	}
	if got.URL != "https://example.com/users" {
		t.Fatalf("clone url = %q", got.URL)
	}
	if got.Metadata.Name != "list users" {
		t.Fatalf("clone name = %q", got.Metadata.Name)
	}
	if got.GRPC == nil {
		t.Fatal("clone grpc request is nil")
	}
	if got.GRPC.Target != "localhost:7443" {
		t.Fatalf("clone grpc target = %q", got.GRPC.Target)
	}
	if got.GRPC.Message != "  {\"id\":1}  " {
		t.Fatalf("clone grpc message = %q", got.GRPC.Message)
	}
	if got.WebSocket == nil {
		t.Fatal("clone websocket request is nil")
	}
	if got.WebSocket.Steps[0].File != "payload.bin" {
		t.Fatalf("clone websocket file = %q", got.WebSocket.Steps[0].File)
	}
	if got.Body.GraphQL == in.Body.GraphQL {
		t.Fatal("GraphQL body was not cloned")
	}
	if got.WebSocket == in.WebSocket {
		t.Fatal("WebSocket config was not cloned")
	}
	if got := got.Metadata.Tags; !reflect.DeepEqual(got, []string{"api", "smoke"}) {
		t.Fatalf("clone tags = %#v", got)
	}
}

func TestCloneRequestDeepCopy(t *testing.T) {
	in := &restfile.Request{
		Headers:  http.Header{"X-Test": {"one"}},
		Settings: map[string]string{"mode": "strict"},
		Metadata: restfile.RequestMetadata{
			Auth: &restfile.AuthSpec{
				Type:   "bearer",
				Params: map[string]string{"token": "secret"},
			},
			Applies: []restfile.ApplySpec{{
				Uses: []string{"base"},
			}},
			Compare: &restfile.CompareSpec{
				Environments: []string{"dev", "prod"},
			},
			Trace: &restfile.TraceSpec{
				Enabled: true,
				Budgets: restfile.TraceBudget{
					Phases: map[string]time.Duration{"dns": time.Second},
				},
			},
		},
		GRPC: &restfile.GRPCRequest{
			Metadata: []restfile.MetadataPair{{Key: "x-id", Value: "1"}},
		},
		WebSocket: &restfile.WebSocketRequest{
			Options: restfile.WebSocketOptions{
				Subprotocols: []string{"chat"},
			},
			Steps: []restfile.WebSocketStep{{
				Type:  restfile.WebSocketStepSendText,
				Value: "hello",
			}},
		},
		SSH: &restfile.SSHSpec{
			Use:    "prod",
			Inline: &restfile.SSHProfile{Name: "ssh-inline"},
		},
		K8s: &restfile.K8sSpec{
			Use:    "cluster",
			Inline: &restfile.K8sProfile{Name: "k8s-inline"},
		},
	}

	got := CloneRequest(in)
	if got == nil {
		t.Fatal("CloneRequest() returned nil")
	}

	got.Headers.Set("X-Test", "two")
	got.Settings["mode"] = "relaxed"
	got.Metadata.Auth.Params["token"] = "changed"
	got.Metadata.Applies[0].Uses[0] = "override"
	got.Metadata.Compare.Environments[0] = "stage"
	got.Metadata.Trace.Budgets.Phases["dns"] = 2 * time.Second
	got.GRPC.Metadata[0].Value = "2"
	got.WebSocket.Options.Subprotocols[0] = "events"
	got.WebSocket.Steps[0].Value = "bye"
	got.SSH.Use = "staging"
	got.SSH.Inline.Name = "ssh-other"
	got.K8s.Use = "cluster-2"
	got.K8s.Inline.Name = "k8s-other"

	if in.Headers.Get("X-Test") != "one" {
		t.Fatalf("input headers changed to %q", in.Headers.Get("X-Test"))
	}
	if in.Settings["mode"] != "strict" {
		t.Fatalf("input settings changed to %q", in.Settings["mode"])
	}
	if in.Metadata.Auth.Params["token"] != "secret" {
		t.Fatalf("input auth params changed to %q", in.Metadata.Auth.Params["token"])
	}
	if in.Metadata.Applies[0].Uses[0] != "base" {
		t.Fatalf("input apply uses changed to %q", in.Metadata.Applies[0].Uses[0])
	}
	if in.Metadata.Compare.Environments[0] != "dev" {
		t.Fatalf("input compare environments changed to %q", in.Metadata.Compare.Environments[0])
	}
	if in.Metadata.Trace.Budgets.Phases["dns"] != time.Second {
		t.Fatalf("input trace phase changed to %s", in.Metadata.Trace.Budgets.Phases["dns"])
	}
	if in.GRPC.Metadata[0].Value != "1" {
		t.Fatalf("input grpc metadata changed to %q", in.GRPC.Metadata[0].Value)
	}
	if in.WebSocket.Options.Subprotocols[0] != "chat" {
		t.Fatalf("input websocket subprotocol changed to %q", in.WebSocket.Options.Subprotocols[0])
	}
	if in.WebSocket.Steps[0].Value != "hello" {
		t.Fatalf("input websocket step changed to %q", in.WebSocket.Steps[0].Value)
	}
	if in.SSH.Use != "prod" || in.SSH.Inline.Name != "ssh-inline" {
		t.Fatalf("input ssh spec changed to %#v", in.SSH)
	}
	if in.K8s.Use != "cluster" || in.K8s.Inline.Name != "k8s-inline" {
		t.Fatalf("input k8s spec changed to %#v", in.K8s)
	}
}

func TestRenderRequestTextGraphQL(t *testing.T) {
	req := &restfile.Request{
		Method: "POST",
		URL:    "https://example.com/graphql",
		Headers: http.Header{
			"X-Trace": {"123"},
			"Accept":  {"application/json"},
		},
		Body: restfile.BodySource{
			GraphQL: &restfile.GraphQLBody{
				OperationName: " Users ",
				Query:         "query Users {\n  users { id }\n}",
				VariablesFile: " vars.json ",
			},
		},
	}

	got := RenderRequestText(req)
	want := "" +
		"POST https://example.com/graphql\n" +
		"Accept: application/json\n" +
		"X-Trace: 123\n" +
		"\n" +
		"# @graphql\n" +
		"# @operation Users\n" +
		"query Users {\n" +
		"  users { id }\n" +
		"}\n" +
		"\n" +
		"# @variables\n" +
		"< vars.json\n"

	if got != want {
		t.Fatalf("RenderRequestText() mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestRenderRequestTextGRPC(t *testing.T) {
	req := &restfile.Request{
		Method: "POST",
		URL:    "grpc://example.com",
		Headers: http.Header{
			"Z-Trace": {"z"},
			"Accept":  {"application/grpc"},
		},
		GRPC: &restfile.GRPCRequest{
			FullMethod:    "/pkg.Service/Call",
			DescriptorSet: "descriptor.pb",
			UseReflection: false,
			Plaintext:     true,
			PlaintextSet:  true,
			Authority:     "api.internal",
			Metadata: []restfile.MetadataPair{
				{Key: "x-id", Value: "1"},
				{Key: "x-role", Value: "admin"},
			},
			Message: "{\"id\":1}",
		},
	}

	got := RenderRequestText(req)
	want := "" +
		"POST grpc://example.com\n" +
		"Accept: application/grpc\n" +
		"Z-Trace: z\n" +
		"\n" +
		"# @grpc pkg.Service/Call\n" +
		"# @grpc-descriptor descriptor.pb\n" +
		"# @grpc-reflection false\n" +
		"# @grpc-plaintext true\n" +
		"# @grpc-authority api.internal\n" +
		"# @grpc-metadata x-id: 1\n" +
		"# @grpc-metadata x-role: admin\n" +
		"\n" +
		"{\"id\":1}\n"

	if got != want {
		t.Fatalf("RenderRequestText() mismatch\nwant:\n%s\ngot:\n%s", want, got)
	}
}
