package grpcclient

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func TestParseFullMethod(t *testing.T) {
	tests := []struct {
		name    string
		full    string
		wantSvc string
		wantMtd string
		wantErr string
	}{
		{
			name:    "leading slash",
			full:    "/pkg.Svc/Call",
			wantSvc: "pkg.Svc",
			wantMtd: "Call",
		},
		{
			name:    "missing leading slash",
			full:    "pkg.Svc/Call",
			wantSvc: "pkg.Svc",
			wantMtd: "Call",
		},
		{
			name:    "empty",
			full:    "",
			wantErr: "grpc method not specified",
		},
		{
			name:    "empty service",
			full:    "/Call",
			wantErr: "invalid grpc method",
		},
		{
			name:    "empty method",
			full:    "/pkg.Svc/",
			wantErr: "invalid grpc method",
		},
		{
			name:    "extra separator",
			full:    "/pkg.Svc/Call/Extra",
			wantErr: "invalid grpc method",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseFullMethod(tt.full)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("expected %q error, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parse full method: %v", err)
			}
			if string(got.svc) != tt.wantSvc {
				t.Fatalf("service = %q, want %q", got.svc, tt.wantSvc)
			}
			if string(got.method) != tt.wantMtd {
				t.Fatalf("method = %q, want %q", got.method, tt.wantMtd)
			}
		})
	}
}

func TestResolveMethodUsesFullMethodWithDescriptorSet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "svc.protoset")
	data, err := proto.Marshal(testSvcDescriptorSet())
	if err != nil {
		t.Fatalf("marshal descriptor set: %v", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write descriptor set: %v", err)
	}

	gr := &restfile.GRPCRequest{
		FullMethod:    "/pkg.Svc/Call",
		DescriptorSet: "svc.protoset",
		Package:       "wrong",
		Service:       "Wrong",
		Method:        "Nope",
	}
	_, md, err := NewClient().resolveMethod(context.Background(), nil, gr, Options{BaseDir: dir})
	if err != nil {
		t.Fatalf("resolve method: %v", err)
	}
	if got := string(md.FullName()); got != "pkg.Svc.Call" {
		t.Fatalf("method = %q, want pkg.Svc.Call", got)
	}
}

func testSvcDescriptorSet() *descriptorpb.FileDescriptorSet {
	return &descriptorpb.FileDescriptorSet{
		File: []*descriptorpb.FileDescriptorProto{
			{
				Name:    proto.String("svc.proto"),
				Package: proto.String("pkg"),
				Syntax:  proto.String("proto3"),
				MessageType: []*descriptorpb.DescriptorProto{
					{Name: proto.String("Msg")},
				},
				Service: []*descriptorpb.ServiceDescriptorProto{
					{
						Name: proto.String("Svc"),
						Method: []*descriptorpb.MethodDescriptorProto{
							{
								Name:       proto.String("Call"),
								InputType:  proto.String(".pkg.Msg"),
								OutputType: proto.String(".pkg.Msg"),
							},
						},
					},
				},
			},
		},
	}
}
