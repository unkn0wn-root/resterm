package grpcclient

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func descriptorRequest(t *testing.T) (*restfile.GRPCRequest, Options) {
	t.Helper()

	dir := t.TempDir()
	data, err := proto.Marshal(testSvcDescriptorSet())
	if err != nil {
		t.Fatalf("marshal descriptor set: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "svc.protoset"), data, 0o600); err != nil {
		t.Fatalf("write descriptor set: %v", err)
	}

	return &restfile.GRPCRequest{
		Target:        "127.0.0.1:1",
		FullMethod:    "/pkg.Svc/Call",
		DescriptorSet: "svc.protoset",
	}, Options{BaseDir: dir}
}

func stubClient(conn *stubConn) *Client {
	return NewClient(WithDialer(func(string, []grpc.DialOption) (Conn, error) {
		return conn, nil
	}))
}

func TestExecuteUnarySuccess(t *testing.T) {
	gr, opt := descriptorRequest(t)
	conn := &stubConn{
		header:  metadata.Pairs("x-req-id", "abc"),
		trailer: metadata.Pairs("x-trace", "xyz"),
	}
	client := stubClient(conn)

	resp, err := client.Execute(context.Background(), &restfile.Request{GRPC: gr}, opt, nil)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if resp.StatusCode != codes.OK || resp.StatusMessage != statusTextOK {
		t.Fatalf("status = %v %q, want OK", resp.StatusCode, resp.StatusMessage)
	}
	if resp.ContentType != defaultContentType {
		t.Fatalf("content type = %q, want %q", resp.ContentType, defaultContentType)
	}
	if resp.WireContentType != wireContentType {
		t.Fatalf("wire content type = %q, want %q", resp.WireContentType, wireContentType)
	}
	if string(resp.Body) != resp.Message {
		t.Fatalf("Body %q and Message %q disagree", resp.Body, resp.Message)
	}
	if !strings.Contains(resp.Message, "{") {
		t.Fatalf("expected a json body, got %q", resp.Message)
	}
	if got := resp.Headers["x-req-id"]; len(got) != 1 || got[0] != "abc" {
		t.Fatalf("headers = %v, want x-req-id abc", resp.Headers)
	}
	if got := resp.Trailers["x-trace"]; len(got) != 1 || got[0] != "xyz" {
		t.Fatalf("trailers = %v, want x-trace xyz", resp.Trailers)
	}
	if !conn.closed {
		t.Fatal("Execute leaked the connection")
	}
}

func TestExecuteUnaryNonOKKeepsResponse(t *testing.T) {
	gr, opt := descriptorRequest(t)
	conn := &stubConn{
		invoke: func(context.Context, string, any, any) error {
			return status.Error(codes.NotFound, "missing")
		},
	}

	resp, err := stubClient(conn).Execute(context.Background(), &restfile.Request{GRPC: gr}, opt, nil)
	if err == nil {
		t.Fatal("expected the rpc error to surface")
	}
	if resp == nil {
		t.Fatal("expected a response alongside the error")
	}
	if resp.StatusCode != codes.NotFound || resp.StatusMessage != "missing" {
		t.Fatalf("status = %v %q, want NotFound missing", resp.StatusCode, resp.StatusMessage)
	}
	if got := resp.StatusText(); got != "NotFound (missing)" {
		t.Fatalf("StatusText() = %q", got)
	}
}

func TestExecuteSendsMetadata(t *testing.T) {
	gr, opt := descriptorRequest(t)
	gr.Metadata = []restfile.MetadataPair{{Key: "X-Trace-Id", Value: "t1"}}
	req := &restfile.Request{GRPC: gr, Headers: map[string][]string{"X-Req-Id": {"r1"}}}

	conn := &stubConn{}
	if _, err := stubClient(conn).Execute(context.Background(), req, opt, nil); err != nil {
		t.Fatalf("execute: %v", err)
	}

	md, ok := metadata.FromOutgoingContext(conn.lastCtx)
	if !ok {
		t.Fatal("expected outgoing metadata on the call context")
	}
	if got := md.Get("x-trace-id"); len(got) != 1 || got[0] != "t1" {
		t.Fatalf("x-trace-id = %v", got)
	}
	if got := md.Get("x-req-id"); len(got) != 1 || got[0] != "r1" {
		t.Fatalf("x-req-id = %v", got)
	}
}

func TestExecuteGuards(t *testing.T) {
	gr, opt := descriptorRequest(t)

	tests := []struct {
		name string
		gr   *restfile.GRPCRequest
		want string
	}{
		{"nil request", nil, "missing grpc metadata"},
		{
			name: "no target",
			gr:   &restfile.GRPCRequest{FullMethod: gr.FullMethod},
			want: "grpc target not specified",
		},
		{
			name: "no method",
			gr:   &restfile.GRPCRequest{Target: gr.Target},
			want: "grpc method not specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := stubClient(&stubConn{}).
				Execute(context.Background(), &restfile.Request{GRPC: tt.gr}, opt, nil)
			if resp != nil {
				t.Fatalf("expected no response, got %+v", resp)
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestClientCloneSnapshotsDialer(t *testing.T) {
	var dialed bool
	c := NewClient(WithDialer(func(string, []grpc.DialOption) (Conn, error) {
		dialed = true
		return &stubConn{}, nil
	}))

	clone := c.Clone()
	c.dial = func(string, []grpc.DialOption) (Conn, error) { return &stubConn{}, nil }

	gr, opt := descriptorRequest(t)
	if _, err := clone.Execute(context.Background(), &restfile.Request{GRPC: gr}, opt, nil); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !dialed {
		t.Fatal("Clone did not keep the dialer it was created with")
	}

	var nilClient *Client
	if nilClient.Clone() != nil {
		t.Fatal("Clone() on nil should stay nil")
	}
}

func TestNewClientDefaultsToRealDialer(t *testing.T) {
	if NewClient().dial == nil {
		t.Fatal("NewClient() left the dialer unset")
	}
	if NewClient(nil).dial == nil {
		t.Fatal("NewClient(nil) left the dialer unset")
	}
	if NewClient(WithDialer(nil)).dial == nil {
		t.Fatal("WithDialer(nil) cleared the default dialer")
	}
}
