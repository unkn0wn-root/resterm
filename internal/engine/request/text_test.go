package request

import (
	"slices"
	"testing"

	"github.com/unkn0wn-root/resterm/internal/parser"
	"github.com/unkn0wn-root/resterm/internal/restfile"
)

func TestRenderRequestTextKeepsGRPCSchemaSource(t *testing.T) {
	tests := []struct {
		name string
		gr   restfile.GRPCRequest
	}{
		{
			name: "proto files",
			gr: restfile.GRPCRequest{
				Target:      "localhost:50051",
				FullMethod:  "/pkg.Svc/Call",
				ProtoFiles:  []string{"proto/svc.proto", "proto/types.proto"},
				ImportPaths: []string{"proto", "vendor"},
			},
		},
		{
			name: "descriptor set",
			gr: restfile.GRPCRequest{
				Target:        "localhost:50051",
				FullMethod:    "/pkg.Svc/Call",
				DescriptorSet: "api.protoset",
				UseReflection: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &restfile.Request{Method: "GRPC", URL: tt.gr.Target, GRPC: &tt.gr}
			text := renderRequestText(req)
			doc := parser.Parse("grpc.http", []byte(text))
			if len(doc.Errors) != 0 || len(doc.Requests) != 1 || doc.Requests[0].GRPC == nil {
				t.Fatalf("parse %q: errors %v, %d requests", text, doc.Errors, len(doc.Requests))
			}
			got := doc.Requests[0].GRPC
			if !slices.Equal(got.ProtoFiles, tt.gr.ProtoFiles) ||
				!slices.Equal(got.ImportPaths, tt.gr.ImportPaths) ||
				got.DescriptorSet != tt.gr.DescriptorSet ||
				got.UseReflection != tt.gr.UseReflection {
				t.Fatalf(
					"parse %q: proto files %q, import paths %q, descriptor %q, reflection %t",
					text, got.ProtoFiles, got.ImportPaths, got.DescriptorSet, got.UseReflection,
				)
			}
		})
	}
}
