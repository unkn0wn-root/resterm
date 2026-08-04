package grpcx

import (
	"context"
	"fmt"
	"strings"

	"github.com/unkn0wn-root/resterm/internal/diag"
	"github.com/unkn0wn-root/resterm/internal/restfile"
	"google.golang.org/grpc"
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

func (m methodID) symbol() string {
	return string(m.svc) + "." + string(m.method)
}

// descriptorSource loads method descriptors from a file or server reflection.
type descriptorSource interface {
	files(ctx context.Context, id methodID) (*protoregistry.Files, error)
}

type fileDescriptorSource struct {
	path string
	rd   reader
}

func (s fileDescriptorSource) files(_ context.Context, _ methodID) (*protoregistry.Files, error) {
	set, err := readDescriptorSet(s.path, s.rd)
	if err != nil {
		return nil, err
	}
	return filesFromDescriptorFile(set)
}

type reflectionSource struct {
	conn grpc.ClientConnInterface
}

func (s reflectionSource) files(ctx context.Context, id methodID) (*protoregistry.Files, error) {
	set, err := fetchDescriptorsViaReflection(ctx, s.conn, id)
	if err != nil {
		return nil, err
	}
	return filesFromSet(set, "build descriptors from reflection")
}

func descriptorSourceFor(
	conn grpc.ClientConnInterface,
	gr *restfile.GRPCRequest,
	rd reader,
) (descriptorSource, error) {
	if gr.DescriptorSet != "" {
		return fileDescriptorSource{path: gr.DescriptorSet, rd: rd}, nil
	}
	if !gr.UseReflection {
		return nil, diag.New(
			diag.ClassProtocol,
			"grpc reflection disabled and no descriptor provided",
			grpcComponent,
		)
	}
	return reflectionSource{conn: conn}, nil
}

// methodTarget keeps the method and codec on the same descriptor registry so
// Any payloads can resolve service-local types.
type methodTarget struct {
	desc  protoreflect.MethodDescriptor
	codec codec
}

func resolveMethod(
	ctx context.Context,
	conn grpc.ClientConnInterface,
	gr *restfile.GRPCRequest,
	id methodID,
	rd reader,
) (methodTarget, error) {
	src, err := descriptorSourceFor(conn, gr, rd)
	if err != nil {
		return methodTarget{}, err
	}

	files, err := src.files(ctx, id)
	if err != nil {
		return methodTarget{}, err
	}

	desc, err := findMethod(files, id)
	if err != nil {
		return methodTarget{}, err
	}
	return methodTarget{desc: desc, codec: newCodec(files)}, nil
}

func parseFullMethod(full string) (methodID, error) {
	if full == "" {
		return methodID{}, diag.New(diag.ClassProtocol, "grpc method not specified", grpcComponent)
	}

	trimmed := strings.TrimPrefix(full, "/")
	svc, method, ok := strings.Cut(trimmed, "/")
	if !ok || svc == "" || method == "" || strings.Contains(method, "/") {
		return methodID{}, diag.New(
			diag.ClassProtocol,
			fmt.Sprintf("invalid grpc method %q", full),
			grpcComponent,
		)
	}

	return methodID{
		full:   "/" + trimmed,
		svc:    protoreflect.FullName(svc),
		method: protoreflect.Name(method),
	}, nil
}

func readDescriptorSet(path string, rd reader) (*descriptorpb.FileDescriptorSet, error) {
	data, err := rd.read(path, "grpc descriptor")
	if err != nil {
		return nil, err
	}

	set := &descriptorpb.FileDescriptorSet{}
	if err := proto.Unmarshal(data, set); err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, "parse descriptor set", grpcComponent)
	}
	return set, nil
}

func filesFromSet(set *descriptorpb.FileDescriptorSet, msg string) (*protoregistry.Files, error) {
	files, err := protodesc.NewFiles(set)
	if err != nil {
		return nil, diag.WrapAs(diag.ClassProtocol, err, msg, grpcComponent)
	}
	return files, nil
}

// Add the protoc hint that protobuf's unresolved-import error leaves out.
func filesFromDescriptorFile(set *descriptorpb.FileDescriptorSet) (*protoregistry.Files, error) {
	files, err := filesFromSet(set, "build descriptors from file")
	if err == nil {
		return files, nil
	}
	if !strings.Contains(err.Error(), "could not resolve import") {
		return nil, err
	}
	return nil, diag.WrapAs(
		diag.ClassProtocol,
		err,
		"descriptor set is incomplete (rebuild it with protoc --include_imports)",
		grpcComponent,
	)
}

func findMethod(
	files *protoregistry.Files,
	id methodID,
) (protoreflect.MethodDescriptor, error) {
	desc, err := files.FindDescriptorByName(id.svc)
	if err != nil {
		return nil, diag.WrapAs(
			diag.ClassProtocol,
			err,
			fmt.Sprintf("service %s not found", id.svc),
			grpcComponent,
		)
	}

	svc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, diag.New(
			diag.ClassProtocol,
			fmt.Sprintf("descriptor for %s is not a service", id.svc),
			grpcComponent,
		)
	}

	md := svc.Methods().ByName(id.method)
	if md == nil {
		return nil, diag.New(
			diag.ClassProtocol,
			fmt.Sprintf("method %s not found on %s", id.method, id.svc),
			grpcComponent,
		)
	}
	return md, nil
}
