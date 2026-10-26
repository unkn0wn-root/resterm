package grpcx

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/filelookup"
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

func TestDescriptorSourceFor(t *testing.T) {
	tests := []struct {
		name    string
		gr      restfile.GRPCRequest
		want    descriptorSource
		wantErr string
	}{
		{
			name: "descriptor set",
			gr:   restfile.GRPCRequest{DescriptorSet: "svc.protoset", UseReflection: true},
			want: fileDescriptorSource{path: "svc.protoset"},
		},
		{
			name: "proto files",
			gr: restfile.GRPCRequest{
				ProtoFiles:    []string{"svc.proto"},
				ImportPaths:   []string{"proto"},
				UseReflection: true,
			},
			want: protoSource{paths: []string{"svc.proto"}, imports: []string{"proto"}},
		},
		{
			name: "descriptor set wins over proto files",
			gr: restfile.GRPCRequest{
				DescriptorSet: "svc.protoset",
				ProtoFiles:    []string{"svc.proto"},
				UseReflection: true,
			},
			want: fileDescriptorSource{path: "svc.protoset"},
		},
		{
			name: "reflection",
			gr:   restfile.GRPCRequest{UseReflection: true},
			want: reflectionSource{},
		},
		{
			name:    "nothing to load from",
			gr:      restfile.GRPCRequest{},
			wantErr: "no descriptor or proto files provided",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src, err := descriptorSourceFor(nil, &tt.gr, reader{})
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("descriptor source: %v", err)
			}
			if !reflect.DeepEqual(src, tt.want) {
				t.Fatalf("source = %#v, want %#v", src, tt.want)
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
		Method:        "Missing",
	}
	id, err := parseFullMethod(gr.FullMethod)
	if err != nil {
		t.Fatalf("parse full method: %v", err)
	}
	target, err := resolveMethod(
		context.Background(),
		nil,
		gr,
		id,
		newReader(filelookup.OSFileSystem{}, Options{BaseDir: dir}),
	)
	if err != nil {
		t.Fatalf("resolve method: %v", err)
	}
	if got := string(target.desc.FullName()); got != "pkg.Svc.Call" {
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
							{
								Name:            proto.String("ServerStream"),
								InputType:       proto.String(".pkg.Msg"),
								OutputType:      proto.String(".pkg.Msg"),
								ServerStreaming: proto.Bool(true),
							},
							{
								Name:            proto.String("ClientStream"),
								InputType:       proto.String(".pkg.Msg"),
								OutputType:      proto.String(".pkg.Msg"),
								ClientStreaming: proto.Bool(true),
							},
						},
					},
				},
			},
		},
	}
}
