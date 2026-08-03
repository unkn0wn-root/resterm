package grpcclient

import (
	"context"
	"fmt"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	reflectv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/descriptorpb"
)

func fetchDescriptorsViaReflection(
	ctx context.Context,
	conn grpc.ClientConnInterface,
	id methodID,
) (*descriptorpb.FileDescriptorSet, error) {
	sym := id.symbol()

	set, err := v1Reflection.fetch(ctx, conn, sym)
	if err == nil {
		return set, nil
	}

	alpha, alphaErr := alphaReflection.fetch(ctx, conn, sym)
	if alphaErr == nil {
		return alpha, nil
	}

	// Prefer the v1alpha error only when the v1 service is unavailable.
	if status.Code(err) == codes.Unimplemented {
		return nil, alphaErr
	}
	return nil, err
}

// reflectionAPI lets the v1 and v1alpha services share one descriptor walk.
type reflectionAPI[Req any, Res any] struct {
	open     func(context.Context, grpc.ClientConnInterface) (grpc.BidiStreamingClient[Req, Res], error)
	bySymbol func(string) *Req
	byName   func(string) *Req
	errOf    func(*Res) (int32, string, bool)
	filesOf  func(*Res) ([][]byte, bool)
}

var v1Reflection = reflectionAPI[reflectv1.ServerReflectionRequest, reflectv1.ServerReflectionResponse]{
	open: func(
		ctx context.Context,
		conn grpc.ClientConnInterface,
	) (grpc.BidiStreamingClient[reflectv1.ServerReflectionRequest, reflectv1.ServerReflectionResponse], error) {
		return reflectv1.NewServerReflectionClient(conn).ServerReflectionInfo(ctx)
	},
	bySymbol: func(sym string) *reflectv1.ServerReflectionRequest {
		return &reflectv1.ServerReflectionRequest{
			MessageRequest: &reflectv1.ServerReflectionRequest_FileContainingSymbol{
				FileContainingSymbol: sym,
			},
		}
	},
	byName: func(name string) *reflectv1.ServerReflectionRequest {
		return &reflectv1.ServerReflectionRequest{
			MessageRequest: &reflectv1.ServerReflectionRequest_FileByFilename{
				FileByFilename: name,
			},
		}
	},
	errOf: func(res *reflectv1.ServerReflectionResponse) (int32, string, bool) {
		errRes := res.GetErrorResponse()
		if errRes == nil {
			return 0, "", false
		}
		return errRes.GetErrorCode(), errRes.GetErrorMessage(), true
	},
	filesOf: func(res *reflectv1.ServerReflectionResponse) ([][]byte, bool) {
		fileRes := res.GetFileDescriptorResponse()
		if fileRes == nil {
			return nil, false
		}
		return fileRes.GetFileDescriptorProto(), true
	},
}

func (a reflectionAPI[Req, Res]) fetch(
	ctx context.Context,
	conn grpc.ClientConnInterface,
	sym string,
) (*descriptorpb.FileDescriptorSet, error) {
	stream, err := a.open(ctx, conn)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "open reflection stream", grpcComponent)
	}
	return a.roundTrip(stream, a.bySymbol(sym))
}

// maxReflectRounds stops a server from extending the dependency walk forever.
const maxReflectRounds = 64

// roundTrip follows missing imports because some reflection servers return only
// the requested file. It also removes duplicates before the registry is built.
func (a reflectionAPI[Req, Res]) roundTrip(
	stream grpc.BidiStreamingClient[Req, Res],
	req *Req,
) (set *descriptorpb.FileDescriptorSet, err error) {
	defer func() {
		if closeErr := stream.CloseSend(); closeErr != nil && err == nil {
			err = diag.WrapAs(
				diag.ClassProtocol,
				closeErr,
				"close reflection stream",
				grpcComponent,
			)
		}
	}()

	set = &descriptorpb.FileDescriptorSet{}
	have := make(map[string]bool)

	for round := 0; round < maxReflectRounds; round++ {
		files, err := a.exchange(stream, req)
		if err != nil {
			return nil, err
		}
		for _, fd := range files {
			if name := fd.GetName(); name != "" && !have[name] {
				have[name] = true
				set.File = append(set.File, fd)
			}
		}

		missing := missingDependency(set.File, have)
		if missing == "" {
			return set, nil
		}
		req = a.byName(missing)
	}
	return set, nil
}

func (a reflectionAPI[Req, Res]) exchange(
	stream grpc.BidiStreamingClient[Req, Res],
	req *Req,
) ([]*descriptorpb.FileDescriptorProto, error) {
	if err := stream.Send(req); err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "send reflection request", grpcComponent)
	}

	res, err := stream.Recv()
	if err != nil {
		return nil, diag.WrapAs(
			diag.ClassProtocol,
			err,
			"receive reflection response",
			grpcComponent,
		)
	}
	if code, msg, ok := a.errOf(res); ok {
		return nil, reflectionErr(code, msg)
	}
	raw, ok := a.filesOf(res)
	if !ok {
		return nil, diag.New(
			diag.ClassProtocol,
			"reflection response missing descriptors",
			grpcComponent,
		)
	}
	return decodeFileDescriptors(raw)
}

func missingDependency(files []*descriptorpb.FileDescriptorProto, have map[string]bool) string {
	for _, fd := range files {
		for _, dep := range fd.GetDependency() {
			if !have[dep] {
				return dep
			}
		}
	}
	return ""
}

func reflectionErr(code int32, msg string) error {
	name := codes.Code(code).String()
	if msg == "" {
		return diag.New(
			diag.ClassProtocol,
			fmt.Sprintf("grpc reflection error %s", name),
			grpcComponent,
		)
	}
	return diag.New(
		diag.ClassProtocol,
		fmt.Sprintf("grpc reflection error %s: %s", name, msg),
		grpcComponent,
	)
}

func decodeFileDescriptors(raw [][]byte) ([]*descriptorpb.FileDescriptorProto, error) {
	out := make([]*descriptorpb.FileDescriptorProto, 0, len(raw))
	for _, data := range raw {
		fd := &descriptorpb.FileDescriptorProto{}
		if err := proto.Unmarshal(data, fd); err != nil {
			return nil, diag.WrapAs(
				diag.ClassProtocol,
				err,
				"decode reflected descriptor",
				grpcComponent,
			)
		}
		out = append(out, fd)
	}
	return out, nil
}
