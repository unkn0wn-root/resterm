package grpcclient

import (
	"context"

	"google.golang.org/grpc"
	reflectalpha "google.golang.org/grpc/reflection/grpc_reflection_v1alpha"
)

//nolint:staticcheck // v1alpha is deprecated, but some servers still expose only this reflection service.
var alphaReflection = reflectionAPI[reflectalpha.ServerReflectionRequest, reflectalpha.ServerReflectionResponse]{
	open: func(
		ctx context.Context,
		conn grpc.ClientConnInterface,
	) (grpc.BidiStreamingClient[reflectalpha.ServerReflectionRequest, reflectalpha.ServerReflectionResponse], error) {
		return reflectalpha.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	},
	bySymbol: func(sym string) *reflectalpha.ServerReflectionRequest {
		return &reflectalpha.ServerReflectionRequest{
			MessageRequest: &reflectalpha.ServerReflectionRequest_FileContainingSymbol{
				FileContainingSymbol: sym,
			},
		}
	},
	byName: func(name string) *reflectalpha.ServerReflectionRequest {
		return &reflectalpha.ServerReflectionRequest{
			MessageRequest: &reflectalpha.ServerReflectionRequest_FileByFilename{
				FileByFilename: name,
			},
		}
	},
	errOf: func(res *reflectalpha.ServerReflectionResponse) (int32, string, bool) {
		errRes := res.GetErrorResponse()
		if errRes == nil {
			return 0, "", false
		}
		return errRes.GetErrorCode(), errRes.GetErrorMessage(), true
	},
	filesOf: func(res *reflectalpha.ServerReflectionResponse) ([][]byte, bool) {
		fileRes := res.GetFileDescriptorResponse()
		if fileRes == nil {
			return nil, false
		}
		return fileRes.GetFileDescriptorProto(), true
	},
}
