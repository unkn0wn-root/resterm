package grpcclient

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	reflectv1 "google.golang.org/grpc/reflection/grpc_reflection_v1"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

type methodID struct {
	full   string
	svc    protoreflect.FullName
	method protoreflect.Name
}

func (c *Client) resolveMethod(
	ctx context.Context,
	conn *grpc.ClientConn,
	gr *restfile.GRPCRequest,
	opt Options,
) (*protoregistry.Files, protoreflect.MethodDescriptor, error) {
	id, err := parseFullMethod(gr.FullMethod)
	if err != nil {
		return nil, nil, err
	}

	if strings.TrimSpace(gr.DescriptorSet) != "" {
		files, err := c.loadDescriptorSet(gr.DescriptorSet, opt.BaseDir)
		if err != nil {
			return nil, nil, err
		}
		md, err := findMethod(files, id)
		return files, md, err
	}

	if !gr.UseReflection {
		return nil, nil, diag.New(
			diag.ClassProtocol,
			"grpc reflection disabled and no descriptor provided",
		)
	}

	set, err := fetchDescriptorsViaReflection(ctx, conn, id)
	if err != nil {
		return nil, nil, err
	}
	files, err := filesFromSet(set, "build descriptors from reflection")
	if err != nil {
		return nil, nil, err
	}
	md, err := findMethod(files, id)
	return files, md, err
}

func parseFullMethod(full string) (methodID, error) {
	full = strings.TrimSpace(full)
	if full == "" {
		return methodID{}, diag.New(diag.ClassProtocol, "grpc method not specified")
	}

	trimmed := strings.TrimPrefix(full, "/")
	svc, method, ok := strings.Cut(trimmed, "/")
	if !ok || svc == "" || method == "" || strings.Contains(method, "/") {
		return methodID{}, diag.Newf(diag.ClassProtocol, "invalid grpc method %q", full)
	}

	return methodID{
		full:   "/" + trimmed,
		svc:    protoreflect.FullName(svc),
		method: protoreflect.Name(method),
	}, nil
}

func (m methodID) symbol() string {
	return string(m.svc) + "." + string(m.method)
}

func (c *Client) loadDescriptorSet(path, baseDir string) (*protoregistry.Files, error) {
	set, err := readDescriptorSet(path, baseDir)
	if err != nil {
		return nil, err
	}
	return filesFromSet(set, "build descriptors from file")
}

func readDescriptorSet(path, baseDir string) (*descriptorpb.FileDescriptorSet, error) {
	orig := path
	if !filepath.IsAbs(path) && baseDir != "" {
		path = filepath.Join(baseDir, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, diag.WrapAsf(diag.ClassFilesystem, err, "read grpc descriptor %s", orig)
	}

	set := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(data, set); err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "parse descriptor set")
	}
	return set, nil
}

func filesFromSet(set *descriptorpb.FileDescriptorSet, msg string) (*protoregistry.Files, error) {
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, msg)
	}
	return files, nil
}

func findMethod(
	files *protoregistry.Files,
	id methodID,
) (protoreflect.MethodDescriptor, error) {
	desc, err := files.FindDescriptorByName(id.svc)
	if err != nil {
		return nil, diag.WrapAsf(diag.ClassProtocol, err, "service %s not found", id.svc)
	}

	svc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, diag.Newf(diag.ClassProtocol, "descriptor for %s is not a service", id.svc)
	}

	md := svc.Methods().ByName(id.method)
	if md == nil {
		return nil, diag.Newf(diag.ClassProtocol, "method %s not found on %s", id.method, id.svc)
	}
	return md, nil
}

func fetchDescriptorsViaReflection(
	ctx context.Context,
	conn *grpc.ClientConn,
	id methodID,
) (*descriptorpb.FileDescriptorSet, error) {
	sym := id.symbol()

	set, err := fetchReflectV1(ctx, conn, sym)
	if err == nil {
		return set, nil
	}

	alpha, alphaErr := fetchReflectAlpha(ctx, conn, sym)
	if alphaErr == nil {
		return alpha, nil
	}

	// Both failed: if v1 is merely unimplemented, the v1alpha error is the
	// meaningful one; otherwise the v1 error describes the real failure.
	if status.Code(err) == codes.Unimplemented {
		return nil, alphaErr
	}
	return nil, err
}

func fetchReflectV1(
	ctx context.Context,
	conn *grpc.ClientConn,
	sym string,
) (*descriptorpb.FileDescriptorSet, error) {
	client := reflectv1.NewServerReflectionClient(conn)
	stream, err := client.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "open reflection stream")
	}
	req := &reflectv1.ServerReflectionRequest{
		MessageRequest: &reflectv1.ServerReflectionRequest_FileContainingSymbol{
			FileContainingSymbol: sym,
		},
	}

	return reflectRoundTrip(stream, req, v1ReflectErr, v1ReflectFiles)
}

func reflectRoundTrip[Req any, Res any](
	stream grpc.BidiStreamingClient[Req, Res],
	req *Req,
	errResp func(*Res) (int32, string, bool),
	filesResp func(*Res) ([][]byte, bool),
) (set *descriptorpb.FileDescriptorSet, err error) {
	defer func() {
		if closeErr := stream.CloseSend(); closeErr != nil && err == nil {
			err = diag.WrapAs(diag.ClassProtocol, closeErr, "close reflection stream")
		}
	}()

	if err := stream.Send(req); err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "send reflection request")
	}

	res, err := stream.Recv()
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "receive reflection response")
	}
	if code, msg, ok := errResp(res); ok {
		return nil, reflectionErr(code, msg)
	}
	raw, ok := filesResp(res)
	if !ok {
		return nil, diag.New(diag.ClassProtocol, "reflection response missing descriptors")
	}
	return decodeFileDescriptors(raw)
}

func v1ReflectErr(res *reflectv1.ServerReflectionResponse) (int32, string, bool) {
	errRes := res.GetErrorResponse()
	if errRes == nil {
		return 0, "", false
	}
	return errRes.GetErrorCode(), errRes.GetErrorMessage(), true
}

func v1ReflectFiles(res *reflectv1.ServerReflectionResponse) ([][]byte, bool) {
	fileRes := res.GetFileDescriptorResponse()
	if fileRes == nil {
		return nil, false
	}
	return fileRes.GetFileDescriptorProto(), true
}

func reflectionErr(code int32, msg string) error {
	name := codes.Code(code).String()
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return diag.Newf(diag.ClassProtocol, "grpc reflection error %s", name)
	}
	return diag.Newf(diag.ClassProtocol, "grpc reflection error %s: %s", name, msg)
}

func decodeFileDescriptors(raw [][]byte) (*descriptorpb.FileDescriptorSet, error) {
	set := &descriptorpb.FileDescriptorSet{}
	for _, data := range raw {
		fd := &descriptorpb.FileDescriptorProto{}
		if err := proto.Unmarshal(data, fd); err != nil {
			return nil, diag.WrapAs(diag.ClassProtocol, err, "decode reflected descriptor")
		}
		set.File = append(set.File, fd)
	}
	return set, nil
}
