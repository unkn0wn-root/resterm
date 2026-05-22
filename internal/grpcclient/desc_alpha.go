package grpcclient

import (
	"context"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"google.golang.org/grpc"
	reflectalpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
	"google.golang.org/protobuf/types/descriptorpb"
)

//nolint:staticcheck // v1alpha is deprecated, but some servers still expose only this reflection service.
func fetchReflectAlpha(
	ctx context.Context,
	conn *grpc.ClientConn,
	sym string,
) (*descriptorpb.FileDescriptorSet, error) {
	client := reflectalpha.NewServerReflectionClient(conn)
	stream, err := client.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "open reflection stream")
	}
	req := &reflectalpha.ServerReflectionRequest{
		MessageRequest: &reflectalpha.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: sym,
		},
	}

	return reflectRoundTrip(stream, req, alphaReflectErr, alphaReflectFiles)
}

//nolint:staticcheck // v1alpha is deprecated, but some servers still expose only this reflection service.
func alphaReflectErr(res *reflectalpha.ServerReflectionResponse) (int32, string, bool) {
	errRes := res.GetErrorResponse()
	if errRes == nil {
		return 0, "", false
	}
	return errRes.GetErrorCode(), errRes.GetErrorMessage(), true
}

//nolint:staticcheck // v1alpha is deprecated, but some servers still expose only this reflection service.
func alphaReflectFiles(res *reflectalpha.ServerReflectionResponse) ([][]byte, bool) {
	fileRes := res.GetFileDescriptorResponse()
	if fileRes == nil {
		return nil, false
	}
	return fileRes.GetFileDescriptorProto(), true
}
