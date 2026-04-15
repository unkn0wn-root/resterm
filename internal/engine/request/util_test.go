package request

import (
	"reflect"
	"testing"

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
